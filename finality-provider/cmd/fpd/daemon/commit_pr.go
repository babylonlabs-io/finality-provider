package daemon

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	bbntypes "github.com/babylonlabs-io/babylon/types"
	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
)

// CommandCommitPubRand returns the commit-pubrand command by connecting to the fpd daemon.
func CommandCommitPubRand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "commit-pubrand [fp-eots-pk-hex] [block-height]",
		Aliases: []string{"cpr"},
		Short:   "Manually trigger public randomness commitment for a finality provider",
		Example: `fpd commit-pubrand --home /home/user/.fpd [fp-eots-pk-hex] [block-height]`,
		Args:    cobra.ExactArgs(2),
		RunE:    fpcmd.RunEWithClientCtx(runCommandCommitPubRand),
	}
	return cmd
}

func runCommandCommitPubRand(ctx client.Context, cmd *cobra.Command, args []string) error {
	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return err
	}
	blkHeight, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		return err
	}

	// Get homePath from context like in start.go
	clientCtx := client.GetClientContextFromCmd(cmd)
	homePath, err := filepath.Abs(clientCtx.HomeDir)
	if err != nil {
		return err
	}
	homePath = util.CleanAndExpandPath(homePath)

	cfg, err := fpcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// override to control the exact block height to commit to
	cfg.MinRandHeightGap = 0
	prCommitInterval := cfg.RandomnessCommitInterval

	logger, err := log.NewRootLoggerWithFile(fpcfg.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	dbBackend, err := cfg.DatabaseConfig.GetDbBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	fpApp, err := service.NewFinalityProviderAppFromConfig(cfg, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create finality-provider app: %w", err)
	}

	fp, err := fpApp.GetFinalityProviderInstance(fpPk)
	if err != nil {
		return fmt.Errorf("failed to get finality provider instance: %w", err)
	}

	return commitUntilHeight(fp, blkHeight, prCommitInterval)
}

func commitUntilHeight(
	fp *service.FinalityProviderInstance,
	blkHeight uint64,
	interval time.Duration,
) error {
	commitRandTicker := time.NewTicker(interval)
	defer commitRandTicker.Stop()

	lastCommittedHeight, err := fp.GetLastCommittedHeight()
	if err != nil {
		return fmt.Errorf("failed to get last committed height: %w", err)
	}

	for lastCommittedHeight < blkHeight {
		<-commitRandTicker.C
		_, err = fp.CommitPubRand(blkHeight)
		if err != nil {
			return fmt.Errorf("failed to commit public randomness: %w", err)
		}

		lastCommittedHeight, err = fp.GetLastCommittedHeight()
		if err != nil {
			return fmt.Errorf("failed to get last committed height: %w", err)
		}
	}
	return nil
}

package daemon

import (
	"fmt"
	"path/filepath"
	"strconv"

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
		// TODO: add --start-height as an optional flag
		Use:     "unsafe-commit-pubrand [fp-eots-pk-hex] [block-height]",
		Aliases: []string{"unsafe-cpr"},
		Short:   "[UNSAFE] Manually trigger public randomness commitment for a finality provider",
		Long: `[UNSAFE] Manually trigger public randomness commitment for a finality provider. ` +
			`WARNING: this can drain the finality provider's balance if the block-height is too high.` +
			`Note: if there is no pubrand committed before, it will only commit the pubrand for the target block-height.`,
		Example: `fpd unsafe-commit-pubrand --home /home/user/.fpd [fp-eots-pk-hex] [block-height]`,
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

	return fp.TestCommitPubRand(blkHeight)
}

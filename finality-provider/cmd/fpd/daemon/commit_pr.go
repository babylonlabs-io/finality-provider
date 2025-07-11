package daemon

import (
	"fmt"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"math"
	"path/filepath"
	"strconv"

	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	eotsclient "github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/util"
)

// CommandCommitPubRand returns the commit-pubrand command by connecting to the fpd daemon.
func CommandCommitPubRand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "unsafe-commit-pubrand [fp-eots-pk-hex] [target-height]",
		Aliases: []string{"unsafe-cpr"},
		Short:   "[UNSAFE] Manually trigger public randomness commitment for a finality provider",
		Long: `[UNSAFE] Manually trigger public randomness commitment for a finality provider.
WARNING: this can drain the finality provider's balance if the target height is too high.`,
		Example: `fpd unsafe-commit-pubrand --home /home/user/.fpd [fp-eots-pk-hex] [target-height]`,
		Args:    cobra.ExactArgs(2),
		RunE:    fpcmd.RunEWithClientCtx(runCommandCommitPubRand),
	}
	cmd.Flags().Uint64("start-height", math.MaxUint64, "The block height to start committing pubrand from (optional)")
	cmd.Flags().String(flags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")

	return cmd
}

func runCommandCommitPubRand(ctx client.Context, cmd *cobra.Command, args []string) error {
	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return err
	}
	targetHeight, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		return err
	}
	startHeight, err := cmd.Flags().GetUint64("start-height")
	if err != nil {
		return err
	}

	// Get homePath from context like in start.go
	homePath, err := filepath.Abs(ctx.HomeDir)
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

	db, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	fpStore, err := store.NewFinalityProviderStore(db)
	if err != nil {
		return fmt.Errorf("failed to initiate finality provider store: %w", err)
	}
	pubRandStore, err := store.NewPubRandProofStore(db)
	if err != nil {
		return fmt.Errorf("failed to initiate public randomness store: %w", err)
	}
	cc, err := fpcc.NewBabylonController(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create rpc client for the Babylon chain: %w", err)
	}
	if err := cc.Start(); err != nil {
		return fmt.Errorf("failed to start client controller: %w", err)
	}
	consumerCon, err := fpcc.NewConsumerController(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create rpc client for the consumer chain %s: %w", cfg.ChainType, err)
	}
	em, err := eotsclient.NewEOTSManagerGRpcClient(cfg.EOTSManagerAddress, cfg.HMACKey)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager client: %w", err)
	}

	fpMetrics := metrics.NewFpMetrics()
	poller := service.NewChainPoller(logger, cfg.PollerConfig, consumerCon, fpMetrics)

	fp, err := service.NewFinalityProviderInstance(
		fpPk, cfg, fpStore, pubRandStore, cc, consumerCon, em, poller, fpMetrics,
		make(chan<- *service.CriticalError), logger)
	if err != nil {
		return fmt.Errorf("failed to create finality-provider %s instance: %w", fpPk.MarshalHex(), err)
	}

	if startHeight == math.MaxUint64 {
		return fp.TestCommitPubRand(targetHeight)
	}

	return fp.TestCommitPubRandWithStartHeight(startHeight, targetHeight)
}

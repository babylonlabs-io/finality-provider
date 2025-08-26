package daemon

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"

	"github.com/cosmos/cosmos-sdk/client/flags"

	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	eotsclient "github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	clientctx "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/util"
)

// CommandCommitPubRand returns the commit-pubrand command by connecting to the fpd daemon.
func CommandCommitPubRand(binaryName string) *cobra.Command {
	cmd := CommandCommitPubRandTemplate(binaryName)
	cmd.RunE = clientctx.RunEWithClientCtx(runCommandCommitPubRand)

	return cmd
}

// CommandCommitPubRandTemplate returns the commit-pubrand command template
// One needs to set the RunE function to the command after creating it
func CommandCommitPubRandTemplate(binaryName string) *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "unsafe-commit-pubrand [fp-eots-pk-hex] [target-height]",
		Aliases: []string{"unsafe-cpr"},
		Short:   "[UNSAFE] Manually trigger public randomness commitment for a finality provider",
		Long: `[UNSAFE] Manually trigger public randomness commitment for a finality provider.
WARNING: this can drain the finality provider's balance if the target height is too high.`,
		Example: fmt.Sprintf(`%s unsafe-commit-pubrand --home /home/user/.fpd [fp-eots-pk-hex] [target-height]`, binaryName),
		Args:    cobra.ExactArgs(2),
	}
	cmd.Flags().Uint64("start-height", math.MaxUint64, "The block height to start committing pubrand from (optional)")
	cmd.Flags().String(flags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")

	return cmd
}

func runCommandCommitPubRand(ctx client.Context, cmd *cobra.Command, args []string) error {
	// Get homePath from context like in start.go
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute home path: %w", err)
	}
	homePath = util.CleanAndExpandPath(homePath)

	cfg, err := fpcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	return RunCommandCommitPubRandWithConfig(ctx, cmd, homePath, cfg, args)
}

func RunCommandCommitPubRandWithConfig(_ client.Context, cmd *cobra.Command, homePath string, cfg *fpcfg.Config, args []string) error {
	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return fmt.Errorf("failed to parse BIP340 public key from hex: %w", err)
	}
	targetHeight, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse target height: %w", err)
	}
	startHeight, err := cmd.Flags().GetUint64("start-height")
	if err != nil {
		return fmt.Errorf("failed to get start-height flag: %w", err)
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
	cc, err := fpcc.NewBabylonController(cfg.BabylonConfig, logger)
	if err != nil {
		return fmt.Errorf("failed to create rpc client for the Babylon chain: %w", err)
	}
	if err := cc.Start(); err != nil {
		return fmt.Errorf("failed to start client controller: %w", err)
	}
	consumerCon, err := babylon.NewBabylonConsumerController(cfg.BabylonConfig, logger)
	if err != nil {
		return fmt.Errorf("failed to create rpc client for the consumer chain: %w", err)
	}
	em, err := eotsclient.NewEOTSManagerGRPCClient(cfg.EOTSManagerAddress, cfg.HMACKey)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager client: %w", err)
	}

	fpMetrics := metrics.NewFpMetrics()
	poller := service.NewChainPoller(logger, cfg.PollerConfig, consumerCon, fpMetrics)

	rndCommitter := service.NewDefaultRandomnessCommitter(
		service.NewRandomnessCommitterConfig(cfg.NumPubRand, int64(cfg.TimestampingDelayBlocks), cfg.ContextSigningHeight),
		service.NewPubRandState(pubRandStore), consumerCon, em, logger, fpMetrics)
	heightDeterminer := service.NewStartHeightDeterminer(consumerCon, cfg.PollerConfig, logger)
	finalitySubmitter := service.NewDefaultFinalitySubmitter(consumerCon,
		em,
		rndCommitter.GetPubRandProofList,
		service.NewDefaultFinalitySubmitterConfig(cfg.MaxSubmissionRetries,
			cfg.ContextSigningHeight,
			cfg.SubmissionRetryInterval),
		logger,
		fpMetrics,
	)

	fp, err := service.NewFinalityProviderInstance(
		fpPk, cfg, fpStore, pubRandStore, cc, consumerCon, em, poller, rndCommitter, heightDeterminer, finalitySubmitter, fpMetrics,
		make(chan<- *service.CriticalError), logger)
	if err != nil {
		return fmt.Errorf("failed to create finality-provider %s instance: %w", fpPk.MarshalHex(), err)
	}

	fpTester := fp.NewTestHelper()

	if startHeight == math.MaxUint64 {
		if err := fpTester.CommitPubRand(cmd.Context(), targetHeight); err != nil {
			return fmt.Errorf("failed to commit pubrand: %w", err)
		}

		return nil
	}

	if err := fpTester.CommitPubRandWithStartHeight(cmd.Context(), startHeight, targetHeight); err != nil {
		return fmt.Errorf("failed to commit pubrand with start height: %w", err)
	}

	return nil
}

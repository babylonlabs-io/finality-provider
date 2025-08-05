package service

import (
	"context"
	"fmt"

	rollupfpcc "github.com/babylonlabs-io/finality-provider/bsn/rollup/clientcontroller"
	rollupfpcfg "github.com/babylonlabs-io/finality-provider/bsn/rollup/config"
	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/lightningnetwork/lnd/kvdb"
	"go.uber.org/zap"
)

// NewRollupBSNFinalityProviderAppFromConfig creates a new FinalityProviderApp instance from the given configuration for rollup BSN.
func NewRollupBSNFinalityProviderAppFromConfig(
	cfg *rollupfpcfg.RollupFPConfig,
	db kvdb.Backend,
	logger *zap.Logger,
) (*service.FinalityProviderApp, error) {
	cc, err := fpcc.NewBabylonController(cfg.Common.BabylonConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create rpc client for the Babylon chain: %w", err)
	}
	if err := cc.Start(); err != nil {
		return nil, fmt.Errorf("failed to start rpc client for the Babylon chain: %w", err)
	}

	consumerCon, err := rollupfpcc.NewRollupBSNController(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create rpc client for the consumer chain rollup: %w", err)
	}

	// if the EOTSManagerAddress is empty, run a local EOTS manager;
	// otherwise connect a remote one with a gRPC client
	em, err := service.InitEOTSManagerClient(cfg.Common.EOTSManagerAddress, cfg.Common.HMACKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create EOTS manager client: %w", err)
	}

	logger.Info("successfully connected to a remote EOTS manager", zap.String("address", cfg.Common.EOTSManagerAddress))

	fpMetrics := metrics.NewFpMetrics()

	poller := service.NewChainPoller(logger, cfg.Common.PollerConfig, consumerCon, fpMetrics)

	pubRandStore, err := store.NewPubRandProofStore(db)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate public randomness store: %w", err)
	}

	// Determine whether to use sparse randomness generation based on finality signature interval
	var rndCommitter types.RandomnessCommitter

	// Query the contract config to get finality signature interval
	contractConfig, err := consumerCon.QueryContractConfig(context.Background())
	if err != nil {
		// If we can't query the config, fall back to default committer with warning
		logger.Warn("Failed to query contract config, using default randomness committer", zap.Error(err))
		rndCommitter = service.NewDefaultRandomnessCommitter(
			service.NewRandomnessCommitterConfig(cfg.Common.NumPubRand, int64(cfg.Common.TimestampingDelayBlocks), cfg.Common.ContextSigningHeight),
			service.NewPubRandState(pubRandStore),
			consumerCon,
			em,
			logger,
			fpMetrics,
		)
	} else if contractConfig.FinalitySignatureInterval > 1 {
		// Use RollupRandomnessCommitter for sparse generation
		logger.Info("Using RollupRandomnessCommitter with sparse generation",
			zap.Uint64("finality_signature_interval", contractConfig.FinalitySignatureInterval))

		rndCommitter = service.NewRollupRandomnessCommitter(
			service.NewRandomnessCommitterConfig(cfg.Common.NumPubRand, int64(cfg.Common.TimestampingDelayBlocks), cfg.Common.ContextSigningHeight),
			service.NewPubRandState(pubRandStore),
			consumerCon,
			em,
			logger,
			fpMetrics,
			contractConfig.FinalitySignatureInterval,
		)
	} else {
		// Use default committer for interval=1 (vote every block)
		logger.Info("Using DefaultRandomnessCommitter for consecutive generation",
			zap.Uint64("finality_signature_interval", contractConfig.FinalitySignatureInterval))

		rndCommitter = service.NewDefaultRandomnessCommitter(
			service.NewRandomnessCommitterConfig(cfg.Common.NumPubRand, int64(cfg.Common.TimestampingDelayBlocks), cfg.Common.ContextSigningHeight),
			service.NewPubRandState(pubRandStore),
			consumerCon,
			em,
			logger,
			fpMetrics,
		)
	}

	heightDeterminer := service.NewStartHeightDeterminer(consumerCon, cfg.Common.PollerConfig, logger)
	finalitySubmitter := service.NewDefaultFinalitySubmitter(consumerCon,
		em,
		rndCommitter.GetPubRandProofList,
		service.NewDefaultFinalitySubmitterConfig(cfg.Common.MaxSubmissionRetries,
			cfg.Common.ContextSigningHeight,
			cfg.Common.SubmissionRetryInterval),
		logger,
		fpMetrics,
	)

	fpApp, err := service.NewFinalityProviderApp(cfg.Common, cc, consumerCon, em, poller, rndCommitter, heightDeterminer, finalitySubmitter, fpMetrics, db, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create finality provider app: %w", err)
	}

	return fpApp, nil
}

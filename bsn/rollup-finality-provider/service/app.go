package service

import (
	"fmt"

	rollupcc "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/clientcontroller"
	rollupcfg "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/config"
	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/lightningnetwork/lnd/kvdb"
	"go.uber.org/zap"
)

// NewRollupFinalityProviderAppFromConfig creates a new rollup FinalityProviderApp instance from the given configuration.
func NewRollupFinalityProviderAppFromConfig(
	cfg *rollupcfg.RollupFPConig,
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

	consumerCon, err := rollupcc.NewRollupBSNController(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create rpc client for the consumer chain %s: %w", cfg.Common.ChainType, err)
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

	return service.NewFinalityProviderApp(cfg.Common, cc, consumerCon, em, poller, fpMetrics, db, logger)
}

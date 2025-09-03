package service

import (
	"context"
	"fmt"

	"github.com/avast/retry-go/v4"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	ccapi "github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/types"
	"go.uber.org/zap"
)

var _ types.HeightDeterminer = (*StartHeightDeterminer)(nil)

// StartHeightDeterminer is responsible for determining the appropriate starting height for block processing.
// It uses configuration and consumer chain status to decide between static or automatic chain scanning modes.
type StartHeightDeterminer struct {
	btcPk       *bbntypes.BIP340PubKey
	consumerCon ccapi.ConsumerController
	cfg         *config.ChainPollerConfig
	logger      *zap.Logger
}

func NewStartHeightDeterminer(consumerCon ccapi.ConsumerController, cfg *config.ChainPollerConfig, logger *zap.Logger) *StartHeightDeterminer {
	return &StartHeightDeterminer{
		consumerCon: consumerCon,
		cfg:         cfg,
		logger:      logger,
	}
}

// DetermineStartHeight determines start height for block processing by:
//
// If AutoChainScanningMode is disabled:
//   - Returns StaticChainScanningStartHeight from config
//
// If AutoChainScanningMode is enabled:
//   - Gets finalityActivationHeight from chain
//   - Gets lastFinalizedHeight from chain
//   - Gets lastVotedHeight from local state
//   - Gets highestVotedHeight from chain
//   - Sets startHeight = max(lastVotedHeight, highestVotedHeight, lastFinalizedHeight) + 1
//   - Returns max(startHeight, finalityActivationHeight) to ensure startHeight is not
//     lower than the finality activation height
//
// This ensures that:
// 1. The FP will not vote for heights below the finality activation height
// 2. The FP will resume from its last voting position or the chain's last finalized height
// 3. The FP will not process blocks it has already voted on
//
// Note: Starting from the lastFinalizedHeight when there's a gap to the last processed height
// may result in missed rewards, depending on the consumer chain's reward distribution mechanism.
func (bt *StartHeightDeterminer) DetermineStartHeight(
	ctx context.Context,
	btcPk *bbntypes.BIP340PubKey,
	lastVotedHeightFunc types.LastVotedHeightProvider,
) (uint64, error) {
	if btcPk == nil {
		return 0, fmt.Errorf("BIP340 public key cannot be nil")
	}
	if lastVotedHeightFunc == nil {
		return 0, fmt.Errorf("lastVotedHeightFunc cannot be nil")
	}

	bt.btcPk = btcPk

	// start from a height from config if AutoChainScanningMode is disabled
	if !bt.cfg.AutoChainScanningMode {
		bt.logger.Info("using static chain scanning mode",
			zap.String("pk", bt.btcPk.MarshalHex()),
			zap.Uint64("start_height", bt.cfg.StaticChainScanningStartHeight))

		return bt.cfg.StaticChainScanningStartHeight, nil
	}

	highestVotedHeight, err := bt.highestVotedHeightWithRetry(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get the highest voted height: %w", err)
	}

	lastFinalizedHeight, err := bt.latestFinalizedHeightWithRetry(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get the last finalized height: %w", err)
	}

	// Determine start height to be the max height among local last-voted height, highest voted height
	// from Babylon, and the last finalized height
	// NOTE: if highestVotedHeight is selected, it could lead to issues when there are missed blocks between
	// the gap due to bugs. A potential solution is to check if the fp has voted for each block within
	// the gap. This issue is not critical if we can assume the votes are sent in the monotonically
	// increasing order.
	lastVotedHeight, err := lastVotedHeightFunc()
	if err != nil {
		return 0, fmt.Errorf("failed to get last voted height: %w", err)
	}
	startHeight := max(lastVotedHeight, highestVotedHeight, lastFinalizedHeight) + 1

	finalityActivationHeight, err := bt.getFinalityActivationHeightWithRetry(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get finality activation height: %w", err)
	}

	// ensure start height is not lower than the finality activation height
	startHeight = max(startHeight, finalityActivationHeight)

	bt.logger.Info("determined poller starting height",
		zap.String("pk", bt.btcPk.MarshalHex()),
		zap.Uint64("start_height", lastFinalizedHeight),
		zap.Uint64("finality_activation_height", finalityActivationHeight),
		zap.Uint64("last_voted_height", lastVotedHeight),
		zap.Uint64("last_finalized_height", lastFinalizedHeight),
		zap.Uint64("highest_voted_height", highestVotedHeight))

	return lastFinalizedHeight, nil
}

func (bt *StartHeightDeterminer) highestVotedHeightWithRetry(ctx context.Context) (uint64, error) {
	var height uint64
	if err := retry.Do(func() error {
		h, err := bt.consumerCon.QueryFinalityProviderHighestVotedHeight(ctx, bt.btcPk.MustToBTCPK())
		if err != nil {
			return fmt.Errorf("failed to query the highest voted height: %w", err)
		}
		height = h

		return nil
	}, retry.Context(ctx), RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		bt.logger.Debug(
			"failed to query babylon for the highest voted height",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return 0, fmt.Errorf("failed to query the highest voted height: %w", err)
	}

	return height, nil
}

func (bt *StartHeightDeterminer) latestFinalizedHeightWithRetry(ctx context.Context) (uint64, error) {
	var height uint64
	if err := retry.Do(func() error {
		block, err := bt.consumerCon.QueryLatestFinalizedBlock(ctx)
		if err != nil {
			return fmt.Errorf("failed to query the latest finalized height: %w", err)
		}
		if block == nil {
			// no finalized block yet
			return nil
		}
		height = block.GetHeight()

		return nil
	}, retry.Context(ctx), RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		bt.logger.Debug(
			"failed to query babylon for the latest finalised height",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return 0, fmt.Errorf("failed to query the latest finalized height: %w", err)
	}

	return height, nil
}

func (bt *StartHeightDeterminer) getFinalityActivationHeightWithRetry(ctx context.Context) (uint64, error) {
	var response uint64
	if err := retry.Do(func() error {
		finalityActivationHeight, err := bt.consumerCon.QueryFinalityActivationBlockHeight(ctx)
		if err != nil {
			return fmt.Errorf("failed to query the finality activation height: %w", err)
		}
		response = finalityActivationHeight

		return nil
	}, retry.Context(ctx), RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		bt.logger.Debug(
			"failed to query babylon for the finality activation height",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return 0, fmt.Errorf("failed to query the finality activation height: %w", err)
	}

	return response, nil
}

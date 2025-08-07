package service

import (
	"context"
	"fmt"

	ccapi "github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	service "github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"go.uber.org/zap"
)

// RollupRandomnessCommitter is a randomness committer for rollup FPs that supports sparse generation
// It generates randomness only for heights where the FP will vote (based on finality signature interval)
type RollupRandomnessCommitter struct {
	*service.DefaultRandomnessCommitter
	interval uint64
}

func NewRollupRandomnessCommitter(
	cfg *service.RandomnessCommitterConfig,
	pubRandState *service.PubRandState,
	consumerCon ccapi.ConsumerController,
	em eotsmanager.EOTSManager,
	logger *zap.Logger,
	metrics *metrics.FpMetrics,
	interval uint64,
) *RollupRandomnessCommitter {
	return &RollupRandomnessCommitter{
		DefaultRandomnessCommitter: service.NewDefaultRandomnessCommitter(cfg, pubRandState, consumerCon, em, logger, metrics),
		interval:                   interval,
	}
}

// ShouldCommit overrides the default implementation with rollup-specific logic
// that directly calculates aligned startHeight without redundant parent calls
func (rrc *RollupRandomnessCommitter) ShouldCommit(ctx context.Context) (bool, uint64, error) {
	// Get last committed height (same as parent)
	lastCommittedHeight, err := rrc.GetLastCommittedHeight(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("failed to get last committed height: %w", err)
	}

	// Get current tip height (same as parent)
	tipBlock, err := rrc.ConsumerCon.QueryLatestBlock(ctx)
	if tipBlock == nil || err != nil {
		return false, 0, fmt.Errorf("failed to get the last block: %w", err)
	}

	if rrc.Cfg.TimestampingDelayBlocks < 0 {
		return false, 0, fmt.Errorf("TimestampingDelayBlocks cannot be negative: %d", rrc.Cfg.TimestampingDelayBlocks)
	}

	// Get activation height first for interval-aware calculations
	activationBlkHeight, err := rrc.ConsumerCon.QueryFinalityActivationBlockHeight(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("failed to query finality activation block height: %w", err)
	}

	// Calculate tip height with delay
	tipHeightWithDelay := tipBlock.GetHeight() + uint64(rrc.Cfg.TimestampingDelayBlocks) // #nosec G115

	// ROLLUP-SPECIFIC: Determine startHeight with interval awareness from the beginning
	var alignedStartHeight uint64
	switch {
	case lastCommittedHeight < tipHeightWithDelay:
		// Need to start from tipHeightWithDelay, but align it to voting schedule
		baseHeight := max(tipHeightWithDelay, activationBlkHeight)
		alignedStartHeight = rrc.calculateFirstEligibleHeightWithActivation(baseHeight, activationBlkHeight)

	case rrc.needsMoreVotingRandomness(lastCommittedHeight, tipHeightWithDelay, activationBlkHeight):
		// Need to continue from where we left off, but with interval spacing
		// For sparse generation, we need to check if we have enough *voting* heights covered
		baseHeight := max(lastCommittedHeight+1, activationBlkHeight)
		alignedStartHeight = rrc.calculateFirstEligibleHeightWithActivation(baseHeight, activationBlkHeight)

	default:
		// Check if we have sufficient voting randomness, not just any randomness
		// Calculate the last voting height we have randomness for
		lastVotingHeight := rrc.getLastVotingHeightWithRandomness(lastCommittedHeight, activationBlkHeight)
		requiredVotingHeight := tipHeightWithDelay + uint64(rrc.Cfg.NumPubRand)*rrc.interval

		if lastVotingHeight >= requiredVotingHeight {
			// Sufficient voting randomness, no need to commit
			rrc.Logger.Debug(
				"the rollup finality-provider has sufficient voting randomness, skip committing more",
				zap.String("pk", rrc.BtcPk.MarshalHex()),
				zap.Uint64("tip_height", tipBlock.GetHeight()),
				zap.Uint64("last_committed_height", lastCommittedHeight),
				zap.Uint64("last_voting_height", lastVotingHeight),
				zap.Uint64("required_voting_height", requiredVotingHeight),
			)

			return false, 0, nil
		}

		// Need more voting randomness
		baseHeight := max(lastCommittedHeight+1, activationBlkHeight)
		alignedStartHeight = rrc.calculateFirstEligibleHeightWithActivation(baseHeight, activationBlkHeight)
	}

	rrc.Logger.Debug(
		"the rollup finality-provider should commit randomness",
		zap.String("pk", rrc.BtcPk.MarshalHex()),
		zap.Uint64("tip_height", tipBlock.GetHeight()),
		zap.Uint64("last_committed_height", lastCommittedHeight),
		zap.Uint64("aligned_start_height", alignedStartHeight),
		zap.Uint64("interval", rrc.interval),
	)

	return true, alignedStartHeight, nil
}

// needsMoreVotingRandomness determines if we need more voting randomness
// by checking if our last committed voting height can cover the required buffer
func (rrc *RollupRandomnessCommitter) needsMoreVotingRandomness(lastCommittedHeight, tipHeightWithDelay, activationHeight uint64) bool {
	// Find the last voting height we have randomness for
	lastVotingHeight := rrc.getLastVotingHeightWithRandomness(lastCommittedHeight, activationHeight)

	// Find the next voting height we need to cover after tipHeightWithDelay
	nextRequiredVotingHeight := rrc.calculateFirstEligibleHeightWithActivation(tipHeightWithDelay, activationHeight)

	// We need randomness for NumPubRand voting heights starting from nextRequiredVotingHeight
	requiredVotingHeight := nextRequiredVotingHeight + (uint64(rrc.Cfg.NumPubRand)-1)*rrc.interval

	return lastVotingHeight < requiredVotingHeight
}

// getLastVotingHeightWithRandomness calculates the last voting height for which we have randomness
// based on the lastCommittedHeight and interval spacing
func (rrc *RollupRandomnessCommitter) getLastVotingHeightWithRandomness(lastCommittedHeight, activationHeight uint64) uint64 {
	if lastCommittedHeight < activationHeight {
		return 0 // No voting randomness yet
	}

	// For sparse generation, we need to find the last voting height <= lastCommittedHeight
	// that aligns with the voting schedule
	offset := lastCommittedHeight - activationHeight
	votingHeightIndex := offset / rrc.interval

	return activationHeight + votingHeightIndex*rrc.interval
}

// getPubRandList overrides the default implementation to use sparse generation
// startHeight is already aligned by ShouldCommit, so we can use it directly
func (rrc *RollupRandomnessCommitter) getPubRandList(startHeight uint64, numPubRand uint32) ([]*btcec.FieldVal, error) {
	pubRandList, err := rrc.Em.CreateRandomnessPairListWithInterval(
		rrc.BtcPk.MustMarshal(),
		rrc.Cfg.ChainID,
		startHeight, // Already aligned by ShouldCommit
		numPubRand,
		rrc.interval,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create randomness pair list with interval: %w", err)
	}

	return pubRandList, nil
}

// calculateFirstEligibleHeightWithActivation finds the first height >= startHeight that is eligible for voting
// using the provided activation height (avoids redundant contract queries)
func (rrc *RollupRandomnessCommitter) calculateFirstEligibleHeightWithActivation(startHeight, activationHeight uint64) uint64 {
	// If startHeight is before activation, first eligible is activation height
	if startHeight <= activationHeight {
		return activationHeight
	}

	// Calculate the first eligible height at or after startHeight
	// Formula: activationHeight + n*interval where n is chosen so result >= startHeight
	offset := startHeight - activationHeight
	remainder := offset % rrc.interval

	if remainder == 0 {
		// startHeight is already aligned
		return startHeight
	}

	// Round up to next aligned height
	return startHeight + (rrc.interval - remainder)
}

// Commit overrides the default implementation to use interval-aware storage
// startHeight is already aligned by ShouldCommit, so we can use it directly
func (rrc *RollupRandomnessCommitter) Commit(ctx context.Context, startHeight uint64) (*types.TxResponse, error) {
	// Generate sparse randomness aligned with voting schedule
	// startHeight is already aligned by ShouldCommit
	pubRandList, err := rrc.getPubRandList(startHeight, rrc.Cfg.NumPubRand)
	if err != nil {
		return nil, fmt.Errorf("failed to generate randomness: %w", err)
	}
	numPubRand := uint64(len(pubRandList))

	// Generate commitment and proof for each public randomness (same as default)
	commitment, proofList := types.GetPubRandCommitAndProofs(pubRandList)

	// Store them to database with interval-aware keys
	// startHeight is already aligned, so proofs are stored at the correct voting heights
	if err := rrc.AddPubRandProofListWithInterval(
		startHeight, // Already aligned by ShouldCommit
		uint64(rrc.Cfg.NumPubRand),
		proofList,
		rrc.interval,
	); err != nil {
		return nil, fmt.Errorf("failed to save public randomness to DB: %w", err)
	}

	// Sign the commitment using the aligned startHeight
	schnorrSig, err := rrc.SignPubRandCommit(startHeight, numPubRand, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the Schnorr signature: %w", err)
	}

	// Submit to consumer chain using the aligned startHeight
	res, err := rrc.ConsumerCon.CommitPubRandList(ctx, &ccapi.CommitPubRandListRequest{
		FpPk:        rrc.BtcPk.MustToBTCPK(),
		StartHeight: startHeight,
		NumPubRand:  numPubRand,
		Commitment:  commitment,
		Sig:         schnorrSig,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to commit public randomness to the consumer chain: %w", err)
	}

	// Update metrics using aligned heights
	rrc.Metrics.RecordFpRandomnessTime(rrc.BtcPk.MarshalHex())
	// For sparse generation, the last height is startHeight + (numPubRand-1)*interval
	lastHeight := startHeight + (numPubRand-1)*rrc.interval
	rrc.Metrics.RecordFpLastCommittedRandomnessHeight(rrc.BtcPk.MarshalHex(), lastHeight)
	rrc.Metrics.AddToFpTotalCommittedRandomness(rrc.BtcPk.MarshalHex(), float64(len(pubRandList)))

	return res, nil
}

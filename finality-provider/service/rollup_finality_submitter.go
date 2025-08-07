package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"go.uber.org/zap"
)

// RollupFinalitySubmitter is a finality submitter for rollup FPs that uses sparse randomness generation
// It generates randomness only for heights where the FP will vote (based on finality signature interval)
type RollupFinalitySubmitter struct {
	*DefaultFinalitySubmitter
	interval uint64
}

func NewRollupFinalitySubmitter(
	consumerCtrl api.ConsumerController,
	em eotsmanager.EOTSManager,
	proofListGetterFunc PubRandProofListGetterFunc,
	cfg *FinalitySubmitterConfig,
	logger *zap.Logger,
	metrics *metrics.FpMetrics,
	interval uint64,
) *RollupFinalitySubmitter {
	return &RollupFinalitySubmitter{
		DefaultFinalitySubmitter: NewDefaultFinalitySubmitter(consumerCtrl, em, proofListGetterFunc, cfg, logger, metrics),
		interval:                 interval,
	}
}

// GetPubRandList overrides the default implementation to use sparse generation
// This ensures the randomness retrieval matches the sparse commitment pattern
func (rfs *RollupFinalitySubmitter) GetPubRandList(startHeight uint64, numPubRand uint32) ([]*btcec.FieldVal, error) {
	pubRandList, err := rfs.Em.CreateRandomnessPairListWithInterval(
		rfs.getBtcPkBIP340().MustMarshal(),
		rfs.State.GetChainID(),
		startHeight,
		numPubRand,
		rfs.interval,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create sparse public randomness list: %w", err)
	}

	return pubRandList, nil
}

// SubmitBatchFinalitySignatures overrides the default implementation to ensure
// our sparse GetPubRandList method is called throughout the submission process
func (rfs *RollupFinalitySubmitter) SubmitBatchFinalitySignatures(ctx context.Context, blocks []types.BlockDescription) (*types.TxResponse, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("cannot send signatures for empty blocks")
	}

	blocks, err := rfs.filterBlocksForVoting(ctx, blocks)
	if err != nil {
		return nil, fmt.Errorf("failed to filter blocks for voting: %w", err)
	}

	if len(blocks) == 0 {
		rfs.Logger.Debug(
			"no blocks to vote for after filtering",
			zap.String("pk", rfs.getBtcPkHex()),
			zap.Uint64("last_voted_height", rfs.State.GetLastVotedHeight()),
		)

		return nil, nil // No blocks to vote for
	}

	var failedCycles uint32
	targetHeight := blocks[len(blocks)-1].GetHeight()

	// Retry loop with internal retry logic (copied from DefaultFinalitySubmitter)
	for {
		res, err := rfs.submitBatchFinalitySignaturesOnce(ctx, blocks)
		if err != nil {
			rfs.Logger.Debug(
				"failed to submit finality signature to the consumer chain",
				zap.String("pk", rfs.getBtcPkHex()),
				zap.Uint32("current_failures", failedCycles),
				zap.Uint64("target_start_height", blocks[0].GetHeight()),
				zap.Uint64("target_end_height", targetHeight),
				zap.Error(err),
			)

			// Handle different error types
			if fpcc.IsUnrecoverable(err) {
				return nil, err
			}

			if fpcc.IsExpected(err) {
				return nil, nil
			}

			failedCycles++
			if failedCycles > rfs.Cfg.MaxSubmissionRetries {
				return nil, fmt.Errorf("reached max failed cycles with err: %w", err)
			}
		} else {
			// The signature has been successfully submitted
			return res, nil
		}

		// Check if the block is already finalized
		finalized, err := rfs.checkBlockFinalization(ctx, targetHeight)
		if err != nil {
			return nil, fmt.Errorf("failed to query block finalization at height %v: %w", targetHeight, err)
		}
		if finalized {
			rfs.Logger.Debug(
				"the block is already finalized, skip submission",
				zap.String("pk", rfs.getBtcPkHex()),
				zap.Uint64("target_height", targetHeight),
			)

			rfs.Metrics.IncrementFpTotalFailedVotes(rfs.getBtcPkHex())

			return nil, nil
		}

		// Wait for the retry interval
		select {
		case <-time.After(rfs.Cfg.SubmissionRetryInterval):
			// Continue to next retry iteration
			continue
		case <-ctx.Done():
			rfs.Logger.Debug("the finality-provider instance is closing", zap.String("pk", rfs.getBtcPkHex()))

			return nil, ErrFinalityProviderShutDown
		}
	}
}

// submitBatchFinalitySignaturesOnce overrides to ensure our GetPubRandList method is called
func (rfs *RollupFinalitySubmitter) submitBatchFinalitySignaturesOnce(ctx context.Context, blocks []types.BlockDescription) (*types.TxResponse, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("should not submit batch finality signature with zero block")
	}

	// Get proofs and public randomness for each block
	var proofBytesList [][]byte
	var prList []*btcec.FieldVal

	for _, block := range blocks {
		// Get public randomness for this specific height using sparse generation method
		// We request exactly 1 randomness value since we need randomness for this single block height
		pr, err := rfs.GetPubRandList(block.GetHeight(), 1)
		if err != nil {
			return nil, fmt.Errorf("failed to get public randomness for height %d: %w", block.GetHeight(), err)
		}
		prList = append(prList, pr[0])

		// Get proof for this specific height
		proofs, err := rfs.ProofListGetterFunc(block.GetHeight(), 1)
		if err != nil {
			return nil, fmt.Errorf("failed to get public randomness inclusion proof for height %d: %w\nplease recover the randomness proof from db", block.GetHeight(), err)
		}
		if len(proofs) != 1 {
			return nil, fmt.Errorf("expected exactly one proof for height %d, got %d", block.GetHeight(), len(proofs))
		}
		proofBytesList = append(proofBytesList, proofs[0])
	}

	// Process each block and collect only valid items
	var validBlocks []types.BlockDescription
	var validPrList []*btcec.FieldVal
	var validProofList [][]byte
	var validSigList []*btcec.ModNScalar

	for i, b := range blocks {
		eotsSig, err := rfs.signFinalitySig(b)
		if err != nil {
			if !errors.Is(err, ErrFailedPrecondition) {
				return nil, err
			}
			// Skip this block if we encounter FailedPrecondition
			rfs.Logger.Warn("encountered FailedPrecondition error, skipping block",
				zap.Uint64("height", b.GetHeight()),
				zap.String("hash", hex.EncodeToString(b.GetHash())),
				zap.Error(err))

			continue
		}

		// If signature is valid, append all corresponding items
		validBlocks = append(validBlocks, b)
		validPrList = append(validPrList, prList[i])
		validProofList = append(validProofList, proofBytesList[i])
		validSigList = append(validSigList, eotsSig.ToModNScalar())
	}

	// If all blocks were skipped, return early
	if len(validBlocks) == 0 {
		rfs.Logger.Info("all blocks were skipped due to double sign errors")

		return nil, nil
	}

	// Send finality signature to the consumer chain
	res, err := rfs.ConsumerCtrl.SubmitBatchFinalitySigs(ctx, api.NewSubmitBatchFinalitySigsRequest(
		rfs.getBtcPk(),
		validBlocks,
		validPrList,
		validProofList,
		validSigList,
	))

	if err != nil {
		if strings.Contains(err.Error(), "jailed") {
			return nil, ErrFinalityProviderJailed
		}
		if strings.Contains(err.Error(), "slashed") {
			return nil, ErrFinalityProviderSlashed
		}

		return nil, fmt.Errorf("failed to submit finality signature to the consumer chain: %w", err)
	}

	// Update the metrics with voted blocks
	for _, b := range validBlocks {
		rfs.Metrics.RecordFpVotedHeight(rfs.getBtcPkHex(), b.GetHeight())
	}

	// Update state with the highest height of this batch
	highBlock := blocks[len(blocks)-1]
	rfs.mustSetLastVotedHeight(highBlock.GetHeight())

	return res, nil
}

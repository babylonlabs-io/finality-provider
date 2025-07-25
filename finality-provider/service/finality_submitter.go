package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/avast/retry-go/v4"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"go.uber.org/zap"
	"math"
	"strings"
	"time"
)

var _ types.FinalitySignatureSubmitter = (*DefaultFinalitySubmitter)(nil)

type PubRandProofListGetterFunc func(startHeight uint64, numPubRand uint64) ([][]byte, error)

type DefaultFinalitySubmitter struct {
	state               types.FinalityProviderState
	em                  eotsmanager.EOTSManager
	consumerCtrl        api.ConsumerController
	proofListGetterFunc PubRandProofListGetterFunc
	cfg                 *FinalitySubmitterConfig
	logger              *zap.Logger
	metrics             *metrics.FpMetrics
}

type FinalitySubmitterConfig struct {
	MaxSubmissionRetries    uint32
	ContextSigningHeight    uint64
	SubmissionRetryInterval time.Duration
}

func NewDefaultFinalitySubmitterConfig(
	maxSubmissionRetries uint32,
	contextSigningHeight uint64,
	submissionRetryInterval time.Duration,
) *FinalitySubmitterConfig {
	return &FinalitySubmitterConfig{
		MaxSubmissionRetries:    maxSubmissionRetries,
		SubmissionRetryInterval: submissionRetryInterval,
		ContextSigningHeight:    contextSigningHeight,
	}
}

func NewDefaultFinalitySubmitter(
	consumerCtrl api.ConsumerController,
	em eotsmanager.EOTSManager,
	proofListGetterFunc PubRandProofListGetterFunc,
	cfg *FinalitySubmitterConfig,
	logger *zap.Logger,
	metrics *metrics.FpMetrics) *DefaultFinalitySubmitter {
	return &DefaultFinalitySubmitter{
		em:                  em,
		consumerCtrl:        consumerCtrl,
		proofListGetterFunc: proofListGetterFunc,
		cfg:                 cfg,
		logger:              logger.With(zap.String("module", "finality_submitter")),
		metrics:             metrics,
	}
}

func (ds *DefaultFinalitySubmitter) getBtcPkHex() string {
	return ds.getBtcPkBIP340().MarshalHex()
}

func (ds *DefaultFinalitySubmitter) getBtcPk() *btcec.PublicKey {
	return ds.state.GetBtcPk()
}

func (ds *DefaultFinalitySubmitter) getBtcPkBIP340() *bbntypes.BIP340PubKey {
	return ds.state.GetBtcPkBIP340()
}

func (ds *DefaultFinalitySubmitter) mustSetLastVotedHeight(height uint64) {
	if err := ds.state.SetLastVotedHeight(height); err != nil {
		ds.logger.Fatal("failed to update state after finality signature submitted",
			zap.String("pk", ds.getBtcPkHex()), zap.Uint64("height", height), zap.Error(err))
	}
}

func (ds *DefaultFinalitySubmitter) mustSetStatus(s proto.FinalityProviderStatus) {
	if err := ds.state.SetStatus(s); err != nil {
		ds.logger.Fatal("failed to set finality-provider status",
			zap.String("pk", ds.getBtcPkHex()), zap.String("status", s.String()))
	}
}

// InitState sets the finality provider state store.
func (ds *DefaultFinalitySubmitter) InitState(state types.FinalityProviderState) error {
	if ds.state != nil {
		return fmt.Errorf("finality provider state is already set")
	}

	ds.state = state

	return nil
}

// filterBlocksForVoting filters blocks based on the finality provider's voting power and height criteria for submission.
// It returns a slice of blocks eligible for voting and an error if any issues are encountered during processing.
// It also updates the fp instance status according to the block's voting power
func (ds *DefaultFinalitySubmitter) filterBlocksForVoting(ctx context.Context, blocks []types.BlockDescription) ([]types.BlockDescription, error) {
	processedBlocks := make([]types.BlockDescription, 0, len(blocks))

	var hasPower bool
	var err error
	for _, b := range blocks {
		blk := b
		if blk.GetHeight() <= ds.state.GetLastVotedHeight() {
			ds.logger.Debug(
				"the block height is lower than last processed height",
				zap.String("pk", ds.getBtcPkHex()),
				zap.Uint64("block_height", blk.GetHeight()),
				zap.Uint64("last_voted_height", ds.state.GetLastVotedHeight()),
			)

			continue
		}

		// check whether the finality provider has voting power
		hasPower, err = ds.getVotingPowerWithRetry(ctx, blk.GetHeight())
		if err != nil {
			return nil, fmt.Errorf("failed to get voting power for height %d: %w", blk.GetHeight(), err)
		}
		if !hasPower {
			ds.logger.Debug(
				"the finality-provider does not have voting power",
				zap.String("pk", ds.getBtcPkHex()),
				zap.Uint64("block_height", blk.GetHeight()),
			)

			// the finality provider does not have voting power
			// and it will never will at this block, so continue
			ds.metrics.IncrementFpTotalBlocksWithoutVotingPower(ds.getBtcPkHex())

			continue
		}

		processedBlocks = append(processedBlocks, blk)
	}

	// update fp status according to the power for the last block
	if hasPower && ds.state.GetStatus() != proto.FinalityProviderStatus_ACTIVE {
		ds.mustSetStatus(proto.FinalityProviderStatus_ACTIVE)
	}

	if !hasPower && ds.state.GetStatus() == proto.FinalityProviderStatus_ACTIVE {
		ds.mustSetStatus(proto.FinalityProviderStatus_INACTIVE)
	}

	return processedBlocks, nil
}

// SubmitBatchFinalitySignatures builds and sends a finality signature over the given block to the consumer chain
// Contract:
//  1. the input blocks should be in the ascending order of height
//  2. the returned response could be nil due to no transactions might be made in the end
func (ds *DefaultFinalitySubmitter) SubmitBatchFinalitySignatures(ctx context.Context, blocks []types.BlockDescription) (*types.TxResponse, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("cannot send signatures for empty blocks")
	}

	blocks, err := ds.filterBlocksForVoting(ctx, blocks)
	if err != nil {
		return nil, fmt.Errorf("failed to filter blocks for voting: %w", err)
	}

	if len(blocks) == 0 {
		ds.logger.Debug(
			"no blocks to vote for after filtering",
			zap.String("pk", ds.getBtcPkHex()),
			zap.Uint64("last_voted_height", ds.state.GetLastVotedHeight()),
		)

		return nil, nil // No blocks to vote for
	}

	var failedCycles uint32
	targetHeight := blocks[len(blocks)-1].GetHeight()

	// Retry loop with internal retry logic
	for {
		// Attempt submission
		res, err := ds.submitBatchFinalitySignaturesOnce(ctx, blocks)
		if err != nil {
			ds.logger.Debug(
				"failed to submit finality signature to the consumer chain",
				zap.String("pk", ds.getBtcPkHex()),
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
			if failedCycles > ds.cfg.MaxSubmissionRetries {
				return nil, fmt.Errorf("reached max failed cycles with err: %w", err)
			}
		} else {
			// The signature has been successfully submitted
			return res, nil
		}

		// Check if the block is already finalized
		finalized, err := ds.checkBlockFinalization(ctx, targetHeight)
		if err != nil {
			return nil, fmt.Errorf("failed to query block finalization at height %v: %w", targetHeight, err)
		}
		if finalized {
			ds.logger.Debug(
				"the block is already finalized, skip submission",
				zap.String("pk", ds.getBtcPkHex()),
				zap.Uint64("target_height", targetHeight),
			)

			ds.metrics.IncrementFpTotalFailedVotes(ds.getBtcPkHex())

			return nil, nil
		}

		// Wait for the retry interval
		select {
		case <-time.After(ds.cfg.SubmissionRetryInterval):
			// Continue to next retry iteration
			continue
		case <-ctx.Done():
			ds.logger.Debug("the finality-provider instance is closing", zap.String("pk", ds.getBtcPkHex()))

			return nil, ErrFinalityProviderShutDown
		}
	}
}

// submitBatchFinalitySignaturesOnce performs a single submission attempt (original SubmitBatchFinalitySignatures logic)
func (ds *DefaultFinalitySubmitter) submitBatchFinalitySignaturesOnce(ctx context.Context, blocks []types.BlockDescription) (*types.TxResponse, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("should not submit batch finality signature with zero block")
	}

	if len(blocks) > math.MaxUint32 {
		return nil, fmt.Errorf("should not submit batch finality signature with too many blocks")
	}

	// get public randomness list
	numPubRand := len(blocks)
	// #nosec G115 -- performed the conversion check above
	prList, err := ds.GetPubRandList(blocks[0].GetHeight(), uint32(numPubRand))
	if err != nil {
		return nil, fmt.Errorf("failed to get public randomness list: %w", err)
	}

	// get proof list
	proofBytesList, err := ds.proofListGetterFunc(
		blocks[0].GetHeight(),
		uint64(numPubRand),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get public randomness inclusion proof list: %w\nplease recover the randomness proof from db", err)
	}

	// Create slices to store only the valid items
	validBlocks := make([]types.BlockDescription, 0, len(blocks))
	validPrList := make([]*btcec.FieldVal, 0, len(blocks))
	validProofList := make([][]byte, 0, len(blocks))
	validSigList := make([]*btcec.ModNScalar, 0, len(blocks))

	// Process each block and collect only valid items
	for i, b := range blocks {
		eotsSig, err := ds.signFinalitySig(b)
		if err != nil {
			if !errors.Is(err, ErrFailedPrecondition) {
				return nil, err
			}
			// Skip this block if we encounter FailedPrecondition
			ds.logger.Warn("encountered FailedPrecondition error, skipping block",
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
		ds.logger.Info("all blocks were skipped due to double sign errors")

		return nil, nil
	}

	// send finality signature to the consumer chain
	res, err := ds.consumerCtrl.SubmitBatchFinalitySigs(ctx, api.NewSubmitBatchFinalitySigsRequest(
		ds.getBtcPk(),
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

	// update the metrics with voted blocks
	for _, b := range validBlocks {
		ds.metrics.RecordFpVotedHeight(ds.getBtcPkHex(), b.GetHeight())
	}

	// update state with the highest height of this batch
	highBlock := blocks[len(blocks)-1]
	ds.mustSetLastVotedHeight(highBlock.GetHeight())

	return res, nil
}

// checkBlockFinalization checks if a block at given height is finalized
func (ds *DefaultFinalitySubmitter) checkBlockFinalization(ctx context.Context, height uint64) (bool, error) {
	b, err := ds.consumerCtrl.QueryBlock(ctx, height)
	if err != nil {
		return false, fmt.Errorf("failed to query block at height %d: %w", height, err)
	}

	return b.IsFinalized(), nil
}

func (ds *DefaultFinalitySubmitter) signFinalitySig(b types.BlockDescription) (*bbntypes.SchnorrEOTSSig, error) {
	// build proper finality signature request
	var msgToSign []byte
	if ds.cfg.ContextSigningHeight > b.GetHeight() {
		signCtx := ds.consumerCtrl.GetFpFinVoteContext()
		msgToSign = b.MsgToSign(signCtx)
	} else {
		msgToSign = b.MsgToSign("")
	}

	sig, err := ds.em.SignEOTS(ds.getBtcPkBIP340().MustMarshal(), ds.state.GetChainID(), msgToSign, b.GetHeight())
	if err != nil {
		if strings.Contains(err.Error(), failedPreconditionErrStr) {
			return nil, ErrFailedPrecondition
		}

		return nil, fmt.Errorf("failed to sign EOTS: %w", err)
	}

	return bbntypes.NewSchnorrEOTSSigFromModNScalar(sig), nil
}

func (ds *DefaultFinalitySubmitter) GetPubRandList(startHeight uint64, numPubRand uint32) ([]*btcec.FieldVal, error) {
	pubRandList, err := ds.em.CreateRandomnessPairList(
		ds.getBtcPkBIP340().MustMarshal(),
		ds.state.GetChainID(),
		startHeight,
		numPubRand,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create public randomness list: %w", err)
	}

	return pubRandList, nil
}

func (ds *DefaultFinalitySubmitter) getVotingPowerWithRetry(ctx context.Context, height uint64) (bool, error) {
	var (
		hasPower bool
		err      error
	)

	if err := retry.Do(func() error {
		hasPower, err = ds.consumerCtrl.QueryFinalityProviderHasPower(ctx, api.NewQueryFinalityProviderHasPowerRequest(
			ds.getBtcPk(),
			height,
		))
		if err != nil {
			return fmt.Errorf("failed to query the voting power: %w", err)
		}

		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		ds.logger.Debug(
			"failed to query the voting power",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return false, fmt.Errorf("failed to query the voting power: %w", err)
	}

	return hasPower, nil
}

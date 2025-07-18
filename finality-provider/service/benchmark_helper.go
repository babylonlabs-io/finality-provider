package service

import (
	"context"
	"fmt"
	"time"

	ccapi "github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/types"
	"go.uber.org/zap"
)

// CommitPubRandTiming - helper struct used to capture times for benchmark
type CommitPubRandTiming struct {
	GetPubRandListTime      time.Duration
	AddPubRandProofListTime time.Duration
	CommitPubRandListTime   time.Duration
}

// HelperCommitPubRand used for benchmark
func (fp *FinalityProviderInstance) HelperCommitPubRand(ctx context.Context, tipHeight uint64) (*types.TxResponse, *CommitPubRandTiming, error) {
	lastCommittedHeight, err := fp.GetLastCommittedHeight(ctx)
	if err != nil {
		return nil, nil, err
	}

	var startHeight uint64
	switch {
	case lastCommittedHeight == uint64(0):
		// the finality-provider has never submitted public rand before
		startHeight = tipHeight + 1
	case lastCommittedHeight < uint64(fp.cfg.TimestampingDelayBlocks)+tipHeight:
		// (should not use subtraction because they are in the type of uint64)
		// we are running out of the randomness
		startHeight = lastCommittedHeight + 1
	default:
		fp.logger.Debug(
			"the finality-provider has sufficient public randomness, skip committing more",
			zap.String("pk", fp.GetBtcPkHex()),
			zap.Uint64("block_height", tipHeight),
			zap.Uint64("last_committed_height", lastCommittedHeight),
		)

		return nil, nil, nil
	}

	return fp.commitPubRandPairsWithTiming(ctx, startHeight)
}

func (fp *FinalityProviderInstance) commitPubRandPairsWithTiming(ctx context.Context, startHeight uint64) (*types.TxResponse, *CommitPubRandTiming, error) {
	timing := &CommitPubRandTiming{}

	activationBlkHeight, err := fp.consumerCon.QueryFinalityActivationBlockHeight(ctx)
	if err != nil {
		return nil, timing, fmt.Errorf("failed to query the finality activation block height: %w", err)
	}

	startHeight = max(startHeight, activationBlkHeight)

	// Measure getPubRandList
	pubRandListStart := time.Now()
	pubRandList, err := fp.GetPubRandList(startHeight, fp.cfg.NumPubRand)
	if err != nil {
		return nil, timing, fmt.Errorf("failed to generate randomness: %w", err)
	}
	timing.GetPubRandListTime = time.Since(pubRandListStart)

	numPubRand := uint64(len(pubRandList))
	commitment, proofList := types.GetPubRandCommitAndProofs(pubRandList)

	// Measure addPubRandProofList
	addProofStart := time.Now()
	if err := fp.pubRandState.addPubRandProofList(fp.GetChainID(), fp.btcPk.MustMarshal(), startHeight, uint64(fp.cfg.NumPubRand), proofList); err != nil {
		return nil, timing, fmt.Errorf("failed to save public randomness to DB: %w", err)
	}
	timing.AddPubRandProofListTime = time.Since(addProofStart)

	// Measure CommitPubRandList
	commitListStart := time.Now()
	schnorrSig, err := fp.SignPubRandCommit(startHeight, numPubRand, commitment)
	if err != nil {
		return nil, timing, fmt.Errorf("failed to sign the Schnorr signature: %w", err)
	}

	res, err := fp.consumerCon.CommitPubRandList(ctx, ccapi.NewCommitPubRandListRequest(
		fp.GetBtcPk(),
		startHeight,
		numPubRand,
		commitment,
		schnorrSig,
	))
	if err != nil {
		return nil, timing, fmt.Errorf("failed to commit public randomness to the consumer chain: %w", err)
	}
	timing.CommitPubRandListTime = time.Since(commitListStart)

	return res, timing, nil
}

package service

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/types"
)

// CommitPubRandTiming - helper struct used to capture times for benchmark
type CommitPubRandTiming struct {
	GetPubRandListTime      time.Duration
	AddPubRandProofListTime time.Duration
	CommitPubRandListTime   time.Duration
}

// HelperCommitPubRand used for benchmark
func (fp *FinalityProviderInstance) HelperCommitPubRand(tipHeight uint64) (*types.TxResponse, *CommitPubRandTiming, error) {
	lastCommittedHeight, err := fp.GetLastCommittedHeight()
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

	return fp.commitPubRandPairsWithTiming(startHeight)
}

func (fp *FinalityProviderInstance) commitPubRandPairsWithTiming(startHeight uint64) (*types.TxResponse, *CommitPubRandTiming, error) {
	timing := &CommitPubRandTiming{}

	activationBlkHeight, err := fp.cc.QueryFinalityActivationBlockHeight()
	if err != nil {
		return nil, timing, err
	}

	startHeight = max(startHeight, activationBlkHeight)

	// Measure getPubRandList
	pubRandListStart := time.Now()
	pubRandList, err := fp.getPubRandList(startHeight, fp.cfg.NumPubRand)
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
	schnorrSig, err := fp.signPubRandCommit(startHeight, numPubRand, commitment)
	if err != nil {
		return nil, timing, fmt.Errorf("failed to sign the Schnorr signature: %w", err)
	}

	res, err := fp.cc.CommitPubRandList(fp.GetBtcPk(), startHeight, numPubRand, commitment, schnorrSig)
	if err != nil {
		return nil, timing, fmt.Errorf("failed to commit public randomness to the consumer chain: %w", err)
	}
	timing.CommitPubRandListTime = time.Since(commitListStart)

	return res, timing, nil
}

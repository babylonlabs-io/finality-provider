package service

import (
	"context"
	"fmt"

	"github.com/avast/retry-go/v4"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	ccapi "github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"go.uber.org/zap"
)

var _ types.RandomnessCommitter = (*DefaultRandomnessCommitter)(nil)

// RandomnessCommitterConfig holds configuration for randomness commitment operations.
type RandomnessCommitterConfig struct {
	// NumPubRand is the number of public randomness values to commit at once
	NumPubRand uint32

	// TimestampingDelayBlocks is the estimated delay in blocks for timestamping
	TimestampingDelayBlocks int64

	// ContextSigningHeight is the height at which the context signing is enabled
	ContextSigningHeight uint64

	// ChainID is the ID of the consumer chain where the randomness will be committed
	ChainID []byte
}

func NewRandomnessCommitterConfig(
	numPubRand uint32,
	timestampingDelayBlocks int64,
	contextSigningHeight uint64,
) *RandomnessCommitterConfig {
	return &RandomnessCommitterConfig{
		NumPubRand:              numPubRand,
		TimestampingDelayBlocks: timestampingDelayBlocks,
		ContextSigningHeight:    contextSigningHeight,
	}
}

type DefaultRandomnessCommitter struct {
	btcPk        *bbntypes.BIP340PubKey
	cfg          *RandomnessCommitterConfig
	pubRandState *PubRandState
	consumerCon  ccapi.ConsumerController
	em           eotsmanager.EOTSManager
	logger       *zap.Logger
	metrics      *metrics.FpMetrics
}

func NewDefaultRandomnessCommitter(
	cfg *RandomnessCommitterConfig,
	pubRandState *PubRandState,
	consumerCon ccapi.ConsumerController,
	em eotsmanager.EOTSManager,
	logger *zap.Logger,
	metrics *metrics.FpMetrics,
) *DefaultRandomnessCommitter {
	return &DefaultRandomnessCommitter{
		cfg:          cfg,
		pubRandState: pubRandState,
		consumerCon:  consumerCon,
		em:           em,
		logger:       logger,
		metrics:      metrics,
	}
}

func (rc *DefaultRandomnessCommitter) Init(btcPk *bbntypes.BIP340PubKey, chainID []byte) error {
	if btcPk == nil {
		return fmt.Errorf("btcPk cannot be nil")
	}
	if len(chainID) == 0 {
		return fmt.Errorf("chainID cannot be empty")
	}

	if rc.btcPk != nil && rc.cfg.ChainID != nil {
		return fmt.Errorf("randomness committer is already initialized with btcPk and chainID")
	}

	rc.btcPk = btcPk
	rc.cfg.ChainID = chainID

	return nil
}

// ShouldCommit determines whether a new randomness commit should be made
// Note: there's a delay from the commit is submitted to it is available to use due
// to timestamping. Therefore, the start height of the commit should consider an
// estimated delay.
// If randomness should be committed, the start height of the commit will be returned
func (rc *DefaultRandomnessCommitter) ShouldCommit(ctx context.Context) (bool, uint64, error) {
	lastCommittedHeight, err := rc.GetLastCommittedHeight(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("failed to get last committed height: %w", err)
	}

	tipBlock, err := rc.consumerCon.QueryLatestBlock(ctx)
	if tipBlock == nil || err != nil {
		return false, 0, fmt.Errorf("failed to get the last block: %w", err)
	}

	if rc.cfg.TimestampingDelayBlocks < 0 {
		return false, 0, fmt.Errorf("TimestampingDelayBlocks cannot be negative: %d", rc.cfg.TimestampingDelayBlocks)
	}

	// #nosec G115
	tipHeightWithDelay := tipBlock.GetHeight() + uint64(rc.cfg.TimestampingDelayBlocks)

	var startHeight uint64
	switch {
	case lastCommittedHeight < tipHeightWithDelay:
		// the start height should consider the timestamping delay
		// as it is only available to use after tip height + estimated timestamping delay
		startHeight = tipHeightWithDelay
	case lastCommittedHeight < tipHeightWithDelay+uint64(rc.cfg.NumPubRand):
		startHeight = lastCommittedHeight + 1
	default:
		// the randomness is enough, no need to make another commit
		rc.logger.Debug(
			"the finality-provider has sufficient public randomness, skip committing more",
			zap.String("pk", rc.btcPk.MarshalHex()),
			zap.Uint64("tip_height", tipBlock.GetHeight()),
			zap.Uint64("last_committed_height", lastCommittedHeight),
		)

		return false, 0, nil
	}

	rc.logger.Debug(
		"the finality-provider should commit randomness",
		zap.String("pk", rc.btcPk.MarshalHex()),
		zap.Uint64("tip_height", tipBlock.GetHeight()),
		zap.Uint64("last_committed_height", lastCommittedHeight),
	)

	activationBlkHeight, err := rc.consumerCon.QueryFinalityActivationBlockHeight(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("failed to query finality activation block height: %w", err)
	}

	// make sure that the start height is at least the finality activation height
	// and updated to generate the list with the same as the committed height.
	startHeight = max(startHeight, activationBlkHeight)

	return true, startHeight, nil
}

func (rc *DefaultRandomnessCommitter) GetLastCommittedHeight(ctx context.Context) (uint64, error) {
	pubRandCommit, err := rc.lastCommittedPublicRandWithRetry(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to query the last committed public randomness: %w", err)
	}

	// no committed randomness yet
	if pubRandCommit == nil {
		return 0, nil
	}

	return pubRandCommit.EndHeight(), nil
}

func (rc *DefaultRandomnessCommitter) lastCommittedPublicRandWithRetry(ctx context.Context) (*types.PubRandCommit, error) {
	var response *types.PubRandCommit
	if err := retry.Do(func() error {
		resp, err := rc.consumerCon.QueryLastPublicRandCommit(ctx, rc.btcPk.MustToBTCPK())
		if err != nil {
			return fmt.Errorf("failed to query the last committed public randomness: %w", err)
		}
		if resp != nil {
			if err := resp.Validate(); err != nil {
				return fmt.Errorf("failed to validate the last committed public randomness: %w", err)
			}
		}
		response = resp

		return nil
	}, retry.Context(ctx), RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		rc.logger.Debug(
			"failed to query the last committed public randomness",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return nil, fmt.Errorf("failed to query the last committed public randomness: %w", err)
	}

	return response, nil
}

// Commit commits a list of randomness from a given start height
func (rc *DefaultRandomnessCommitter) Commit(ctx context.Context, startHeight uint64) (*types.TxResponse, error) {
	// generate a list of Schnorr randomness pairs
	// NOTE: currently, calling this will create and save a list of randomness
	// in case of failure, randomness that has been created will be overwritten
	// for safety reason as the same randomness must not be used twice
	pubRandList, err := rc.getPubRandList(startHeight, rc.cfg.NumPubRand)
	if err != nil {
		return nil, fmt.Errorf("failed to generate randomness: %w", err)
	}
	numPubRand := uint64(len(pubRandList))

	// generate commitment and proof for each public randomness
	commitment, proofList := types.GetPubRandCommitAndProofs(pubRandList)

	// store them to database
	if err := rc.pubRandState.addPubRandProofList(rc.btcPk.MustMarshal(), rc.cfg.ChainID, startHeight, uint64(rc.cfg.NumPubRand), proofList); err != nil {
		return nil, fmt.Errorf("failed to save public randomness to DB: %w", err)
	}

	// sign the commitment
	schnorrSig, err := rc.signPubRandCommit(startHeight, numPubRand, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the Schnorr signature: %w", err)
	}

	res, err := rc.consumerCon.CommitPubRandList(ctx, &ccapi.CommitPubRandListRequest{
		FpPk:        rc.btcPk.MustToBTCPK(),
		StartHeight: startHeight,
		NumPubRand:  numPubRand,
		Commitment:  commitment,
		Sig:         schnorrSig,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to commit public randomness to the consumer chain: %w", err)
	}

	// Update metrics
	rc.metrics.RecordFpRandomnessTime(rc.btcPk.MarshalHex())
	rc.metrics.RecordFpLastCommittedRandomnessHeight(rc.btcPk.MarshalHex(), startHeight+numPubRand-1)
	rc.metrics.AddToFpTotalCommittedRandomness(rc.btcPk.MarshalHex(), float64(len(pubRandList)))
	rc.metrics.RecordFpLastCommittedRandomnessHeight(rc.btcPk.MarshalHex(), startHeight+numPubRand-1)

	return res, nil
}

func (rc *DefaultRandomnessCommitter) getPubRandList(startHeight uint64, numPubRand uint32) ([]*btcec.FieldVal, error) {
	pubRandList, err := rc.em.CreateRandomnessPairList(
		rc.btcPk.MustMarshal(),
		rc.cfg.ChainID,
		startHeight,
		numPubRand,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create randomness pair list: %w", err)
	}

	return pubRandList, nil
}

func (rc *DefaultRandomnessCommitter) signPubRandCommit(startHeight uint64, numPubRand uint64, commitment []byte) (*schnorr.Signature, error) {
	var (
		hash []byte
		err  error
	)

	if startHeight >= rc.cfg.ContextSigningHeight {
		signCtx := rc.consumerCon.GetFpRandCommitContext()
		hash, err = getHashToSignForCommitPubRandWithContext(signCtx, startHeight, numPubRand, commitment)
		if err != nil {
			return nil, fmt.Errorf("failed to sign the commit public randomness message: %w", err)
		}
	} else {
		hash, err = getHashToSignForCommitPubRandWithContext("", startHeight, numPubRand, commitment)
		if err != nil {
			return nil, fmt.Errorf("failed to sign the commit public randomness message: %w", err)
		}
	}

	// sign the message hash using the finality-provider's BTC private key
	sig, err := rc.em.SignSchnorrSig(rc.btcPk.MustMarshal(), hash)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the commit public randomness message: %w", err)
	}

	return sig, nil
}

func (rc *DefaultRandomnessCommitter) GetPubRandProofList(height uint64, numPubRand uint64) ([][]byte, error) {
	proofList, err := rc.pubRandState.getPubRandProofList(rc.btcPk.MustMarshal(), rc.cfg.ChainID, height, numPubRand)
	if err != nil {
		return nil, fmt.Errorf("failed to get public randomness proof list: %w", err)
	}

	if len(proofList) == 0 {
		return nil, fmt.Errorf("no public randomness proof found for height %d and num %d", height, numPubRand)
	}

	return proofList, nil
}

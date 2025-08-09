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
	"github.com/cometbft/cometbft/crypto/merkle"
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
	BtcPk        *bbntypes.BIP340PubKey
	Cfg          *RandomnessCommitterConfig
	PubRandState *PubRandState
	ConsumerCon  ccapi.ConsumerController
	Em           eotsmanager.EOTSManager
	Logger       *zap.Logger
	Metrics      *metrics.FpMetrics
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
		Cfg:          cfg,
		PubRandState: pubRandState,
		ConsumerCon:  consumerCon,
		Em:           em,
		Logger:       logger,
		Metrics:      metrics,
	}
}

func (rc *DefaultRandomnessCommitter) Init(btcPk *bbntypes.BIP340PubKey, chainID []byte) error {
	if btcPk == nil {
		return fmt.Errorf("BtcPk cannot be nil")
	}
	if len(chainID) == 0 {
		return fmt.Errorf("chainID cannot be empty")
	}

	if rc.BtcPk != nil && rc.Cfg.ChainID != nil {
		return fmt.Errorf("randomness committer is already initialized with BtcPk and chainID")
	}

	rc.BtcPk = btcPk
	rc.Cfg.ChainID = chainID

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

	tipBlock, err := rc.ConsumerCon.QueryLatestBlock(ctx)
	if tipBlock == nil || err != nil {
		return false, 0, fmt.Errorf("failed to get the last block: %w", err)
	}

	if rc.Cfg.TimestampingDelayBlocks < 0 {
		return false, 0, fmt.Errorf("TimestampingDelayBlocks cannot be negative: %d", rc.Cfg.TimestampingDelayBlocks)
	}

	// #nosec G115
	tipHeightWithDelay := tipBlock.GetHeight() + uint64(rc.Cfg.TimestampingDelayBlocks)

	var startHeight uint64
	switch {
	case lastCommittedHeight < tipHeightWithDelay:
		// the start height should consider the timestamping delay
		// as it is only available to use after tip height + estimated timestamping delay
		startHeight = tipHeightWithDelay
	case lastCommittedHeight < tipHeightWithDelay+uint64(rc.Cfg.NumPubRand):
		startHeight = lastCommittedHeight + 1
	default:
		// the randomness is enough, no need to make another commit
		rc.Logger.Debug(
			"the finality-provider has sufficient public randomness, skip committing more",
			zap.String("pk", rc.BtcPk.MarshalHex()),
			zap.Uint64("tip_height", tipBlock.GetHeight()),
			zap.Uint64("last_committed_height", lastCommittedHeight),
		)

		return false, 0, nil
	}

	rc.Logger.Debug(
		"the finality-provider should commit randomness",
		zap.String("pk", rc.BtcPk.MarshalHex()),
		zap.Uint64("tip_height", tipBlock.GetHeight()),
		zap.Uint64("last_committed_height", lastCommittedHeight),
	)

	activationBlkHeight, err := rc.ConsumerCon.QueryFinalityActivationBlockHeight(ctx)
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

	return pubRandCommit.GetEndHeight(), nil
}

func (rc *DefaultRandomnessCommitter) lastCommittedPublicRandWithRetry(ctx context.Context) (types.PubRandCommit, error) {
	var response types.PubRandCommit
	if err := retry.Do(func() error {
		resp, err := rc.ConsumerCon.QueryLastPubRandCommit(ctx, rc.BtcPk.MustToBTCPK())
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
		rc.Logger.Debug(
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
	pubRandList, err := rc.getPubRandList(startHeight, rc.Cfg.NumPubRand)
	if err != nil {
		return nil, fmt.Errorf("failed to generate randomness: %w", err)
	}
	numPubRand := uint64(len(pubRandList))

	// generate commitment and proof for each public randomness
	commitment, proofList := types.GetPubRandCommitAndProofs(pubRandList)

	// store them to database
	if err := rc.PubRandState.addPubRandProofList(rc.BtcPk.MustMarshal(), rc.Cfg.ChainID, startHeight, uint64(rc.Cfg.NumPubRand), proofList); err != nil {
		return nil, fmt.Errorf("failed to save public randomness to DB: %w", err)
	}

	// sign the commitment
	schnorrSig, err := rc.SignPubRandCommit(ctx, startHeight, numPubRand, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the Schnorr signature: %w", err)
	}

	res, err := rc.ConsumerCon.CommitPubRandList(ctx, &ccapi.CommitPubRandListRequest{
		FpPk:        rc.BtcPk.MustToBTCPK(),
		StartHeight: startHeight,
		NumPubRand:  numPubRand,
		Commitment:  commitment,
		Sig:         schnorrSig,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to commit public randomness to the consumer chain: %w", err)
	}

	// Update metrics
	rc.Metrics.RecordFpRandomnessTime(rc.BtcPk.MarshalHex())
	rc.Metrics.RecordFpLastCommittedRandomnessHeight(rc.BtcPk.MarshalHex(), startHeight+numPubRand-1)
	rc.Metrics.AddToFpTotalCommittedRandomness(rc.BtcPk.MarshalHex(), float64(len(pubRandList)))
	rc.Metrics.RecordFpLastCommittedRandomnessHeight(rc.BtcPk.MarshalHex(), startHeight+numPubRand-1)

	return res, nil
}

func (rc *DefaultRandomnessCommitter) getPubRandList(startHeight uint64, numPubRand uint32) ([]*btcec.FieldVal, error) {
	pubRandList, err := rc.Em.CreateRandomnessPairList(
		rc.BtcPk.MustMarshal(),
		rc.Cfg.ChainID,
		startHeight,
		numPubRand,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create randomness pair list: %w", err)
	}

	return pubRandList, nil
}

func (rc *DefaultRandomnessCommitter) SignPubRandCommit(ctx context.Context, startHeight uint64, numPubRand uint64, commitment []byte) (*schnorr.Signature, error) {
	var (
		hash []byte
		err  error
	)

	latestHeight, err := LatestBlockHeightWithRetry(ctx, rc.ConsumerCon, rc.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to query the latest block: %w", err)
	}

	if latestHeight >= rc.Cfg.ContextSigningHeight {
		signCtx := rc.ConsumerCon.GetFpRandCommitContext()
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
	sig, err := rc.Em.SignSchnorrSig(rc.BtcPk.MustMarshal(), hash)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the commit public randomness message: %w", err)
	}

	return sig, nil
}

func (rc *DefaultRandomnessCommitter) GetPubRandProofList(height uint64, numPubRand uint64) ([][]byte, error) {
	proofList, err := rc.PubRandState.getPubRandProofList(rc.BtcPk.MustMarshal(), rc.Cfg.ChainID, height, numPubRand)
	if err != nil {
		return nil, fmt.Errorf("failed to get public randomness proof list: %w", err)
	}

	if len(proofList) == 0 {
		return nil, fmt.Errorf("no public randomness proof found for height %d and num %d", height, numPubRand)
	}

	return proofList, nil
}
func (rc *DefaultRandomnessCommitter) AddPubRandProofListWithInterval(startHeight uint64, numPubRand uint64, proofList []*merkle.Proof, interval uint64) error {
	err := rc.PubRandState.addPubRandProofListWithInterval(rc.BtcPk.MustMarshal(), rc.Cfg.ChainID, startHeight, numPubRand, proofList, interval)
	if err != nil {
		return fmt.Errorf("failed to add public randomness proof list with interval: %w", err)
	}

	return nil
}

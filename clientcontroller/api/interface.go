package api

import (
	"context"

	"cosmossdk.io/math"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"

	"github.com/babylonlabs-io/finality-provider/types"
)

//nolint:revive,unused
const babylonConsumerChainType = "babylon"

// BabylonController defines the interface for interacting with the Babylon blockchain
// for finality provider operations
type BabylonController interface {
	// Start initializes the client connection
	Start() error

	// GetFpPopContextV0 returns the signing context for proof-of-possession
	GetFpPopContextV0() string

	// RegisterFinalityProvider registers a finality provider to the consumer chain
	RegisterFinalityProvider(ctx context.Context, req *RegisterFinalityProviderRequest) (*types.TxResponse, error)

	// QueryFinalityProvider queries the finality provider by public key
	// Note: the following queries are only for PoC
	QueryFinalityProvider(ctx context.Context, fpPk *btcec.PublicKey) (*btcstakingtypes.QueryFinalityProviderResponse, error)

	// EditFinalityProvider edits description and commission of a finality provider
	EditFinalityProvider(ctx context.Context, req *EditFinalityProviderRequest) (*btcstakingtypes.MsgEditFinalityProvider, error)

	// Close cleanly shuts down the client
	Close() error
}

// RegisterFinalityProviderRequest contains parameters for registering a finality provider
type RegisterFinalityProviderRequest struct {
	ChainID     string                          `json:"chain_id"`
	FpPk        *btcec.PublicKey                `json:"fp_pk"`
	Pop         []byte                          `json:"pop"`
	Commission  btcstakingtypes.CommissionRates `json:"commission"`
	Description []byte                          `json:"description"`
}

// EditFinalityProviderRequest contains parameters for editing a finality provider
type EditFinalityProviderRequest struct {
	FpPk        *btcec.PublicKey `json:"fp_pk"`
	Commission  *math.LegacyDec  `json:"commission,omitempty"`
	Description []byte           `json:"description,omitempty"`
}

type ConsumerController interface {
	RandomnessCommitter
	BlockQuerier[types.BlockDescription]
	FinalityOperator
	IsBSN() bool
	Close() error
}

// RandomnessCommitter handles public randomness commitment operations
type RandomnessCommitter interface {
	// GetFpRandCommitContext returns the signing context for public randomness commitment
	GetFpRandCommitContext() string

	// CommitPubRandList commits a list of EOTS public randomness to the consumer chain
	CommitPubRandList(ctx context.Context, req *CommitPubRandListRequest) (*types.TxResponse, error)

	// QueryLastPubRandCommit returns the last public randomness commitment
	QueryLastPubRandCommit(ctx context.Context, fpPk *btcec.PublicKey) (types.PubRandCommit, error)

	// QueryPubRandCommitList returns a list of public randomness commitments
	QueryPubRandCommitList(ctx context.Context, fpPk *btcec.PublicKey, startHeight uint64) ([]types.PubRandCommit, error)
}

type BlockQuerier[T types.BlockDescription] interface {
	// QueryLatestFinalizedBlock returns the latest finalized block
	QueryLatestFinalizedBlock(ctx context.Context) (T, error)

	// QueryBlock queries the block at the given height
	QueryBlock(ctx context.Context, height uint64) (T, error)

	// QueryIsBlockFinalized queries if the block at the given height is
	// finalized
	QueryIsBlockFinalized(ctx context.Context, height uint64) (bool, error)

	// QueryBlocks returns a list of blocks from startHeight to endHeight
	QueryBlocks(ctx context.Context, req *QueryBlocksRequest) ([]T, error)

	// QueryLatestBlock queries the tip block of the consumer chain
	QueryLatestBlock(ctx context.Context) (T, error)

	// QueryFinalityActivationBlockHeight return the block height when finality voting starts
	QueryFinalityActivationBlockHeight(ctx context.Context) (uint64, error)
}

// FinalityOperator handles finality signature submission operations
type FinalityOperator interface {
	// GetFpFinVoteContext returns the signing context for finality vote
	GetFpFinVoteContext() string
	// SubmitBatchFinalitySigs submits a batch of finality signatures to the consumer chain
	SubmitBatchFinalitySigs(ctx context.Context, req *SubmitBatchFinalitySigsRequest) (*types.TxResponse, error)

	// UnjailFinalityProvider sends an unjail transaction to the consumer chain
	UnjailFinalityProvider(ctx context.Context, fpPk *btcec.PublicKey) (*types.TxResponse, error)

	// QueryFinalityProviderHasPower queries whether the finality provider has voting power at a given height
	QueryFinalityProviderHasPower(ctx context.Context, req *QueryFinalityProviderHasPowerRequest) (bool, error)

	// QueryFinalityProviderStatus queries the finality provider status
	QueryFinalityProviderStatus(ctx context.Context, fpPk *btcec.PublicKey) (*FinalityProviderStatusResponse, error)

	// QueryFinalityProviderHighestVotedHeight queries the highest voted height of the given finality provider
	QueryFinalityProviderHighestVotedHeight(ctx context.Context, fpPk *btcec.PublicKey) (uint64, error)
}

type SubmitBatchFinalitySigsRequest struct {
	FpPk        *btcec.PublicKey
	Blocks      []types.BlockDescription
	PubRandList []*btcec.FieldVal
	ProofList   [][]byte
	Sigs        []*btcec.ModNScalar
}

type QueryBlocksRequest struct {
	StartHeight uint64
	EndHeight   uint64
	Limit       uint32
}

type CommitPubRandListRequest struct {
	FpPk        *btcec.PublicKey
	StartHeight uint64
	NumPubRand  uint64
	Commitment  []byte
	Sig         *schnorr.Signature
}

type FinalityProviderStatusResponse struct {
	Slashed bool
	Jailed  bool
}

type QueryFinalityProviderHasPowerRequest struct {
	FpPk        *btcec.PublicKey
	BlockHeight uint64
}

func NewSubmitBatchFinalitySigsRequest(
	fpPk *btcec.PublicKey,
	blocks []types.BlockDescription,
	pubRandList []*btcec.FieldVal,
	proofList [][]byte,
	sigs []*btcec.ModNScalar,
) *SubmitBatchFinalitySigsRequest {
	return &SubmitBatchFinalitySigsRequest{
		FpPk:        fpPk,
		Blocks:      blocks,
		PubRandList: pubRandList,
		ProofList:   proofList,
		Sigs:        sigs,
	}
}

func NewQueryBlocksRequest(
	startHeight uint64,
	endHeight uint64,
	limit uint32,
) *QueryBlocksRequest {
	return &QueryBlocksRequest{
		StartHeight: startHeight,
		EndHeight:   endHeight,
		Limit:       limit,
	}
}

func NewCommitPubRandListRequest(
	fpPk *btcec.PublicKey,
	startHeight uint64,
	numPubRand uint64,
	commitment []byte,
	sig *schnorr.Signature,
) *CommitPubRandListRequest {
	return &CommitPubRandListRequest{
		FpPk:        fpPk,
		StartHeight: startHeight,
		NumPubRand:  numPubRand,
		Commitment:  commitment,
		Sig:         sig,
	}
}

func NewFinalityProviderStatusResponse(
	slashed bool,
	jailed bool,
) *FinalityProviderStatusResponse {
	return &FinalityProviderStatusResponse{
		Slashed: slashed,
		Jailed:  jailed,
	}
}

func NewQueryFinalityProviderHasPowerRequest(
	fpPk *btcec.PublicKey,
	blockHeight uint64,
) *QueryFinalityProviderHasPowerRequest {
	return &QueryFinalityProviderHasPowerRequest{
		FpPk:        fpPk,
		BlockHeight: blockHeight,
	}
}

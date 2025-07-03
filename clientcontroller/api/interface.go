package api

import (
	"cosmossdk.io/math"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"

	"github.com/babylonlabs-io/finality-provider/types"
)

//nolint:revive,unused
const babylonConsumerChainType = "babylon"

type ClientController interface {
	// Start - starts the client controller
	Start() error
	// RegisterFinalityProvider registers a finality provider to the consumer chain
	// it returns tx hash and error. The address of the finality provider will be
	// the signer of the msg.
	RegisterFinalityProvider(
		chainID string,
		fpPk *btcec.PublicKey,
		pop []byte,
		commission btcstakingtypes.CommissionRates,
		description []byte,
	) (*types.TxResponse, error)

	// QueryFinalityProvider queries the finality provider by pk
	QueryFinalityProvider(fpPk *btcec.PublicKey) (*btcstakingtypes.QueryFinalityProviderResponse, error)

	// Note: the following queries are only for PoC

	// EditFinalityProvider edits description and commission of a finality provider
	EditFinalityProvider(fpPk *btcec.PublicKey, commission *math.LegacyDec, description []byte) (*btcstakingtypes.MsgEditFinalityProvider, error)

	Close() error
}

type ConsumerController interface {
	// CommitPubRandList commits a list of EOTS public randomness the consumer chain
	// it returns tx hash and error
	CommitPubRandList(fpPk *btcec.PublicKey, startHeight uint64, numPubRand uint64, commitment []byte, sig *schnorr.Signature) (*types.TxResponse, error)

	// SubmitFinalitySig submits the finality signature to the consumer chain
	SubmitFinalitySig(fpPk *btcec.PublicKey, block *types.BlockInfo, pubRand *btcec.FieldVal, proof []byte, sig *btcec.ModNScalar) (*types.TxResponse, error)

	// SubmitBatchFinalitySigs submits a batch of finality signatures to the consumer chain
	SubmitBatchFinalitySigs(fpPk *btcec.PublicKey, blocks []*types.BlockInfo, pubRandList []*btcec.FieldVal, proofList [][]byte, sigs []*btcec.ModNScalar) (*types.TxResponse, error)

	// UnjailFinalityProvider sends an unjail transaction to the consumer chain
	UnjailFinalityProvider(fpPk *btcec.PublicKey) (*types.TxResponse, error)

	/*
		The following methods are queries to the consumer chain
	*/

	// QueryFinalityProviderHasPower queries whether the finality provider has voting power at a given height
	QueryFinalityProviderHasPower(fpPk *btcec.PublicKey, blockHeight uint64) (bool, error)

	// QueryFinalityProviderSlashedOrJailed queries if the finality provider is slashed or slashed
	// Note: if the FP wants to get the information from the consumer chain directly, they should add this interface
	// function in ConsumerController. (https://github.com/babylonchain/finality-provider/pull/335#discussion_r1606175344)
	QueryFinalityProviderSlashedOrJailed(fpPk *btcec.PublicKey) (slashed bool, jailed bool, err error)

	// QueryLatestFinalizedBlock returns the latest finalized block
	// Note: nil will be returned if the finalized block does not exist
	QueryLatestFinalizedBlock() (*types.BlockInfo, error)

	// QueryFinalityProviderHighestVotedHeight queries the highest voted height of the given finality provider
	QueryFinalityProviderHighestVotedHeight(fpPk *btcec.PublicKey) (uint64, error)

	// QueryLastPublicRandCommit returns the last public randomness commitment
	QueryLastPublicRandCommit(fpPk *btcec.PublicKey) (*types.PubRandCommit, error)

	// QueryBlock queries the block at the given height
	QueryBlock(height uint64) (*types.BlockInfo, error)

	// QueryIsBlockFinalized queries if the block at the given height is finalized
	QueryIsBlockFinalized(height uint64) (bool, error)

	// QueryBlocks returns a list of blocks from startHeight to endHeight
	QueryBlocks(startHeight, endHeight uint64, limit uint32) ([]*types.BlockInfo, error)

	// QueryLatestBlockHeight queries the tip block height of the consumer chain
	QueryLatestBlockHeight() (uint64, error)

	// QueryActivatedHeight returns the activated height of the consumer chain
	// error will be returned if the consumer chain has not been activated
	QueryActivatedHeight() (uint64, error)

	// QueryFinalityActivationBlockHeight returns the block height of the consumer chain
	// starts to accept finality voting and pub rand commit as start height
	// error will be returned if the consumer chain failed to get this value
	// if the consumer chain wants to accept finality voting at any block height
	// the value zero should be returned.
	QueryFinalityActivationBlockHeight() (uint64, error)

	Close() error
}

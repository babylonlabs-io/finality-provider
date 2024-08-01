package api

import (
	"cosmossdk.io/math"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"

	"github.com/babylonlabs-io/finality-provider/types"
)

type ClientController interface {
	// RegisterFinalityProvider registers a finality provider to the consumer chain
	// it returns tx hash and error. The address of the finality provider will be
	// the signer of the msg.
	RegisterFinalityProvider(
		chainID string,
		fpPk *btcec.PublicKey,
		pop []byte,
		commission *math.LegacyDec,
		description []byte,
	) (*types.TxResponse, error)

	// Note: the following queries are only for PoC

	// QueryFinalityProviderSlashed queries if the finality provider is slashed
	// Note: if the FP wants to get the information from the consumer chain directly, they should add this interface
	// function in ConsumerController. (https://github.com/babylonchain/finality-provider/pull/335#discussion_r1606175344)
	QueryFinalityProviderSlashed(fpPk *btcec.PublicKey) (bool, error)

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

	// Note: the following queries are only for PoC

	// QueryFinalityProviderHasPower queries whether the finality provider has voting power at a given height
	QueryFinalityProviderHasPower(fpPk *btcec.PublicKey, blockHeight uint64) (bool, error)

	// QueryLatestFinalizedBlock returns the latest finalized block
	// Note: nil will be returned if the finalized block does not exist
	QueryLatestFinalizedBlock() (*types.BlockInfo, error)

	// QueryLastPublicRandCommit returns the last committed public randomness
	QueryLastPublicRandCommit(fpPk *btcec.PublicKey) (*types.PubRandCommit, error)

	// QueryBlock queries the block at the given height
	QueryBlock(height uint64) (*types.BlockInfo, error)

	// QueryIsBlockFinalized queries if the block at the given height is finalized
	QueryIsBlockFinalized(height uint64) (bool, error)

	// QueryBlocks returns a list of blocks from startHeight to endHeight
	QueryBlocks(startHeight, endHeight, limit uint64) ([]*types.BlockInfo, error)

	// QueryLatestBlockHeight queries the tip block height of the consumer chain
	QueryLatestBlockHeight() (uint64, error)

	// QueryActivatedHeight returns the activated height of the consumer chain
	// error will be returned if the consumer chain has not been activated
	QueryActivatedHeight() (uint64, error)

	Close() error
}

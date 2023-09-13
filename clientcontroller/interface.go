package clientcontroller

import (
	"fmt"

	"github.com/babylonchain/babylon/types"
	btcstakingtypes "github.com/babylonchain/babylon/x/btcstaking/types"
	finalitytypes "github.com/babylonchain/babylon/x/finality/types"
	"github.com/babylonchain/btc-validator/valcfg"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdkTypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/relayer/v2/relayer/provider"
	"github.com/sirupsen/logrus"
)

const (
	babylonConsumerChainName = "babylon"
)

type StakingParams struct {
	// K-deep
	ComfirmationTimeBlocks uint64
	// W-deep
	FinalizationTimeoutBlocks uint64

	// Minimum amount of satoshis required for slashing transaction
	MinSlashingTxFeeSat btcutil.Amount

	// Bitcoin public key of the current jury
	JuryPk *btcec.PublicKey

	// Address to which slashing transactions are sent
	SlashingAddress string

	// Minimum comission required by babylon
	MinComissionRate sdkTypes.Dec
}

type ClientController interface {
	GetStakingParams() (*StakingParams, error)
	// RegisterValidator registers a BTC validator via a MsgCreateBTCValidator to Babylon
	// it returns tx hash and error
	RegisterValidator(
		bbnPubKey *secp256k1.PubKey,
		btcPubKey *types.BIP340PubKey,
		pop *btcstakingtypes.ProofOfPossession,
		commission sdkTypes.Dec) (*provider.RelayerTxResponse, error)
	// CommitPubRandList commits a list of Schnorr public randomness via a MsgCommitPubRand to Babylon
	// it returns tx hash and error
	CommitPubRandList(btcPubKey *types.BIP340PubKey, startHeight uint64, pubRandList []types.SchnorrPubRand, sig *types.BIP340Signature) (*provider.RelayerTxResponse, error)
	// SubmitJurySig submits the Jury signature via a MsgAddJurySig to Babylon if the daemon runs in Jury mode
	// it returns tx hash and error
	SubmitJurySig(btcPubKey *types.BIP340PubKey, delPubKey *types.BIP340PubKey, stakingTxHash string, sig *types.BIP340Signature) (*provider.RelayerTxResponse, error)

	// SubmitJuryUnbondingSigs submits the Jury signatures via a MsgAddJuryUnbondingSigs to Babylon if the daemon runs in Jury mode
	// it returns tx hash and error
	SubmitJuryUnbondingSigs(
		btcPubKey *types.BIP340PubKey,
		delPubKey *types.BIP340PubKey,
		stakingTxHash string,
		unbondingSig *types.BIP340Signature,
		slashUnbondingSig *types.BIP340Signature,
	) (*provider.RelayerTxResponse, error)

	// SubmitFinalitySig submits the finality signature via a MsgAddVote to Babylon
	SubmitFinalitySig(btcPubKey *types.BIP340PubKey, blockHeight uint64, blockHash []byte, sig *types.SchnorrEOTSSig) (*provider.RelayerTxResponse, error)

	// SubmitValidatorUnbondingSig submits the validator signature for unbonding transaction
	SubmitValidatorUnbondingSig(
		valPubKey *types.BIP340PubKey,
		delPubKey *types.BIP340PubKey,
		stakingTxHash string,
		sig *types.BIP340Signature) (*provider.RelayerTxResponse, error)

	// Note: the following queries are only for PoC

	// QueryHeightWithLastPubRand queries the height of the last block with public randomness
	QueryHeightWithLastPubRand(btcPubKey *types.BIP340PubKey) (uint64, error)
	// QueryPendingBTCDelegations queries BTC delegations that need a Jury signature
	// it is only used when the program is running in Jury mode
	QueryPendingBTCDelegations() ([]*btcstakingtypes.BTCDelegation, error)

	// QueryUnbondindBTCDelegations queries BTC delegations that need a Jury sig for unbodning
	// it is only used when the program is running in Jury mode
	QueryUnbondindBTCDelegations() ([]*btcstakingtypes.BTCDelegation, error)

	// QueryValidatorVotingPower queries the voting power of the validator at a given height
	QueryValidatorVotingPower(btcPubKey *types.BIP340PubKey, blockHeight uint64) (uint64, error)
	// QueryLatestFinalisedBlocks returns the latest `count` finalised blocks
	QueryLatestFinalisedBlocks(count uint64) ([]*finalitytypes.IndexedBlock, error)
	// QueryIndexedBlock queries the Babylon indexed block at the given height
	QueryIndexedBlock(height uint64) (*finalitytypes.IndexedBlock, error)

	// QueryNodeStatus returns current node status, with info about latest block
	QueryNodeStatus() (*ctypes.ResultStatus, error)

	// QueryHeader queries the header at the given height, if header is not found
	// it returns result with nil header
	QueryHeader(height int64) (*ctypes.ResultHeader, error)

	// QueryBestHeader queries the tip header of the Babylon chain, if header is not found
	// it returns result with nil header
	QueryBestHeader() (*ctypes.ResultHeader, error)

	// QueryBTCValidatorUnbondingDelegations queries the unbonding delegations.UnbondingDelegations:
	// - already received unbodning transaction on babylon chain
	// - not received validator signature yet
	QueryBTCValidatorUnbondingDelegations(valBtcPk *types.BIP340PubKey, max uint64) ([]*btcstakingtypes.BTCDelegation, error)

	Close() error
}

func NewClientController(cfg *valcfg.Config, logger *logrus.Logger) (ClientController, error) {
	var (
		cc  ClientController
		err error
	)
	switch cfg.ChainName {
	case babylonConsumerChainName:
		cc, err = NewBabylonController(cfg.DataDir, cfg.BabylonConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create Babylon rpc client: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported consumer chain")
	}

	return cc, err
}
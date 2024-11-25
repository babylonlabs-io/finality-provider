package babylon

import (
	"context"
	"fmt"
	"strings"

	sdkErr "cosmossdk.io/errors"
	"cosmossdk.io/math"
	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	bsctypes "github.com/babylonlabs-io/babylon/x/btcstkconsumer/types"
	finalitytypes "github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	sttypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/relayer/v2/relayer/provider"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/types"
)

var _ api.ClientController = &BabylonController{}

var emptyErrs = []*sdkErr.Error{}

type BabylonController struct {
	bbnClient *bbnclient.Client
	cfg       *fpcfg.BBNConfig
	btcParams *chaincfg.Params
	logger    *zap.Logger
}

func NewBabylonController(
	cfg *fpcfg.BBNConfig,
	btcParams *chaincfg.Params,
	logger *zap.Logger,
) (*BabylonController, error) {

	bbnConfig := fpcfg.BBNConfigToBabylonConfig(cfg)

	bc, err := bbnclient.New(
		&bbnConfig,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Babylon client: %w", err)
	}

	// makes sure that the key in config really exists and it is a valid bech 32 addr
	// to allow using mustGetTxSigner
	if _, err := bc.GetAddr(); err != nil {
		return nil, err
	}

	return &BabylonController{
		bc,
		cfg,
		btcParams,
		logger,
	}, nil
}

func (bc *BabylonController) MustGetTxSigner() string {
	signer := bc.GetKeyAddress()
	prefix := bc.cfg.AccountPrefix
	return sdk.MustBech32ifyAddressBytes(prefix, signer)
}

func (bc *BabylonController) GetKeyAddress() sdk.AccAddress {
	// get key address, retrieves address based on the key name which is configured in
	// cfg *stakercfg.BBNConfig. If this fails, it means we have a misconfiguration problem
	// and we should panic.
	// This is checked at the start of BabylonController, so if it fails something is really wrong

	keyRec, err := bc.bbnClient.GetKeyring().Key(bc.cfg.Key)
	if err != nil {
		panic(fmt.Sprintf("Failed to get key address: %s", err))
	}

	addr, err := keyRec.GetAddress()
	if err != nil {
		panic(fmt.Sprintf("Failed to get key address: %s", err))
	}

	return addr
}

func (bc *BabylonController) reliablySendMsg(msg sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*provider.RelayerTxResponse, error) {
	return bc.reliablySendMsgs([]sdk.Msg{msg}, expectedErrs, unrecoverableErrs)
}

func (bc *BabylonController) reliablySendMsgs(msgs []sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*provider.RelayerTxResponse, error) {
	return bc.bbnClient.ReliablySendMsgs(
		context.Background(),
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)
}

// RegisterFinalityProvider registers a finality provider via a MsgCreateFinalityProvider to Babylon
// it returns tx hash and error
func (bc *BabylonController) RegisterFinalityProvider(
	chainID string,
	fpPk *btcec.PublicKey,
	pop []byte,
	commission *math.LegacyDec,
	description []byte,
) (*types.TxResponse, error) {
	var bbnPop btcstakingtypes.ProofOfPossessionBTC
	if err := bbnPop.Unmarshal(pop); err != nil {
		return nil, fmt.Errorf("invalid proof-of-possession: %w", err)
	}

	var sdkDescription sttypes.Description
	if err := sdkDescription.Unmarshal(description); err != nil {
		return nil, fmt.Errorf("invalid description: %w", err)
	}

	fpAddr := bc.MustGetTxSigner()
	msg := &btcstakingtypes.MsgCreateFinalityProvider{
		Addr:        fpAddr,
		BtcPk:       bbntypes.NewBIP340PubKeyFromBTCPK(fpPk),
		Pop:         &bbnPop,
		Commission:  commission,
		Description: &sdkDescription,
		ConsumerId:  chainID,
	}

	res, err := bc.reliablySendMsg(msg, emptyErrs, emptyErrs)
	if err != nil {
		return nil, err
	}

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

// CommitPubRandList commits a list of Schnorr public randomness via a MsgCommitPubRand to Babylon
// it returns tx hash and error
func (bc *BabylonController) CommitPubRandList(
	fpPk *btcec.PublicKey,
	startHeight uint64,
	numPubRand uint64,
	commitment []byte,
	sig *schnorr.Signature,
) (*types.TxResponse, error) {
	msg := &finalitytypes.MsgCommitPubRandList{
		Signer:      bc.MustGetTxSigner(),
		FpBtcPk:     bbntypes.NewBIP340PubKeyFromBTCPK(fpPk),
		StartHeight: startHeight,
		NumPubRand:  numPubRand,
		Commitment:  commitment,
		Sig:         bbntypes.NewBIP340SignatureFromBTCSig(sig),
	}

	unrecoverableErrs := []*sdkErr.Error{
		finalitytypes.ErrInvalidPubRand,
		finalitytypes.ErrTooFewPubRand,
		finalitytypes.ErrNoPubRandYet,
		btcstakingtypes.ErrFpNotFound,
	}

	res, err := bc.reliablySendMsg(msg, emptyErrs, unrecoverableErrs)
	if err != nil {
		return nil, err
	}

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

// SubmitFinalitySig submits the finality signature via a MsgAddVote to Babylon
func (bc *BabylonController) SubmitFinalitySig(
	fpPk *btcec.PublicKey,
	block *types.BlockInfo,
	pubRand *btcec.FieldVal,
	proof []byte, // TODO: have a type for proof
	sig *btcec.ModNScalar,
) (*types.TxResponse, error) {
	return bc.SubmitBatchFinalitySigs(
		fpPk, []*types.BlockInfo{block}, []*btcec.FieldVal{pubRand},
		[][]byte{proof}, []*btcec.ModNScalar{sig},
	)
}

// SubmitBatchFinalitySigs submits a batch of finality signatures to Babylon
func (bc *BabylonController) SubmitBatchFinalitySigs(
	fpPk *btcec.PublicKey,
	blocks []*types.BlockInfo,
	pubRandList []*btcec.FieldVal,
	proofList [][]byte,
	sigs []*btcec.ModNScalar,
) (*types.TxResponse, error) {
	if len(blocks) != len(sigs) {
		return nil, fmt.Errorf("the number of blocks %v should match the number of finality signatures %v", len(blocks), len(sigs))
	}

	msgs := make([]sdk.Msg, 0, len(blocks))
	for i, b := range blocks {
		cmtProof := cmtcrypto.Proof{}
		if err := cmtProof.Unmarshal(proofList[i]); err != nil {
			return nil, err
		}

		msg := &finalitytypes.MsgAddFinalitySig{
			Signer:       bc.MustGetTxSigner(),
			FpBtcPk:      bbntypes.NewBIP340PubKeyFromBTCPK(fpPk),
			BlockHeight:  b.Height,
			PubRand:      bbntypes.NewSchnorrPubRandFromFieldVal(pubRandList[i]),
			Proof:        &cmtProof,
			BlockAppHash: b.Hash,
			FinalitySig:  bbntypes.NewSchnorrEOTSSigFromModNScalar(sigs[i]),
		}
		msgs = append(msgs, msg)
	}

	unrecoverableErrs := []*sdkErr.Error{
		finalitytypes.ErrInvalidFinalitySig,
		finalitytypes.ErrPubRandNotFound,
		btcstakingtypes.ErrFpAlreadySlashed,
	}

	res, err := bc.reliablySendMsgs(msgs, emptyErrs, unrecoverableErrs)
	if err != nil {
		return nil, err
	}

	if res == nil {
		return &types.TxResponse{}, nil
	}

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

// UnjailFinalityProvider sends an unjail transaction to the consumer chain
func (bc *BabylonController) UnjailFinalityProvider(fpPk *btcec.PublicKey) (*types.TxResponse, error) {
	msg := &finalitytypes.MsgUnjailFinalityProvider{
		Signer:  bc.MustGetTxSigner(),
		FpBtcPk: bbntypes.NewBIP340PubKeyFromBTCPK(fpPk),
	}

	unrecoverableErrs := []*sdkErr.Error{
		btcstakingtypes.ErrFpNotFound,
		btcstakingtypes.ErrFpNotJailed,
		btcstakingtypes.ErrFpAlreadySlashed,
	}

	res, err := bc.reliablySendMsg(msg, emptyErrs, unrecoverableErrs)
	if err != nil {
		return nil, err
	}

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

// QueryFinalityProviderSlashedOrJailed - returns if the fp has been slashed, jailed, err
func (bc *BabylonController) QueryFinalityProviderSlashedOrJailed(fpPk *btcec.PublicKey) (bool, bool, error) {
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)
	res, err := bc.bbnClient.QueryClient.FinalityProvider(fpPubKey.MarshalHex())
	if err != nil {
		return false, false, fmt.Errorf("failed to query the finality provider %s: %v", fpPubKey.MarshalHex(), err)
	}

	return res.FinalityProvider.SlashedBtcHeight > 0, res.FinalityProvider.Jailed, nil
}

// QueryFinalityProviderHasPower queries whether the finality provider has voting power at a given height
func (bc *BabylonController) QueryFinalityProviderHasPower(fpPk *btcec.PublicKey, blockHeight uint64) (bool, error) {
	res, err := bc.bbnClient.QueryClient.FinalityProviderPowerAtHeight(
		bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex(),
		blockHeight,
	)
	if err != nil {
		// voting power table not updated indicates that no fp has voting power
		// therefore, it should be treated as the fp having 0 voting power
		if strings.Contains(err.Error(), btcstakingtypes.ErrVotingPowerTableNotUpdated.Error()) {
			bc.logger.Info("the voting power table not updated yet")
			return false, nil
		}

		return false, fmt.Errorf("failed to query Finality Voting Power at Height %d: %w", blockHeight, err)
	}

	return res.VotingPower > 0, nil
}

func (bc *BabylonController) QueryLatestFinalizedBlocks(count uint64) ([]*types.BlockInfo, error) {
	return bc.queryLatestBlocks(nil, count, finalitytypes.QueriedBlockStatus_FINALIZED, true)
}

func (bc *BabylonController) QueryBlocks(startHeight, endHeight, limit uint64) ([]*types.BlockInfo, error) {
	if endHeight < startHeight {
		return nil, fmt.Errorf("the startHeight %v should not be higher than the endHeight %v", startHeight, endHeight)
	}
	count := endHeight - startHeight + 1
	if count > limit {
		count = limit
	}
	return bc.queryLatestBlocks(sdk.Uint64ToBigEndian(startHeight), count, finalitytypes.QueriedBlockStatus_ANY, false)
}

func (bc *BabylonController) queryLatestBlocks(startKey []byte, count uint64, status finalitytypes.QueriedBlockStatus, reverse bool) ([]*types.BlockInfo, error) {
	var blocks []*types.BlockInfo
	pagination := &sdkquery.PageRequest{
		Limit:   count,
		Reverse: reverse,
		Key:     startKey,
	}

	res, err := bc.bbnClient.QueryClient.ListBlocks(status, pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to query finalized blocks: %v", err)
	}

	for _, b := range res.Blocks {
		ib := &types.BlockInfo{
			Height: b.Height,
			Hash:   b.AppHash,
		}
		blocks = append(blocks, ib)
	}

	return blocks, nil
}

func (bc *BabylonController) Close() error {
	if !bc.bbnClient.IsRunning() {
		return nil
	}

	return bc.bbnClient.Stop()
}

/*
	Implementations for e2e tests only
*/

func (bc *BabylonController) CreateBTCDelegation(
	delBtcPk *bbntypes.BIP340PubKey,
	fpPks []*btcec.PublicKey,
	pop *btcstakingtypes.ProofOfPossessionBTC,
	stakingTime uint32,
	stakingValue int64,
	stakingTxInfo *btcctypes.TransactionInfo,
	slashingTx *btcstakingtypes.BTCSlashingTx,
	delSlashingSig *bbntypes.BIP340Signature,
	unbondingTx []byte,
	unbondingTime uint32,
	unbondingValue int64,
	unbondingSlashingTx *btcstakingtypes.BTCSlashingTx,
	delUnbondingSlashingSig *bbntypes.BIP340Signature,
) (*types.TxResponse, error) {
	fpBtcPks := make([]bbntypes.BIP340PubKey, 0, len(fpPks))
	for _, v := range fpPks {
		fpBtcPks = append(fpBtcPks, *bbntypes.NewBIP340PubKeyFromBTCPK(v))
	}
	msg := &btcstakingtypes.MsgCreateBTCDelegation{
		StakerAddr:                    bc.MustGetTxSigner(),
		Pop:                           pop,
		BtcPk:                         delBtcPk,
		FpBtcPkList:                   fpBtcPks,
		StakingTime:                   stakingTime,
		StakingValue:                  stakingValue,
		StakingTx:                     stakingTxInfo,
		SlashingTx:                    slashingTx,
		DelegatorSlashingSig:          delSlashingSig,
		UnbondingTx:                   unbondingTx,
		UnbondingTime:                 unbondingTime,
		UnbondingValue:                unbondingValue,
		UnbondingSlashingTx:           unbondingSlashingTx,
		DelegatorUnbondingSlashingSig: delUnbondingSlashingSig,
	}

	res, err := bc.reliablySendMsg(msg, emptyErrs, emptyErrs)
	if err != nil {
		return nil, err
	}

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

func (bc *BabylonController) InsertBtcBlockHeaders(headers []bbntypes.BTCHeaderBytes) (*provider.RelayerTxResponse, error) {
	msg := &btclctypes.MsgInsertHeaders{
		Signer:  bc.MustGetTxSigner(),
		Headers: headers,
	}

	res, err := bc.reliablySendMsg(msg, emptyErrs, emptyErrs)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// TODO: only used in test. this should not be put here. it causes confusion that this is a method
// that will be used when FP runs. in that's the case, it implies it should work all all consumer
// types. but `bbnClient.QueryClient.FinalityProviders` doesn't work for consumer chains
func (bc *BabylonController) QueryFinalityProviders() ([]*btcstakingtypes.FinalityProviderResponse, error) {
	var fps []*btcstakingtypes.FinalityProviderResponse
	pagination := &sdkquery.PageRequest{
		Limit: 100,
	}

	for {
		res, err := bc.bbnClient.QueryClient.FinalityProviders(pagination)
		if err != nil {
			return nil, fmt.Errorf("failed to query finality providers: %v", err)
		}
		fps = append(fps, res.FinalityProviders...)
		if res.Pagination == nil || res.Pagination.NextKey == nil {
			break
		}

		pagination.Key = res.Pagination.NextKey
	}

	return fps, nil
}

func (bc *BabylonController) QueryConsumerFinalityProviders(consumerId string) ([]*bsctypes.FinalityProviderResponse, error) {
	var fps []*bsctypes.FinalityProviderResponse
	pagination := &sdkquery.PageRequest{
		Limit: 100,
	}

	for {
		res, err := bc.bbnClient.QueryClient.QueryConsumerFinalityProviders(consumerId, pagination)
		if err != nil {
			return nil, fmt.Errorf("failed to query finality providers: %v", err)
		}
		fps = append(fps, res.FinalityProviders...)
		if res.Pagination == nil || res.Pagination.NextKey == nil {
			break
		}

		pagination.Key = res.Pagination.NextKey
	}

	return fps, nil
}

func (bc *BabylonController) QueryBtcLightClientTip() (*btclctypes.BTCHeaderInfoResponse, error) {
	res, err := bc.bbnClient.QueryClient.BTCHeaderChainTip()
	if err != nil {
		return nil, fmt.Errorf("failed to query BTC tip: %v", err)
	}

	return res.Header, nil
}

func (bc *BabylonController) QueryCurrentEpoch() (uint64, error) {
	res, err := bc.bbnClient.QueryClient.CurrentEpoch()
	if err != nil {
		return 0, fmt.Errorf("failed to query BTC tip: %v", err)
	}

	return res.CurrentEpoch, nil
}

func (bc *BabylonController) QueryVotesAtHeight(height uint64) ([]bbntypes.BIP340PubKey, error) {
	res, err := bc.bbnClient.QueryClient.VotesAtHeight(height)
	if err != nil {
		return nil, fmt.Errorf("failed to query BTC delegations: %w", err)
	}

	return res.BtcPks, nil
}

func (bc *BabylonController) QueryPendingDelegations(limit uint64) ([]*btcstakingtypes.BTCDelegationResponse, error) {
	return bc.queryDelegationsWithStatus(btcstakingtypes.BTCDelegationStatus_PENDING, limit)
}

func (bc *BabylonController) QueryActiveDelegations(limit uint64) ([]*btcstakingtypes.BTCDelegationResponse, error) {
	return bc.queryDelegationsWithStatus(btcstakingtypes.BTCDelegationStatus_ACTIVE, limit)
}

// queryDelegationsWithStatus queries BTC delegations
// with the given status (either pending or unbonding)
// it is only used when the program is running in Covenant mode
func (bc *BabylonController) queryDelegationsWithStatus(status btcstakingtypes.BTCDelegationStatus, limit uint64) ([]*btcstakingtypes.BTCDelegationResponse, error) {
	pagination := &sdkquery.PageRequest{
		Limit: limit,
	}

	res, err := bc.bbnClient.QueryClient.BTCDelegations(status, pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to query BTC delegations: %v", err)
	}

	return res.BtcDelegations, nil
}

func (bc *BabylonController) QueryStakingParams() (*types.StakingParams, error) {
	// query btc checkpoint params
	ckptParamRes, err := bc.bbnClient.QueryClient.BTCCheckpointParams()
	if err != nil {
		return nil, fmt.Errorf("failed to query params of the btccheckpoint module: %v", err)
	}

	// query btc staking params
	stakingParamRes, err := bc.bbnClient.QueryClient.BTCStakingParams()
	if err != nil {
		return nil, fmt.Errorf("failed to query staking params: %v", err)
	}

	covenantPks := make([]*btcec.PublicKey, 0, len(stakingParamRes.Params.CovenantPks))
	for _, pk := range stakingParamRes.Params.CovenantPks {
		covPk, err := pk.ToBTCPK()
		if err != nil {
			return nil, fmt.Errorf("invalid covenant public key")
		}
		covenantPks = append(covenantPks, covPk)
	}

	return &types.StakingParams{
		ComfirmationTimeBlocks:    ckptParamRes.Params.BtcConfirmationDepth,
		FinalizationTimeoutBlocks: ckptParamRes.Params.CheckpointFinalizationTimeout,
		MinSlashingTxFeeSat:       btcutil.Amount(stakingParamRes.Params.MinSlashingTxFeeSat),
		CovenantPks:               covenantPks,
		SlashingPkScript:          stakingParamRes.Params.SlashingPkScript,
		CovenantQuorum:            stakingParamRes.Params.CovenantQuorum,
		SlashingRate:              stakingParamRes.Params.SlashingRate,
		MinUnbondingTime:          stakingParamRes.Params.MinUnbondingTimeBlocks,
	}, nil
}

func (bc *BabylonController) SubmitCovenantSigs(
	covPk *btcec.PublicKey,
	stakingTxHash string,
	slashingSigs [][]byte,
	unbondingSig *schnorr.Signature,
	unbondingSlashingSigs [][]byte,
) (*types.TxResponse, error) {
	bip340UnbondingSig := bbntypes.NewBIP340SignatureFromBTCSig(unbondingSig)

	msg := &btcstakingtypes.MsgAddCovenantSigs{
		Signer:                  bc.MustGetTxSigner(),
		Pk:                      bbntypes.NewBIP340PubKeyFromBTCPK(covPk),
		StakingTxHash:           stakingTxHash,
		SlashingTxSigs:          slashingSigs,
		UnbondingTxSig:          bip340UnbondingSig,
		SlashingUnbondingTxSigs: unbondingSlashingSigs,
	}

	res, err := bc.reliablySendMsg(msg, emptyErrs, emptyErrs)
	if err != nil {
		return nil, err
	}

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

func (bc *BabylonController) InsertSpvProofs(submitter string, proofs []*btcctypes.BTCSpvProof) (*provider.RelayerTxResponse, error) {
	msg := &btcctypes.MsgInsertBTCSpvProof{
		Submitter: submitter,
		Proofs:    proofs,
	}

	res, err := bc.reliablySendMsg(msg, emptyErrs, emptyErrs)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// RegisterConsumerChain registers a consumer chain via a MsgRegisterChain to Babylon
func (bc *BabylonController) RegisterConsumerChain(id, name, description string) (*types.TxResponse, error) {
	msg := &bsctypes.MsgRegisterConsumer{
		Signer:              bc.MustGetTxSigner(),
		ConsumerId:          id,
		ConsumerName:        name,
		ConsumerDescription: description,
	}

	res, err := bc.reliablySendMsg(msg, emptyErrs, emptyErrs)
	if err != nil {
		return nil, err
	}

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

func (bc *BabylonController) GetBBNClient() *bbnclient.Client {
	return bc.bbnClient
}

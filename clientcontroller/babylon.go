package clientcontroller

import (
	"context"
	"fmt"
	"github.com/babylonlabs-io/babylon/client/babylonclient"
	"strings"
	"time"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"

	sdkErr "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	finalitytypes "github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	sttypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"go.uber.org/zap"
	protobuf "google.golang.org/protobuf/proto"

	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/types"
)

var _ ClientController = &BabylonController{}

var emptyErrs = []*sdkErr.Error{}

type BabylonController struct {
	bbnClient *bbnclient.Client
	cfg       *fpcfg.BBNConfig
	btcParams *chaincfg.Params
	logger    *zap.Logger
}

func NewBabylonController(
	bbnClient *bbnclient.Client,
	cfg *fpcfg.BBNConfig,
	btcParams *chaincfg.Params,
	logger *zap.Logger,
) *BabylonController {
	return &BabylonController{
		bbnClient,
		cfg,
		btcParams,
		logger,
	}
}

func (bc *BabylonController) Start() error {
	// makes sure that the key in config really exists and is a valid bech32 addr
	// to allow using mustGetTxSigner
	if _, err := bc.bbnClient.GetAddr(); err != nil {
		return fmt.Errorf("failed to get addr: %w", err)
	}

	enabled, err := bc.NodeTxIndexEnabled()
	if err != nil {
		return err
	}

	if !enabled {
		return fmt.Errorf("tx indexing in the babylon node must be enabled")
	}

	return nil
}

func (bc *BabylonController) mustGetTxSigner() string {
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

func (bc *BabylonController) reliablySendMsg(msg sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*babylonclient.RelayerTxResponse, error) {
	return bc.reliablySendMsgs([]sdk.Msg{msg}, expectedErrs, unrecoverableErrs)
}

func (bc *BabylonController) reliablySendMsgs(msgs []sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*babylonclient.RelayerTxResponse, error) {
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
	fpPk *btcec.PublicKey,
	pop []byte,
	commission *sdkmath.LegacyDec,
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

	fpAddr := bc.mustGetTxSigner()
	msg := &btcstakingtypes.MsgCreateFinalityProvider{
		Addr:        fpAddr,
		BtcPk:       bbntypes.NewBIP340PubKeyFromBTCPK(fpPk),
		Pop:         &bbnPop,
		Commission:  commission,
		Description: &sdkDescription,
	}

	res, err := bc.reliablySendMsg(msg, emptyErrs, emptyErrs)
	if err != nil {
		return nil, err
	}

	return &types.TxResponse{TxHash: res.TxHash, Events: res.Events}, nil
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
		Signer:      bc.mustGetTxSigner(),
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

	return &types.TxResponse{TxHash: res.TxHash, Events: res.Events}, nil
}

// SubmitFinalitySig submits the finality signature via a MsgAddVote to Babylon
func (bc *BabylonController) SubmitFinalitySig(
	fpPk *btcec.PublicKey,
	block *types.BlockInfo,
	pubRand *btcec.FieldVal,
	proof []byte,
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
			Signer:       bc.mustGetTxSigner(),
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

	expectedErrs := []*sdkErr.Error{
		finalitytypes.ErrDuplicatedFinalitySig,
	}

	res, err := bc.reliablySendMsgs(msgs, expectedErrs, unrecoverableErrs)
	if err != nil {
		return nil, err
	}

	if res == nil {
		return &types.TxResponse{}, nil
	}

	return &types.TxResponse{TxHash: res.TxHash, Events: res.Events}, nil
}

// UnjailFinalityProvider sends an unjail transaction to the consumer chain
func (bc *BabylonController) UnjailFinalityProvider(fpPk *btcec.PublicKey) (*types.TxResponse, error) {
	msg := &finalitytypes.MsgUnjailFinalityProvider{
		Signer:  bc.mustGetTxSigner(),
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

	return &types.TxResponse{TxHash: res.TxHash, Events: res.Events}, nil
}

// QueryFinalityProviderSlashedOrJailed - returns if the fp has been slashed, jailed, err
func (bc *BabylonController) QueryFinalityProviderSlashedOrJailed(fpPk *btcec.PublicKey) (bool, bool, error) {
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)
	res, err := bc.bbnClient.QueryClient.FinalityProvider(fpPubKey.MarshalHex())
	if err != nil {
		return false, false, fmt.Errorf("failed to query the finality provider %s: %w", fpPubKey.MarshalHex(), err)
	}

	return res.FinalityProvider.SlashedBtcHeight > 0, res.FinalityProvider.Jailed, nil
}

// QueryFinalityProviderVotingPower queries the voting power of the finality provider at a given height
func (bc *BabylonController) QueryFinalityProviderVotingPower(fpPk *btcec.PublicKey, blockHeight uint64) (uint64, error) {
	res, err := bc.bbnClient.QueryClient.FinalityProviderPowerAtHeight(
		bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex(),
		blockHeight,
	)
	if err != nil {
		// voting power table not updated indicates that no fp has voting power
		// therefore, it should be treated as the fp having 0 voting power
		if strings.Contains(err.Error(), finalitytypes.ErrVotingPowerTableNotUpdated.Error()) {
			bc.logger.Info("the voting power table not updated yet")

			return 0, nil
		}

		return 0, err
	}

	return res.VotingPower, nil
}

// QueryFinalityProviderHighestVotedHeight queries the highest voted height of the given finality provider
func (bc *BabylonController) QueryFinalityProviderHighestVotedHeight(fpPk *btcec.PublicKey) (uint64, error) {
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)
	res, err := bc.bbnClient.QueryClient.FinalityProvider(fpPubKey.MarshalHex())
	if err != nil {
		return 0, fmt.Errorf("failed to query highest voted height for finality provider %s: %w", fpPubKey.MarshalHex(), err)
	}

	return uint64(res.FinalityProvider.HighestVotedHeight), nil
}

func (bc *BabylonController) QueryLatestFinalizedBlocks(count uint64) ([]*types.BlockInfo, error) {
	return bc.queryLatestBlocks(nil, count, finalitytypes.QueriedBlockStatus_FINALIZED, true)
}

// QueryLastCommittedPublicRand returns the last public randomness commitments
func (bc *BabylonController) QueryLastCommittedPublicRand(fpPk *btcec.PublicKey, count uint64) (map[uint64]*finalitytypes.PubRandCommitResponse, error) {
	fpBtcPk := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)

	pagination := &sdkquery.PageRequest{
		// NOTE: the count is limited by pagination queries
		Limit:   count,
		Reverse: true,
	}

	res, err := bc.bbnClient.QueryClient.ListPubRandCommit(fpBtcPk.MarshalHex(), pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to query committed public randomness: %w", err)
	}

	return res.PubRandCommitMap, nil
}

func (bc *BabylonController) QueryBlocks(startHeight, endHeight uint64, limit uint32) ([]*types.BlockInfo, error) {
	if endHeight < startHeight {
		return nil, fmt.Errorf("the startHeight %v should not be higher than the endHeight %v", startHeight, endHeight)
	}
	count := endHeight - startHeight + 1
	if count > uint64(limit) {
		count = uint64(limit)
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
		return nil, fmt.Errorf("failed to query finalized blocks: %w", err)
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

func getContextWithCancel(timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	return ctx, cancel
}

func (bc *BabylonController) QueryBlock(height uint64) (*types.BlockInfo, error) {
	res, err := bc.bbnClient.QueryClient.Block(height)
	if err != nil {
		return nil, fmt.Errorf("failed to query indexed block at height %v: %w", height, err)
	}

	return &types.BlockInfo{
		Height:    height,
		Hash:      res.Block.AppHash,
		Finalized: res.Block.Finalized,
	}, nil
}

func (bc *BabylonController) QueryActivatedHeight() (uint64, error) {
	res, err := bc.bbnClient.QueryClient.ActivatedHeight()
	if err != nil {
		return 0, fmt.Errorf("failed to query activated height: %w", err)
	}

	return res.Height, nil
}

func (bc *BabylonController) QueryFinalityActivationBlockHeight() (uint64, error) {
	res, err := bc.bbnClient.QueryClient.FinalityParams()
	if err != nil {
		return 0, fmt.Errorf("failed to query finality params to get finality activation block height: %w", err)
	}

	return res.Params.FinalityActivationHeight, nil
}

func (bc *BabylonController) QueryBestBlock() (*types.BlockInfo, error) {
	blocks, err := bc.queryLatestBlocks(nil, 1, finalitytypes.QueriedBlockStatus_ANY, true)
	if err != nil || len(blocks) != 1 {
		// try query comet block if the index block query is not available
		return bc.queryCometBestBlock()
	}

	return blocks[0], nil
}

func (bc *BabylonController) NodeTxIndexEnabled() (bool, error) {
	res, err := bc.bbnClient.GetStatus()
	if err != nil {
		return false, fmt.Errorf("failed to query node status: %w", err)
	}

	return res.TxIndexEnabled(), nil
}

func (bc *BabylonController) queryCometBestBlock() (*types.BlockInfo, error) {
	ctx, cancel := getContextWithCancel(bc.cfg.Timeout)
	// this will return 20 items at max in the descending order (highest first)
	chainInfo, err := bc.bbnClient.RPCClient.BlockchainInfo(ctx, 0, 0)
	defer cancel()

	if err != nil {
		return nil, err
	}

	headerHeightInt64 := chainInfo.BlockMetas[0].Header.Height
	if headerHeightInt64 < 0 {
		return nil, fmt.Errorf("block height %v should be positive", headerHeightInt64)
	}
	// Returning response directly, if header with specified number did not exist
	// at request will contain nil header
	return &types.BlockInfo{
		Height: uint64(headerHeightInt64),
		Hash:   chainInfo.BlockMetas[0].Header.AppHash,
	}, nil
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
		StakerAddr:                    bc.mustGetTxSigner(),
		Pop:                           pop,
		BtcPk:                         delBtcPk,
		FpBtcPkList:                   fpBtcPks,
		StakingTime:                   stakingTime,
		StakingValue:                  stakingValue,
		StakingTx:                     stakingTxInfo.Transaction,
		StakingTxInclusionProof:       btcstakingtypes.NewInclusionProof(stakingTxInfo.Key, stakingTxInfo.Proof),
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

func (bc *BabylonController) InsertBtcBlockHeaders(headers []bbntypes.BTCHeaderBytes) (*babylonclient.RelayerTxResponse, error) {
	msg := &btclctypes.MsgInsertHeaders{
		Signer:  bc.mustGetTxSigner(),
		Headers: headers,
	}

	res, err := bc.reliablySendMsg(msg, emptyErrs, emptyErrs)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (bc *BabylonController) QueryFinalityProviders() ([]*btcstakingtypes.FinalityProviderResponse, error) {
	var fps []*btcstakingtypes.FinalityProviderResponse
	pagination := &sdkquery.PageRequest{
		Limit: 100,
	}

	for {
		res, err := bc.bbnClient.QueryClient.FinalityProviders(pagination)
		if err != nil {
			return nil, fmt.Errorf("failed to query finality providers: %w", err)
		}
		fps = append(fps, res.FinalityProviders...)
		if res.Pagination == nil || res.Pagination.NextKey == nil {
			break
		}

		pagination.Key = res.Pagination.NextKey
	}

	return fps, nil
}

func (bc *BabylonController) QueryFinalityProvider(fpPk *btcec.PublicKey) (*btcstakingtypes.QueryFinalityProviderResponse, error) {
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)
	res, err := bc.bbnClient.QueryClient.FinalityProvider(fpPubKey.MarshalHex())
	if err != nil {
		return nil, fmt.Errorf("failed to query the finality provider %s: %w", fpPubKey.MarshalHex(), err)
	}

	return res, nil
}

func (bc *BabylonController) EditFinalityProvider(fpPk *btcec.PublicKey,
	rate *sdkmath.LegacyDec, description []byte) (*btcstakingtypes.MsgEditFinalityProvider, error) {
	var reqDesc proto.Description
	if err := protobuf.Unmarshal(description, &reqDesc); err != nil {
		return nil, err
	}
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)

	fpRes, err := bc.QueryFinalityProvider(fpPk)
	if err != nil {
		return nil, err
	}

	if !strings.EqualFold(fpRes.FinalityProvider.Addr, bc.mustGetTxSigner()) {
		return nil, fmt.Errorf("the signer does not correspond to the finality provider's "+
			"Babylon address, expected %s got %s", bc.mustGetTxSigner(), fpRes.FinalityProvider.Addr)
	}

	getValueOrDefault := func(reqValue, defaultValue string) string {
		if reqValue != "" {
			return reqValue
		}

		return defaultValue
	}

	resDesc := fpRes.FinalityProvider.Description

	desc := &sttypes.Description{
		Moniker:         getValueOrDefault(reqDesc.Moniker, resDesc.Moniker),
		Identity:        getValueOrDefault(reqDesc.Identity, resDesc.Identity),
		Website:         getValueOrDefault(reqDesc.Website, resDesc.Website),
		SecurityContact: getValueOrDefault(reqDesc.SecurityContact, resDesc.SecurityContact),
		Details:         getValueOrDefault(reqDesc.Details, resDesc.Details),
	}

	msg := &btcstakingtypes.MsgEditFinalityProvider{
		Addr:        bc.mustGetTxSigner(),
		BtcPk:       fpPubKey.MustMarshal(),
		Description: desc,
		Commission:  fpRes.FinalityProvider.Commission,
	}

	if rate != nil {
		msg.Commission = rate
	}

	_, err = bc.reliablySendMsg(msg, emptyErrs, emptyErrs)
	if err != nil {
		return nil, fmt.Errorf("failed to query the finality provider %s: %w", fpPk.SerializeCompressed(), err)
	}

	return msg, nil
}

func (bc *BabylonController) QueryBtcLightClientTip() (*btclctypes.BTCHeaderInfoResponse, error) {
	res, err := bc.bbnClient.QueryClient.BTCHeaderChainTip()
	if err != nil {
		return nil, fmt.Errorf("failed to query BTC tip: %w", err)
	}

	return res.Header, nil
}

func (bc *BabylonController) QueryCurrentEpoch() (uint64, error) {
	res, err := bc.bbnClient.QueryClient.CurrentEpoch()
	if err != nil {
		return 0, fmt.Errorf("failed to query BTC tip: %w", err)
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
		return nil, fmt.Errorf("failed to query BTC delegations: %w", err)
	}

	return res.BtcDelegations, nil
}

func (bc *BabylonController) QueryStakingParams() (*types.StakingParams, error) {
	// query btc checkpoint params
	ckptParamRes, err := bc.bbnClient.QueryClient.BTCCheckpointParams()
	if err != nil {
		return nil, fmt.Errorf("failed to query params of the btccheckpoint module: %w", err)
	}

	// query btc staking params
	stakingParamRes, err := bc.bbnClient.QueryClient.BTCStakingParams()
	if err != nil {
		return nil, fmt.Errorf("failed to query staking params: %w", err)
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
		UnbondingTime:             stakingParamRes.Params.UnbondingTimeBlocks,
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
		Signer:                  bc.mustGetTxSigner(),
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

	return &types.TxResponse{TxHash: res.TxHash, Events: res.Events}, nil
}

func (bc *BabylonController) GetBBNClient() *bbnclient.Client {
	return bc.bbnClient
}

func (bc *BabylonController) InsertSpvProofs(submitter string, proofs []*btcctypes.BTCSpvProof) (*babylonclient.RelayerTxResponse, error) {
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

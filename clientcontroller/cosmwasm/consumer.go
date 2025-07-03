package cosmwasm

import (
	"context"
	"encoding/json"
	"fmt"

	"sort"
	"strings"

	sdkErr "cosmossdk.io/errors"
	wasmdparams "github.com/CosmWasm/wasmd/app/params"
	wasmdtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/babylonlabs-io/babylon/v3/client/babylonclient"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	finalitytypes "github.com/babylonlabs-io/babylon/v3/x/finality/types"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	cwcclient "github.com/babylonlabs-io/finality-provider/cosmwasmclient/client"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	fptypes "github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"go.uber.org/zap"
)

var _ api.ConsumerController = &CosmwasmConsumerController{}

//nolint:revive
type CosmwasmConsumerController struct {
	cwClient *cwcclient.Client
	cfg      *fpcfg.CosmwasmConfig
	logger   *zap.Logger
}

func NewCosmwasmConsumerController(
	cfg *fpcfg.CosmwasmConfig,
	encodingCfg wasmdparams.EncodingConfig,
	logger *zap.Logger,
) (*CosmwasmConsumerController, error) {
	wasmdConfig := cfg.ToQueryClientConfig()

	if err := wasmdConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config for Wasmd client: %w", err)
	}

	wc, err := cwcclient.New(
		wasmdConfig,
		"wasmd",
		encodingCfg,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Wasmd client: %w", err)
	}

	return &CosmwasmConsumerController{
		wc,
		cfg,
		logger,
	}, nil
}

func (wc *CosmwasmConsumerController) reliablySendMsg(msg sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*babylonclient.RelayerTxResponse, error) {
	return wc.reliablySendMsgs([]sdk.Msg{msg}, expectedErrs, unrecoverableErrs)
}

func (wc *CosmwasmConsumerController) reliablySendMsgs(msgs []sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*babylonclient.RelayerTxResponse, error) {
	resp, err := wc.cwClient.ReliablySendMsgs(
		context.Background(),
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)
	if err != nil {
		return nil, err
	}

	bbnResp := fptypes.NewBabylonTxResponse(resp)

	return bbnResp, nil
}

func (wc *CosmwasmConsumerController) GetClient() *cwcclient.Client {
	return wc.cwClient
}

// CommitPubRandList commits a list of Schnorr public randomness via a MsgCommitPubRand to Babylon
// it returns tx hash and error
func (wc *CosmwasmConsumerController) CommitPubRandList(
	fpPk *btcec.PublicKey,
	startHeight uint64,
	numPubRand uint64,
	commitment []byte,
	sig *schnorr.Signature,
) (*fptypes.TxResponse, error) {
	bip340Sig := bbntypes.NewBIP340SignatureFromBTCSig(sig).MustMarshal()

	// Construct the ExecMsg struct
	msg := ExecMsg{
		CommitPublicRandomness: &CommitPublicRandomness{
			FPPubKeyHex: bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex(),
			StartHeight: startHeight,
			NumPubRand:  numPubRand,
			Commitment:  commitment,
			Signature:   bip340Sig,
		},
	}

	// Marshal the ExecMsg struct to JSON
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	res, err := wc.ExecuteBTCFinalityContract(msgBytes)
	if err != nil {
		return nil, err
	}

	return &fptypes.TxResponse{TxHash: res.TxHash}, nil
}

// SubmitFinalitySig submits the finality signature via a MsgAddVote to Babylon
func (wc *CosmwasmConsumerController) SubmitFinalitySig(
	fpPk *btcec.PublicKey,
	block *fptypes.BlockInfo,
	pubRand *btcec.FieldVal,
	proof []byte, // TODO: have a type for proof
	sig *btcec.ModNScalar,
) (*fptypes.TxResponse, error) {
	cmtProof := cmtcrypto.Proof{}
	if err := cmtProof.Unmarshal(proof); err != nil {
		return nil, err
	}

	msg := ExecMsg{
		SubmitFinalitySignature: &SubmitFinalitySignature{
			FpPubkeyHex: bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex(),
			Height:      block.Height,
			PubRand:     bbntypes.NewSchnorrPubRandFromFieldVal(pubRand).MustMarshal(),
			Proof:       cmtProof,
			BlockHash:   block.Hash,
			Signature:   bbntypes.NewSchnorrEOTSSigFromModNScalar(sig).MustMarshal(),
		},
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	res, err := wc.ExecuteBTCFinalityContract(msgBytes)
	if err != nil {
		return nil, err
	}

	return &fptypes.TxResponse{TxHash: res.TxHash, Events: res.Events}, nil
}

// SubmitBatchFinalitySigs submits a batch of finality signatures to Babylon
func (wc *CosmwasmConsumerController) SubmitBatchFinalitySigs(
	fpPk *btcec.PublicKey,
	blocks []*fptypes.BlockInfo,
	pubRandList []*btcec.FieldVal,
	proofList [][]byte,
	sigs []*btcec.ModNScalar,
) (*fptypes.TxResponse, error) {
	msgs := make([]sdk.Msg, 0, len(blocks))
	for i, b := range blocks {
		cmtProof := cmtcrypto.Proof{}
		if err := cmtProof.Unmarshal(proofList[i]); err != nil {
			return nil, err
		}

		msg := ExecMsg{
			SubmitFinalitySignature: &SubmitFinalitySignature{
				FpPubkeyHex: bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex(),
				Height:      b.Height,
				PubRand:     bbntypes.NewSchnorrPubRandFromFieldVal(pubRandList[i]).MustMarshal(),
				Proof:       cmtProof,
				BlockHash:   b.Hash,
				Signature:   bbntypes.NewSchnorrEOTSSigFromModNScalar(sigs[i]).MustMarshal(),
			},
		}

		msgBytes, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}

		execMsg := &wasmdtypes.MsgExecuteContract{
			Sender:   wc.cwClient.MustGetAddr(),
			Contract: wc.cfg.BtcFinalityContractAddress,
			Msg:      msgBytes,
		}
		msgs = append(msgs, execMsg)
	}

	expectedErrs := []*sdkErr.Error{
		finalitytypes.ErrDuplicatedFinalitySig,
		finalitytypes.ErrSigHeightOutdated,
	}

	res, err := wc.reliablySendMsgs(msgs, expectedErrs, nil)
	if err != nil {
		return nil, err
	}

	return &fptypes.TxResponse{TxHash: res.TxHash}, nil
}

// QueryFinalityProviderHasPower queries whether the finality provider has voting power at a given height
func (wc *CosmwasmConsumerController) QueryFinalityProviderHasPower(
	fpPk *btcec.PublicKey,
	blockHeight uint64,
) (bool, error) {
	fpBtcPkHex := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex()

	queryMsgStruct := QueryMsgFinalityProviderInfo{
		FinalityProviderInfo: FinalityProviderInfo{
			BtcPkHex: fpBtcPkHex,
			Height:   blockHeight,
		},
	}
	queryMsgBytes, err := json.Marshal(queryMsgStruct)
	if err != nil {
		return false, fmt.Errorf("failed to marshal query message: %w", err)
	}
	dataFromContract, err := wc.QuerySmartContractState(wc.cfg.BtcStakingContractAddress, string(queryMsgBytes))
	if err != nil {
		return false, err
	}

	var resp ConsumerFpInfoResponse
	err = json.Unmarshal(dataFromContract.Data, &resp)
	if err != nil {
		return false, err
	}

	return resp.Power > 0, nil
}

func (wc *CosmwasmConsumerController) QueryFinalityProvidersByPower() (*ConsumerFpsByPowerResponse, error) {
	queryMsgStruct := QueryMsgFinalityProvidersByPower{
		FinalityProvidersByPower: struct{}{},
	}

	queryMsgBytes, err := json.Marshal(queryMsgStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query message: %w", err)
	}

	dataFromContract, err := wc.QuerySmartContractState(wc.cfg.BtcStakingContractAddress, string(queryMsgBytes))
	if err != nil {
		return nil, err
	}

	var resp ConsumerFpsByPowerResponse
	err = json.Unmarshal(dataFromContract.Data, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (wc *CosmwasmConsumerController) QueryLatestFinalizedBlock() (*fptypes.BlockInfo, error) {
	isFinalized := true
	limit := uint64(1)
	blocks, err := wc.queryLatestBlocks(nil, &limit, &isFinalized, nil)
	if err != nil || len(blocks) == 0 {
		// do not return error here as FP handles this situation by
		// not running fast sync
		//nolint:nilerr
		return nil, nil
	}

	return blocks[0], nil
}

func (wc *CosmwasmConsumerController) QueryBlocks(startHeight, endHeight uint64, _ uint32) ([]*fptypes.BlockInfo, error) {
	return wc.queryCometBlocksInRange(startHeight, endHeight)
}

func (wc *CosmwasmConsumerController) QueryBlock(height uint64) (*fptypes.BlockInfo, error) {
	block, err := wc.cwClient.GetBlock(context.Background(), int64(height)) // #nosec G115
	if err != nil {
		return nil, err
	}

	return &fptypes.BlockInfo{
		Height: uint64(block.Block.Header.Height), // #nosec G115
		Hash:   block.Block.Header.AppHash,
	}, nil
}

// QueryLastPublicRandCommit returns the last public randomness commitments
func (wc *CosmwasmConsumerController) QueryLastPublicRandCommit(fpPk *btcec.PublicKey) (*fptypes.PubRandCommit, error) {
	fpBtcPk := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)

	// Construct the query message
	queryMsgStruct := QueryMsgLastPubRandCommit{
		LastPubRandCommit: LastPubRandCommitQuery{
			BtcPkHex: fpBtcPk.MarshalHex(),
		},
	}

	queryMsgBytes, err := json.Marshal(queryMsgStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query message: %w", err)
	}

	// Query the smart contract state
	dataFromContract, err := wc.QuerySmartContractState(wc.cfg.BtcFinalityContractAddress, string(queryMsgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	if dataFromContract == nil || dataFromContract.Data == nil || len(dataFromContract.Data.Bytes()) == 0 || strings.Contains(string(dataFromContract.Data), "null") {
		// expected when there is no PR commit at all
		return nil, nil
	}

	// Define a response struct
	var commit fptypes.PubRandCommit
	err = json.Unmarshal(dataFromContract.Data.Bytes(), &commit)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if err := commit.Validate(); err != nil {
		return nil, err
	}

	return &commit, nil
}

func (wc *CosmwasmConsumerController) QueryIsBlockFinalized(height uint64) (bool, error) {
	resp, err := wc.QueryIndexedBlock(height)
	if err != nil || resp == nil {
		return false, err
	}

	return resp.Finalized, nil
}

func (wc *CosmwasmConsumerController) QueryActivatedHeight() (uint64, error) {
	// Construct the query message
	queryMsg := QueryMsgActivatedHeight{
		ActivatedHeight: struct{}{},
	}

	// Marshal the query message to JSON
	queryMsgBytes, err := json.Marshal(queryMsg)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal query message: %w", err)
	}

	// Query the smart contract state
	dataFromContract, err := wc.QuerySmartContractState(wc.cfg.BtcStakingContractAddress, string(queryMsgBytes))
	if err != nil {
		return 0, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	// Unmarshal the response
	var resp struct {
		Height uint64 `json:"height"`
	}
	err = json.Unmarshal(dataFromContract.Data, &resp) // #nosec G115
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if resp.Height == 0 {
		return 0, fmt.Errorf("BTC staking is not activated yet")
	}

	// Return the activated height
	return resp.Height, nil
}

func (wc *CosmwasmConsumerController) QueryLatestBlockHeight() (uint64, error) {
	block, err := wc.queryCometBestBlock()
	if err != nil {
		return 0, err
	}

	return block.Height, nil
}

func (wc *CosmwasmConsumerController) QueryFinalitySignature(fpBtcPkHex string, height uint64) (*FinalitySignatureResponse, error) {
	queryMsgStruct := QueryMsgFinalitySignature{
		FinalitySignature: FinalitySignatureQuery{
			BtcPkHex: fpBtcPkHex,
			Height:   height,
		},
	}
	queryMsgBytes, err := json.Marshal(queryMsgStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query message: %w", err)
	}

	dataFromContract, err := wc.QuerySmartContractState(wc.cfg.BtcFinalityContractAddress, string(queryMsgBytes))
	if err != nil {
		return nil, err
	}

	var resp FinalitySignatureResponse
	err = json.Unmarshal(dataFromContract.Data, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (wc *CosmwasmConsumerController) QueryFinalityProviders() (*ConsumerFpsResponse, error) {
	queryMsgStruct := QueryMsgFinalityProviders{
		FinalityProviders: struct{}{},
	}

	queryMsgBytes, err := json.Marshal(queryMsgStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query message: %w", err)
	}

	dataFromContract, err := wc.QuerySmartContractState(wc.cfg.BtcStakingContractAddress, string(queryMsgBytes))
	if err != nil {
		return nil, err
	}

	var resp ConsumerFpsResponse
	err = json.Unmarshal(dataFromContract.Data, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (wc *CosmwasmConsumerController) QueryDelegations() (*ConsumerDelegationsResponse, error) {
	queryMsgStruct := QueryMsgDelegations{
		Delegations: struct{}{},
	}

	queryMsgBytes, err := json.Marshal(queryMsgStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query message: %w", err)
	}

	dataFromContract, err := wc.QuerySmartContractState(wc.cfg.BtcStakingContractAddress, string(queryMsgBytes))
	if err != nil {
		return nil, err
	}

	var resp ConsumerDelegationsResponse
	err = json.Unmarshal(dataFromContract.Data, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (wc *CosmwasmConsumerController) queryLatestBlocks(startAfter, limit *uint64, finalized, reverse *bool) ([]*fptypes.BlockInfo, error) {
	// Construct the query message
	queryMsg := QueryMsgBlocks{
		Blocks: BlocksQuery{
			StartAfter: startAfter,
			Limit:      limit,
			Finalized:  finalized,
			Reverse:    reverse,
		},
	}

	// Marshal the query message to JSON
	queryMsgBytes, err := json.Marshal(queryMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query message: %w", err)
	}

	// Query the smart contract state
	dataFromContract, err := wc.QuerySmartContractState(wc.cfg.BtcFinalityContractAddress, string(queryMsgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	// Unmarshal the response
	var resp BlocksResponse
	err = json.Unmarshal(dataFromContract.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Process the blocks and convert them to BlockInfo
	var blocks []*fptypes.BlockInfo
	for _, b := range resp.Blocks {
		block := &fptypes.BlockInfo{
			Height: b.Height,
			Hash:   b.AppHash,
		}
		blocks = append(blocks, block)
	}

	return blocks, nil
}

func (wc *CosmwasmConsumerController) queryCometBestBlock() (*fptypes.BlockInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wc.cfg.Timeout)
	defer cancel()

	// this will return 20 items at max in the descending order (highest first)
	chainInfo, err := wc.cwClient.RPCClient.BlockchainInfo(ctx, 0, 0)
	if err != nil {
		return nil, err
	}

	// Returning response directly, if header with specified number did not exist
	// at request will contain nil header
	return &fptypes.BlockInfo{
		Height: uint64(chainInfo.BlockMetas[0].Header.Height), // #nosec G115
		Hash:   chainInfo.BlockMetas[0].Header.AppHash,
	}, nil
}

func (wc *CosmwasmConsumerController) queryCometBlocksInRange(startHeight, endHeight uint64) ([]*fptypes.BlockInfo, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("the startHeight %v should not be higher than the endHeight %v", startHeight, endHeight)
	}

	ctx, cancel := context.WithTimeout(context.Background(), wc.cfg.Timeout)
	defer cancel()

	// this will return 20 items at max in the descending order (highest first)
	chainInfo, err := wc.cwClient.RPCClient.BlockchainInfo(ctx, int64(startHeight), int64(endHeight)) // #nosec G115
	if err != nil {
		return nil, err
	}

	// If no blocks found, return an empty slice
	if len(chainInfo.BlockMetas) == 0 {
		return nil, fmt.Errorf("no comet blocks found in the range")
	}

	// Process the blocks and convert them to BlockInfo
	var blocks []*fptypes.BlockInfo
	for _, blockMeta := range chainInfo.BlockMetas {
		block := &fptypes.BlockInfo{
			Height: uint64(blockMeta.Header.Height), // #nosec G115
			Hash:   blockMeta.Header.AppHash,
		}
		blocks = append(blocks, block)
	}

	// Sort the blocks by height in ascending order
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Height < blocks[j].Height
	})

	return blocks, nil
}

func (wc *CosmwasmConsumerController) Close() error {
	if !wc.cwClient.IsRunning() {
		return nil
	}

	return wc.cwClient.Stop()
}

func (wc *CosmwasmConsumerController) ExecuteBTCStakingContract(msgBytes []byte) (*babylonclient.RelayerTxResponse, error) {
	execMsg := &wasmdtypes.MsgExecuteContract{
		Sender:   wc.cwClient.MustGetAddr(),
		Contract: wc.cfg.BtcStakingContractAddress,
		Msg:      msgBytes,
	}

	res, err := wc.reliablySendMsg(execMsg, nil, nil)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (wc *CosmwasmConsumerController) ExecuteBTCFinalityContract(msgBytes []byte) (*babylonclient.RelayerTxResponse, error) {
	execMsg := &wasmdtypes.MsgExecuteContract{
		Sender:   wc.cwClient.MustGetAddr(),
		Contract: wc.cfg.BtcFinalityContractAddress,
		Msg:      msgBytes,
	}

	res, err := wc.reliablySendMsg(execMsg, nil, nil)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// QuerySmartContractState queries the smart contract state
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) QuerySmartContractState(contractAddress string, queryData string) (*wasmdtypes.QuerySmartContractStateResponse, error) {
	return wc.cwClient.QuerySmartContractState(context.Background(), contractAddress, queryData)
}

// StoreWasmCode stores the wasm code on the consumer chain
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) StoreWasmCode(wasmFile string) error {
	return wc.cwClient.StoreWasmCode(context.Background(), wasmFile)
}

// InstantiateContract instantiates a contract with the given code id and init msg
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) InstantiateContract(codeID uint64, initMsg []byte) error {
	return wc.cwClient.InstantiateContract(context.Background(), codeID, initMsg)
}

// GetLatestCodeID returns the latest wasm code id.
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) GetLatestCodeID() (uint64, error) {
	return wc.cwClient.GetLatestCodeID(context.Background())
}

// ListContractsByCode lists all contracts by wasm code id
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) ListContractsByCode(codeID uint64, pagination *sdkquerytypes.PageRequest) (*wasmdtypes.QueryContractsByCodeResponse, error) {
	return wc.cwClient.ListContractsByCode(context.Background(), codeID, pagination)
}

// SetBtcStakingContractAddress updates the BtcStakingContractAddress in the configuration
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) SetBtcStakingContractAddress(newAddress string) {
	wc.cfg.BtcStakingContractAddress = newAddress
}

// SetBtcFinalityContractAddress updates the BtcFinalityContractAddress in the configuration
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) SetBtcFinalityContractAddress(newAddress string) {
	wc.cfg.BtcFinalityContractAddress = newAddress
}

// MustGetValidatorAddress gets the validator address of the consumer chain
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) MustGetValidatorAddress() string {
	return wc.cwClient.MustGetAddr()
}

// GetCometNodeStatus gets the tendermint node status of the consumer chain
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) GetCometNodeStatus() (*coretypes.ResultStatus, error) {
	return wc.cwClient.GetStatus(context.Background())
}

// QueryIndexedBlock queries the indexed block at a given height
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) QueryIndexedBlock(height uint64) (*IndexedBlock, error) {
	// Construct the query message
	queryMsgStruct := QueryMsgBlock{
		Block: BlockQuery{
			Height: height,
		},
	}
	queryMsgBytes, err := json.Marshal(queryMsgStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query message: %w", err)
	}

	// Query the smart contract state
	dataFromContract, err := wc.QuerySmartContractState(wc.cfg.BtcFinalityContractAddress, string(queryMsgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	// Unmarshal the response
	var resp IndexedBlock
	err = json.Unmarshal(dataFromContract.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

func (wc *CosmwasmConsumerController) QueryFinalityActivationBlockHeight() (uint64, error) {
	// TODO: implement finality activation feature in OP stack L2
	return 0, nil
}

//nolint:revive
func (wc *CosmwasmConsumerController) QueryFinalityProviderHighestVotedHeight(fpPk *btcec.PublicKey) (uint64, error) {
	// TODO: implement highest voted height feature in OP stack L2
	return 0, nil
}

//nolint:revive
func (wc *CosmwasmConsumerController) QueryFinalityProviderSlashedOrJailed(fpPk *btcec.PublicKey) (bool, bool, error) {
	// TODO: implement slashed or jailed feature in OP stack L2
	return false, false, nil
}

//nolint:revive
func (wc *CosmwasmConsumerController) UnjailFinalityProvider(fpPk *btcec.PublicKey) (*fptypes.TxResponse, error) {
	// TODO: implement unjail feature in OP stack L2
	return nil, nil
}

package clientcontroller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/babylonlabs-io/babylon-sdk/x/babylon/types"
	fpcfg "github.com/babylonlabs-io/finality-provider/bsn/cosmos/config"
	cwcclient "github.com/babylonlabs-io/finality-provider/bsn/cosmos/cosmwasmclient/client"
	"github.com/cosmos/cosmos-sdk/client"

	sdkErr "cosmossdk.io/errors"
	wasmdparams "github.com/CosmWasm/wasmd/app/params"
	wasmdtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/babylonlabs-io/babylon/v3/client/babylonclient"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	finalitytypes "github.com/babylonlabs-io/babylon/v3/x/finality/types"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/finality-provider/signingcontext"
	fptypes "github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
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

func (wc *CosmwasmConsumerController) reliablySendMsg(ctx context.Context, msg sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*babylonclient.RelayerTxResponse, error) {
	return wc.reliablySendMsgs(ctx, []sdk.Msg{msg}, expectedErrs, unrecoverableErrs)
}

func (wc *CosmwasmConsumerController) reliablySendMsgs(ctx context.Context, msgs []sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*babylonclient.RelayerTxResponse, error) {
	resp, err := wc.cwClient.ReliablySendMsgs(
		ctx,
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to reliably send msgs: %w", err)
	}

	bbnResp := fptypes.NewBabylonTxResponse(resp)

	return bbnResp, nil
}

func (wc *CosmwasmConsumerController) GetClient() *cwcclient.Client {
	return wc.cwClient
}

func (wc *CosmwasmConsumerController) GetFpRandCommitContext() string {
	return signingcontext.FpRandCommitContextV0(wc.cfg.ChainID, wc.cfg.BtcFinalityContractAddress)
}

func (wc *CosmwasmConsumerController) GetFpFinVoteContext() string {
	return signingcontext.FpFinVoteContextV0(wc.cfg.ChainID, wc.cfg.BtcFinalityContractAddress)
}

// CommitPubRandList commits a list of Schnorr public randomness via a MsgCommitPubRand to Babylon
// it returns tx hash and error
func (wc *CosmwasmConsumerController) CommitPubRandList(
	ctx context.Context,
	req *api.CommitPubRandListRequest,
) (*fptypes.TxResponse, error) {
	bip340Sig := bbntypes.NewBIP340SignatureFromBTCSig(req.Sig).MustMarshal()

	// Construct the ExecMsg struct
	msg := ExecMsg{
		CommitPublicRandomness: &CommitPublicRandomness{
			FPPubKeyHex: bbntypes.NewBIP340PubKeyFromBTCPK(req.FpPk).MarshalHex(),
			StartHeight: req.StartHeight,
			NumPubRand:  req.NumPubRand,
			Commitment:  req.Commitment,
			Signature:   bip340Sig,
		},
	}

	// Marshal the ExecMsg struct to JSON
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ExecMsg: %w", err)
	}

	res, err := wc.ExecuteBTCFinalityContract(ctx, msgBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to execute BTC finality contract: %w", err)
	}

	return &fptypes.TxResponse{TxHash: res.TxHash}, nil
}

// SubmitBatchFinalitySigs submits a batch of finality signatures to Babylon
func (wc *CosmwasmConsumerController) SubmitBatchFinalitySigs(
	ctx context.Context,
	req *api.SubmitBatchFinalitySigsRequest,
) (*fptypes.TxResponse, error) {
	msgs := make([]sdk.Msg, 0, len(req.Blocks))
	for i, b := range req.Blocks {
		cmtProof := cmtcrypto.Proof{}
		if err := cmtProof.Unmarshal(req.ProofList[i]); err != nil {
			return nil, fmt.Errorf("failed to unmarshal proof: %w", err)
		}

		msg := ExecMsg{
			SubmitFinalitySignature: &SubmitFinalitySignature{
				FpPubkeyHex: bbntypes.NewBIP340PubKeyFromBTCPK(req.FpPk).MarshalHex(),
				Height:      b.GetHeight(),
				PubRand:     bbntypes.NewSchnorrPubRandFromFieldVal(req.PubRandList[i]).MustMarshal(),
				Proof:       cmtProof,
				BlockHash:   b.GetHash(),
				Signature:   bbntypes.NewSchnorrEOTSSigFromModNScalar(req.Sigs[i]).MustMarshal(),
			},
		}

		msgBytes, err := json.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal ExecMsg: %w", err)
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

	res, err := wc.reliablySendMsgs(ctx, msgs, expectedErrs, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to reliably send batch finality sigs: %w", err)
	}

	return &fptypes.TxResponse{TxHash: res.TxHash}, nil
}

// QueryFinalityProviderHasPower queries whether the finality provider has voting power at a given height
func (wc *CosmwasmConsumerController) QueryFinalityProviderHasPower(
	ctx context.Context,
	req *api.QueryFinalityProviderHasPowerRequest,
) (bool, error) {
	fpBtcPkHex := bbntypes.NewBIP340PubKeyFromBTCPK(req.FpPk).MarshalHex()

	queryMsgStruct := QueryMsgFinalityProviderPower{
		FinalityProviderPower: FinalityProviderPowerQuery{
			BtcPkHex: fpBtcPkHex,
			Height:   req.BlockHeight,
		},
	}
	queryMsgBytes, err := json.Marshal(queryMsgStruct)
	if err != nil {
		return false, fmt.Errorf("failed to marshal query message: %w", err)
	}
	dataFromContract, err := wc.QuerySmartContractState(ctx, wc.cfg.BtcFinalityContractAddress, string(queryMsgBytes))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// this is expected when the FP has no power at the given height
			return false, nil
		}

		return false, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	var resp ConsumerFpPowerResponse

	if err = json.Unmarshal(dataFromContract.Data, &resp); err != nil {
		return false, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return resp.Power > 0, nil
}

func (wc *CosmwasmConsumerController) QueryFinalityProvidersByPower(ctx context.Context) (*ConsumerFpsByPowerResponse, error) {
	queryMsgStruct := QueryMsgFinalityProvidersByPower{
		FinalityProvidersByTotalActiveSats: struct{}{},
	}

	queryMsgBytes, err := json.Marshal(queryMsgStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query message: %w", err)
	}

	dataFromContract, err := wc.QuerySmartContractState(ctx, wc.cfg.BtcStakingContractAddress, string(queryMsgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	var resp ConsumerFpsByPowerResponse
	err = json.Unmarshal(dataFromContract.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

func (wc *CosmwasmConsumerController) QueryLatestFinalizedBlock(ctx context.Context) (fptypes.BlockDescription, error) {
	isFinalized := true
	limit := uint64(1)
	blocks, err := wc.queryLatestBlocks(ctx, nil, &limit, &isFinalized, nil)
	if err != nil || len(blocks) == 0 {
		// do not return error here as FP handles this situation by
		// not running fast sync
		//nolint:nilerr
		return nil, nil
	}

	return blocks[0], nil
}

func (wc *CosmwasmConsumerController) QueryBlocks(ctx context.Context, req *api.QueryBlocksRequest) ([]fptypes.BlockDescription, error) {
	return wc.queryCometBlocksInRange(ctx, req.StartHeight, req.EndHeight)
}

func (wc *CosmwasmConsumerController) QueryBlock(ctx context.Context, height uint64) (fptypes.BlockDescription, error) {
	block, err := wc.cwClient.GetBlock(ctx, int64(height)) // #nosec G115
	if err != nil {
		return nil, fmt.Errorf("failed to get block at height %d: %w", height, err)
	}

	// #nosec G115
	return fptypes.NewBlockInfo(uint64(block.Block.Header.Height), block.Block.Header.AppHash, false), nil
}

// QueryLastPublicRandCommit returns the last public randomness commitments
func (wc *CosmwasmConsumerController) QueryLastPublicRandCommit(ctx context.Context, fpPk *btcec.PublicKey) (*fptypes.PubRandCommit, error) {
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
	dataFromContract, err := wc.QuerySmartContractState(ctx, wc.cfg.BtcFinalityContractAddress, string(queryMsgBytes))
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
		return nil, fmt.Errorf("failed to validate pub rand commit: %w", err)
	}

	return &commit, nil
}

func (wc *CosmwasmConsumerController) QueryIsBlockFinalized(ctx context.Context, height uint64) (bool, error) {
	resp, err := wc.QueryIndexedBlock(ctx, height)
	if err != nil || resp == nil {
		return false, err
	}

	return resp.Finalized, nil
}

func (wc *CosmwasmConsumerController) QueryLatestBlockHeight(ctx context.Context) (uint64, error) {
	block, err := wc.queryCometBestBlock(ctx)
	if err != nil {
		return 0, err
	}

	return block.GetHeight(), nil
}

func (wc *CosmwasmConsumerController) QueryFinalityActivationBlockHeight(ctx context.Context) (uint64, error) {
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
	dataFromContract, err := wc.QuerySmartContractState(ctx, wc.cfg.BtcStakingContractAddress, string(queryMsgBytes))
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

	return resp.Height, nil
}

func (wc *CosmwasmConsumerController) QueryLatestBlock(ctx context.Context) (fptypes.BlockDescription, error) {
	block, err := wc.queryCometBestBlock(ctx)
	if err != nil {
		return nil, err
	}

	return block, nil
}

func (wc *CosmwasmConsumerController) QueryFinalityProviderHighestVotedHeight(_ context.Context, _ *btcec.PublicKey) (uint64, error) {
	// TODO: implement highest voted height feature in OP stack L2
	return 0, nil
}

func (wc *CosmwasmConsumerController) QueryFinalityProviderStatus(_ context.Context, _ *btcec.PublicKey) (*api.FinalityProviderStatusResponse, error) {
	// TODO: implement slashed or jailed feature in OP stack L2
	return &api.FinalityProviderStatusResponse{
		Slashed: false,
		Jailed:  false,
	}, nil
}

func (wc *CosmwasmConsumerController) UnjailFinalityProvider(_ context.Context, _ *btcec.PublicKey) (*fptypes.TxResponse, error) {
	// TODO: implement unjail feature in OP stack L2
	return nil, nil
}

func (wc *CosmwasmConsumerController) QueryFinalitySignature(ctx context.Context, fpBtcPkHex string, height uint64) (*FinalitySignatureResponse, error) {
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

	dataFromContract, err := wc.QuerySmartContractState(ctx, wc.cfg.BtcFinalityContractAddress, string(queryMsgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	var resp FinalitySignatureResponse
	err = json.Unmarshal(dataFromContract.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if len(resp.Signature) == 0 {
		return nil, fmt.Errorf("finality signature not found")
	}

	return &resp, nil
}

func (wc *CosmwasmConsumerController) QueryFinalityProviders(ctx context.Context) (*ConsumerFpsResponse, error) {
	queryMsgStruct := QueryMsgFinalityProviders{
		FinalityProviders: struct{}{},
	}

	queryMsgBytes, err := json.Marshal(queryMsgStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query message: %w", err)
	}

	dataFromContract, err := wc.QuerySmartContractState(ctx, wc.cfg.BtcStakingContractAddress, string(queryMsgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	var resp ConsumerFpsResponse
	err = json.Unmarshal(dataFromContract.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

func (wc *CosmwasmConsumerController) QueryDelegations(ctx context.Context) (*ConsumerDelegationsResponse, error) {
	queryMsgStruct := QueryMsgDelegations{
		Delegations: struct{}{},
	}

	queryMsgBytes, err := json.Marshal(queryMsgStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query message: %w", err)
	}

	dataFromContract, err := wc.QuerySmartContractState(ctx, wc.cfg.BtcStakingContractAddress, string(queryMsgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	var resp ConsumerDelegationsResponse
	err = json.Unmarshal(dataFromContract.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

func (wc *CosmwasmConsumerController) queryLatestBlocks(ctx context.Context, startAfter, limit *uint64, finalized, reverse *bool) ([]*fptypes.BlockInfo, error) {
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
	dataFromContract, err := wc.QuerySmartContractState(ctx, wc.cfg.BtcFinalityContractAddress, string(queryMsgBytes))
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
		blocks = append(blocks, fptypes.NewBlockInfo(b.Height, b.AppHash, b.Finalized))
	}

	return blocks, nil
}

func (wc *CosmwasmConsumerController) queryCometBestBlock(ctx context.Context) (*fptypes.BlockInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, wc.cfg.Timeout)
	defer cancel()

	// this will return 20 items at max in the descending order (highest first)
	chainInfo, err := wc.cwClient.RPCClient.BlockchainInfo(ctx, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to query comet best block: %w", err)
	}

	// Returning response directly, if header with specified number did not exist
	// at request will contain nil header
	// #nosec G115
	return fptypes.NewBlockInfo(
		uint64(chainInfo.BlockMetas[0].Header.Height),
		chainInfo.BlockMetas[0].Header.AppHash,
		false,
	), nil
}

func (wc *CosmwasmConsumerController) queryCometBlocksInRange(ctx context.Context, startHeight, endHeight uint64) ([]fptypes.BlockDescription, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("the startHeight %v should not be higher than the endHeight %v", startHeight, endHeight)
	}

	ctx, cancel := context.WithTimeout(ctx, wc.cfg.Timeout)
	defer cancel()

	// this will return 20 items at max in the descending order (highest first)
	chainInfo, err := wc.cwClient.RPCClient.BlockchainInfo(ctx, int64(startHeight), int64(endHeight)) // #nosec G115
	if err != nil {
		return nil, fmt.Errorf("failed to query comet blocks in range: %w", err)
	}

	// If no blocks found, return an empty slice
	if len(chainInfo.BlockMetas) == 0 {
		return nil, fmt.Errorf("no comet blocks found in the range")
	}

	// Process the blocks and convert them to BlockInfo
	var blocks []fptypes.BlockDescription
	for _, blockMeta := range chainInfo.BlockMetas {
		// #nosec G115
		blocks = append(blocks, fptypes.NewBlockInfo(uint64(blockMeta.Header.Height), blockMeta.Header.AppHash, false))
	}

	// Sort the blocks by height in ascending order
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].GetHeight() < blocks[j].GetHeight()
	})

	return blocks, nil
}

func (wc *CosmwasmConsumerController) Close() error {
	if !wc.cwClient.IsRunning() {
		return nil
	}

	if err := wc.cwClient.Stop(); err != nil {
		return fmt.Errorf("failed to stop cosmwasm client: %w", err)
	}

	return nil
}

func (wc *CosmwasmConsumerController) ExecuteBTCStakingContract(ctx context.Context, msgBytes []byte) (*babylonclient.RelayerTxResponse, error) {
	execMsg := &wasmdtypes.MsgExecuteContract{
		Sender:   wc.cwClient.MustGetAddr(),
		Contract: wc.cfg.BtcStakingContractAddress,
		Msg:      msgBytes,
	}

	res, err := wc.reliablySendMsg(ctx, execMsg, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to reliably send msg: %w", err)
	}

	return res, nil
}

func (wc *CosmwasmConsumerController) ExecuteBTCFinalityContract(ctx context.Context, msgBytes []byte) (*babylonclient.RelayerTxResponse, error) {
	execMsg := &wasmdtypes.MsgExecuteContract{
		Sender:   wc.cwClient.MustGetAddr(),
		Contract: wc.cfg.BtcFinalityContractAddress,
		Msg:      msgBytes,
	}

	res, err := wc.reliablySendMsg(ctx, execMsg, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to reliably send msg: %w", err)
	}

	return res, nil
}

// QuerySmartContractState queries the smart contract state
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) QuerySmartContractState(ctx context.Context, contractAddress string, queryData string) (*wasmdtypes.QuerySmartContractStateResponse, error) {
	res, err := wc.cwClient.QuerySmartContractState(ctx, contractAddress, queryData)
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	return res, nil
}

// StoreWasmCode stores the wasm code on the consumer chain
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) StoreWasmCode(wasmFile string) error {
	if err := wc.cwClient.StoreWasmCode(context.Background(), wasmFile); err != nil {
		return fmt.Errorf("failed to store wasm code: %w", err)
	}

	return nil
}

// InstantiateContract instantiates a contract with the given code id and init msg
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) InstantiateContract(codeID uint64, initMsg []byte) error {
	if err := wc.cwClient.InstantiateContract(context.Background(), codeID, initMsg); err != nil {
		return fmt.Errorf("failed to instantiate contract: %w", err)
	}

	return nil
}

// GetLatestCodeID returns the latest wasm code id.
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) GetLatestCodeID() (uint64, error) {
	codeID, err := wc.cwClient.GetLatestCodeID(context.Background())
	if err != nil {
		return 0, fmt.Errorf("failed to get latest code ID: %w", err)
	}

	return codeID, nil
}

// ListContractsByCode lists all contracts by wasm code id
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) ListContractsByCode(codeID uint64, pagination *sdkquerytypes.PageRequest) (*wasmdtypes.QueryContractsByCodeResponse, error) {
	res, err := wc.cwClient.ListContractsByCode(context.Background(), codeID, pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to list contracts by code: %w", err)
	}

	return res, nil
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
	res, err := wc.cwClient.GetStatus(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get comet node status: %w", err)
	}

	return res, nil
}

// QueryIndexedBlock queries the indexed block at a given height
// NOTE: this function is only meant to be used in tests.
func (wc *CosmwasmConsumerController) QueryIndexedBlock(ctx context.Context, height uint64) (*IndexedBlock, error) {
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
	dataFromContract, err := wc.QuerySmartContractState(ctx, wc.cfg.BtcFinalityContractAddress, string(queryMsgBytes))
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

// QueryLastBTCTimestampedHeader - used for testing purposes
func (wc *CosmwasmConsumerController) QueryLastBTCTimestampedHeader(ctx context.Context) (*ConsumerHeaderResponse, error) {
	queryMsgStruct := QueryMsgLastConsumerHeader{
		LastConsumerHeader: struct{}{},
	}
	queryMsgBytes, err := json.Marshal(queryMsgStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query message: %w", err)
	}

	dataFromContract, err := wc.QuerySmartContractState(ctx, wc.MustQueryBabylonContracts(ctx).BabylonContract, string(queryMsgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	var resp ConsumerHeaderResponse
	err = json.Unmarshal(dataFromContract.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

func (wc *CosmwasmConsumerController) MustQueryBabylonContracts(ctx context.Context) *BabylonContracts {
	clientCtx := client.Context{Client: wc.cwClient.RPCClient}
	queryClient := types.NewQueryClient(clientCtx)

	resp, err := queryClient.BSNContracts(ctx, &types.QueryBSNContractsRequest{})
	if err != nil {
		panic(err)
	}

	return &BabylonContracts{
		BabylonContract:        resp.BsnContracts.BabylonContract,
		BtcLightClientContract: resp.BsnContracts.BtcLightClientContract,
		BtcStakingContract:     resp.BsnContracts.BtcStakingContract,
		BtcFinalityContract:    resp.BsnContracts.BtcFinalityContract,
	}
}

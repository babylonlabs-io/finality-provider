package clientcontroller

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	sdkErr "cosmossdk.io/errors"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/babylonlabs-io/babylon/v3/client/babylonclient"
	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	ckpttypes "github.com/babylonlabs-io/babylon/v3/x/checkpointing/types"
	rollupfpconfig "github.com/babylonlabs-io/finality-provider/bsn/rollup/config"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/finality-provider/signingcontext"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"go.uber.org/zap"
)

var (
	// ref: https://github.com/babylonlabs-io/rollup-bsn-contracts/blob/main/contracts/finality/src/error.rs#L87
	ErrBSNDuplicatedFinalitySig = sdkErr.Register("bsn_rollup", 1001, "Duplicated finality signature")
)

var _ api.ConsumerController = &RollupBSNController{}

// nolint:revive // Ignore stutter warning - full name provides clarity
type RollupBSNController struct {
	Cfg       *rollupfpconfig.RollupFPConfig
	ethClient *ethclient.Client
	bbnClient *bbnclient.Client
	logger    *zap.Logger
}

func NewRollupBSNController(
	rollupFPCfg *rollupfpconfig.RollupFPConfig,
	logger *zap.Logger,
) (*RollupBSNController, error) {
	if rollupFPCfg == nil {
		return nil, fmt.Errorf("nil config for rollup BSN controller")
	}
	if err := rollupFPCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ethClient, err := ethclient.Dial(rollupFPCfg.RollupNodeRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create rollup node client: %w", err)
	}

	babylonConfig := rollupFPCfg.GetBabylonConfig()
	if err := babylonConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config for Babylon client: %w", err)
	}

	bc, err := bbnclient.New(
		&babylonConfig,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Babylon client: %w", err)
	}

	cc := &RollupBSNController{
		Cfg:       rollupFPCfg,
		ethClient: ethClient,
		bbnClient: bc,
		logger:    logger,
	}

	return cc, nil
}

func (cc *RollupBSNController) QuerySmartContractState(ctx context.Context, contractAddress string, queryData string) (*wasmtypes.QuerySmartContractStateResponse, error) {
	clientCtx := client.Context{Client: cc.bbnClient.RPCClient}
	queryClient := wasmtypes.NewQueryClient(clientCtx)

	resp, err := queryClient.SmartContractState(ctx, &wasmtypes.QuerySmartContractStateRequest{
		Address:   contractAddress,
		QueryData: wasmtypes.RawContractMessage(queryData),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	return resp, nil
}

func (cc *RollupBSNController) ReliablySendMsg(ctx context.Context, msg sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*babylonclient.RelayerTxResponse, error) {
	return cc.reliablySendMsgs(ctx, []sdk.Msg{msg}, expectedErrs, unrecoverableErrs)
}

// QueryContractConfig queries the finality contract for its config
func (cc *RollupBSNController) QueryContractConfig(ctx context.Context) (*ContractConfig, error) {
	query := QueryMsg{
		Config: &ContractConfig{},
	}
	jsonData, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config query: %w", err)
	}

	stateResp, err := cc.QuerySmartContractState(ctx, cc.Cfg.FinalityContractAddress, string(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}
	if len(stateResp.Data) == 0 {
		return nil, fmt.Errorf("no config found")
	}

	var resp *ContractConfig
	err = json.Unmarshal(stateResp.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config response: %w", err)
	}

	return resp, nil
}

func (cc *RollupBSNController) reliablySendMsgs(ctx context.Context, msgs []sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*babylonclient.RelayerTxResponse, error) {
	resp, err := cc.bbnClient.ReliablySendMsgs(
		ctx,
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to reliably send messages: %w", err)
	}

	return resp, nil
}

func (cc *RollupBSNController) GetFpRandCommitContext() string {
	return signingcontext.FpRandCommitContextV0(cc.bbnClient.GetConfig().ChainID, cc.Cfg.FinalityContractAddress)
}

func (cc *RollupBSNController) GetFpFinVoteContext() string {
	return signingcontext.FpFinVoteContextV0(cc.bbnClient.GetConfig().ChainID, cc.Cfg.FinalityContractAddress)
}

// CommitPubRandList commits a list of Schnorr public randomness to Babylon CosmWasm contract
// it returns tx hash and error
func (cc *RollupBSNController) CommitPubRandList(
	ctx context.Context,
	req *api.CommitPubRandListRequest,
) (*types.TxResponse, error) {
	fpPkHex := bbntypes.NewBIP340PubKeyFromBTCPK(req.FpPk).MarshalHex()
	msg := CommitPublicRandomnessMsg{
		CommitPublicRandomness: CommitPublicRandomnessMsgParams{
			FpPubkeyHex: fpPkHex,
			StartHeight: req.StartHeight,
			NumPubRand:  req.NumPubRand,
			Commitment:  req.Commitment,
			Signature:   req.Sig.Serialize(),
		},
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal commit pubrand message: %w", err)
	}
	execMsg := &wasmtypes.MsgExecuteContract{
		Sender:   cc.bbnClient.MustGetAddr(),
		Contract: cc.Cfg.FinalityContractAddress,
		Msg:      payload,
	}

	res, err := cc.ReliablySendMsg(ctx, execMsg, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send commit pubrand message: %w", err)
	}
	cc.logger.Debug("Successfully committed public randomness",
		zap.String("fp_pk_hex", fpPkHex),
		zap.Uint64("start_height", req.StartHeight),
		zap.Uint64("num_pub_rand", req.NumPubRand),
	)

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

// SubmitBatchFinalitySigs submits a batch of finality signatures
func (cc *RollupBSNController) SubmitBatchFinalitySigs(
	ctx context.Context,
	req *api.SubmitBatchFinalitySigsRequest,
) (*types.TxResponse, error) {
	if len(req.Blocks) != len(req.Sigs) {
		return nil, fmt.Errorf("the number of blocks %v should match the number of finality signatures %v", len(req.Blocks), len(req.Sigs))
	}
	msgs := make([]sdk.Msg, 0, len(req.Blocks))
	fpPkHex := bbntypes.NewBIP340PubKeyFromBTCPK(req.FpPk).MarshalHex()
	for i, block := range req.Blocks {
		cmtProof := cmtcrypto.Proof{}
		if err := cmtProof.Unmarshal(req.ProofList[i]); err != nil {
			return nil, fmt.Errorf("failed to unmarshal proof: %w", err)
		}

		msg := SubmitFinalitySignatureMsg{
			SubmitFinalitySignature: SubmitFinalitySignatureMsgParams{
				FpPubkeyHex: fpPkHex,
				Height:      block.GetHeight(),
				PubRand:     bbntypes.NewSchnorrPubRandFromFieldVal(req.PubRandList[i]).MustMarshal(),
				Proof:       convertProof(cmtProof),
				BlockHash:   block.GetHash(),
				Signature:   bbntypes.NewSchnorrEOTSSigFromModNScalar(req.Sigs[i]).MustMarshal(),
			},
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal finality signature message: %w", err)
		}
		execMsg := &wasmtypes.MsgExecuteContract{
			Sender:   cc.bbnClient.MustGetAddr(),
			Contract: cc.Cfg.FinalityContractAddress,
			Msg:      payload,
		}
		msgs = append(msgs, execMsg)
	}

	expectedErrs := []*sdkErr.Error{
		ErrBSNDuplicatedFinalitySig,
	}

	res, err := cc.reliablySendMsgs(ctx, msgs, expectedErrs, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send finality signature messages: %w", err)
	}

	if res == nil {
		return &types.TxResponse{}, nil
	}

	cc.logger.Debug(
		"Successfully submitted finality signatures in a batch",
		zap.String("fp_pk_hex", fpPkHex),
		zap.Uint64("start_height", req.Blocks[0].GetHeight()),
		zap.Uint64("end_height", req.Blocks[len(req.Blocks)-1].GetHeight()),
	)

	return &types.TxResponse{TxHash: res.TxHash, Events: res.Events}, nil
}

// QueryFinalityProviderHasPower queries whether the finality provider has voting power at a given height
func (cc *RollupBSNController) QueryFinalityProviderHasPower(
	ctx context.Context,
	req *api.QueryFinalityProviderHasPowerRequest,
) (bool, error) {
	// Step 1: Check if finality signatures are allowed for this height (activation + interval)
	eligible, err := cc.isEligibleForFinalitySignature(ctx, req.BlockHeight)
	if err != nil {
		return false, fmt.Errorf("failed to check finality signature eligibility: %w", err)
	}
	if !eligible {
		cc.logger.Debug(
			"FP has 0 voting power - finality signatures not eligible for this height",
			zap.Uint64("height", req.BlockHeight),
		)

		return false, nil
	}

	// Step 2: Validate public randomness (exists, covers height, and is timestamped)
	if !cc.hasTimestampedPubRandomness(ctx, req.FpPk, req.BlockHeight) {
		return false, nil
	}

	// Step 3: Check if FP has active BTC delegations (voting power)
	fpBtcPkHex := bbntypes.NewBIP340PubKeyFromBTCPK(req.FpPk).MarshalHex()
	hasActiveDelegation, err := cc.hasActiveBTCDelegation(fpBtcPkHex)
	if err != nil {
		return false, fmt.Errorf("failed to check active BTC delegations: %w", err)
	}
	if !hasActiveDelegation {
		cc.logger.Debug(
			"FP has 0 voting power - no active BTC delegation",
			zap.String("fp_btc_pk", fpBtcPkHex),
			zap.Uint64("height", req.BlockHeight),
		)

		return false, nil
	}

	cc.logger.Debug(
		"FP has voting power - all checks passed",
		zap.String("fp_btc_pk", fpBtcPkHex),
		zap.Uint64("height", req.BlockHeight),
	)

	return true, nil
}

// hasActiveBTCDelegation checks if the finality provider has any active BTC delegations
func (cc *RollupBSNController) hasActiveBTCDelegation(fpBtcPkHex string) (bool, error) {
	btcStakingParams, err := cc.bbnClient.QueryClient.BTCStakingParams()
	if err != nil {
		return false, fmt.Errorf("failed to query BTC staking params: %w", err)
	}

	var nextKey []byte
	for {
		resp, err := cc.bbnClient.QueryClient.FinalityProviderDelegations(fpBtcPkHex, &sdkquerytypes.PageRequest{Key: nextKey, Limit: 100})
		if err != nil {
			return false, fmt.Errorf("failed to query finality provider delegations: %w", err)
		}

		for _, btcDels := range resp.BtcDelegatorDelegations {
			for _, btcDel := range btcDels.Dels {
				active, err := cc.isDelegationActive(btcStakingParams, btcDel)
				if err != nil {
					continue // Skip invalid delegations but continue checking others
				}
				if active {
					return true, nil
				}
			}
		}

		if resp.Pagination == nil || resp.Pagination.NextKey == nil {
			break
		}
		nextKey = resp.Pagination.NextKey
	}

	return false, nil
}

// hasTimestampedPubRandomness validates that the FP has public randomness that:
// 1. Exists (has committed public randomness that covers the specific height)
// 2. Is Bitcoin timestamped (finalized)
func (cc *RollupBSNController) hasTimestampedPubRandomness(ctx context.Context, fpPk *btcec.PublicKey, blockHeight uint64) bool {
	// NOTE: This query has O(1) best case (recent commits) to O(n) worst case complexity,
	// where n is the number of PR commits for this FP. The contract scans commits in
	// descending order by start_height and stops at the first match.
	pubRand, err := cc.QueryPubRandCommitForHeight(ctx, fpPk, blockHeight)
	if err != nil {
		cc.logger.Debug(
			"FP has 0 voting power - failed to query public randomness commitment for height",
			zap.String("fp_btc_pk", bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex()),
			zap.Uint64("height", blockHeight),
			zap.Error(err),
		)

		return false
	}
	if pubRand == nil {
		cc.logger.Debug(
			"FP has 0 voting power - no public randomness commitment covers this height",
			zap.String("fp_btc_pk", bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex()),
			zap.Uint64("height", blockHeight),
		)

		return false
	}

	// Check if this specific public randomness is Bitcoin timestamped (finalized)
	lastFinalizedCkpt, err := cc.bbnClient.LatestEpochFromStatus(ckpttypes.Finalized)
	if err != nil {
		cc.logger.Debug(
			"FP has 0 voting power - failed to query last finalized checkpoint",
			zap.String("fp_btc_pk", bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex()),
			zap.Uint64("height", blockHeight),
			zap.Error(err),
		)

		return false
	}
	if pubRand.BabylonEpoch > lastFinalizedCkpt.RawCheckpoint.EpochNum {
		cc.logger.Debug(
			"FP has 0 voting power - public randomness epoch not yet finalized",
			zap.String("fp_btc_pk", bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex()),
			zap.Uint64("height", blockHeight),
			zap.Uint64("pub_rand_epoch", pubRand.BabylonEpoch),
			zap.Uint64("last_finalized_epoch", lastFinalizedCkpt.RawCheckpoint.EpochNum),
		)

		return false
	}

	cc.logger.Debug(
		"FP has valid timestamped public randomness",
		zap.String("fp_btc_pk", bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex()),
		zap.Uint64("height", blockHeight),
		zap.Uint64("pub_rand_start_height", pubRand.StartHeight),
		zap.Uint64("num_pub_rand", pubRand.NumPubRand),
		zap.Uint64("pub_rand_epoch", pubRand.BabylonEpoch),
		zap.Uint64("last_finalized_epoch", lastFinalizedCkpt.RawCheckpoint.EpochNum),
	)

	return true
}

// QueryLatestFinalizedBlock returns the finalized L2 block from a RPC call
// NOTE: FP program cannot know if block is btc finalized or not, so it uses the last
// block that is finalized by Ethereum, which is a stronger notion than BTC staking finalized
func (cc *RollupBSNController) QueryLatestFinalizedBlock(ctx context.Context) (types.BlockDescription, error) {
	l2Block, err := cc.ethClient.HeaderByNumber(ctx, big.NewInt(ethrpc.FinalizedBlockNumber.Int64()))
	if err != nil {
		return nil, fmt.Errorf("failed to get finalized block header: %w", err)
	}

	if l2Block.Number.Uint64() == 0 {
		return nil, nil
	}

	return types.NewBlockInfo(l2Block.Number.Uint64(), l2Block.Hash().Bytes(), false), nil
}

func (cc *RollupBSNController) QueryBlocks(ctx context.Context, req *api.QueryBlocksRequest) ([]types.BlockDescription, error) {
	if req.StartHeight > req.EndHeight {
		return nil, fmt.Errorf("the start height %v should not be higher than the end height %v", req.StartHeight, req.EndHeight)
	}
	// limit the number of blocks to query
	count := req.EndHeight - req.StartHeight + 1
	if req.Limit > 0 && count >= uint64(req.Limit) {
		count = uint64(req.Limit)
	}

	// create batch requests
	blockHeaders := make([]*ethtypes.Header, count)
	batchElemList := make([]ethrpc.BatchElem, count)
	for i := range batchElemList {
		batchElemList[i] = ethrpc.BatchElem{
			Method: "eth_getBlockByNumber",
			Args:   []interface{}{hexutil.EncodeUint64(req.StartHeight + uint64(i)), false}, // #nosec G115
			Result: &blockHeaders[i],
		}
	}

	// batch call
	if err := cc.ethClient.Client().BatchCallContext(ctx, batchElemList); err != nil {
		return nil, fmt.Errorf("failed to batch call context: %w", err)
	}
	for i := range batchElemList {
		if batchElemList[i].Error != nil {
			return nil, batchElemList[i].Error
		}
		if blockHeaders[i] == nil {
			return nil, fmt.Errorf("got null header for block %d", req.StartHeight+uint64(i)) // #nosec G115
		}
	}

	// convert to types.BlockInfo
	var blocks []types.BlockDescription
	for _, header := range blockHeaders {
		blocks = append(blocks, types.NewBlockInfo(header.Number.Uint64(), header.Hash().Bytes(), false))
	}
	cc.logger.Debug(
		"Successfully batch query blocks",
		zap.Uint64("start_height", req.StartHeight),
		zap.Uint64("end_height", req.EndHeight),
		zap.Uint32("limit", req.Limit),
		zap.String("last_block_hash", hex.EncodeToString(blocks[len(blocks)-1].GetHash())),
	)

	return blocks, nil
}

// QueryBlock returns the L2 block number and block hash with the given block number from a RPC call
func (cc *RollupBSNController) QueryBlock(ctx context.Context, height uint64) (types.BlockDescription, error) {
	l2Block, err := cc.ethClient.HeaderByNumber(ctx, new(big.Int).SetUint64(height))
	if err != nil {
		return nil, fmt.Errorf("failed to get block header by number: %w", err)
	}

	blockHashBytes := l2Block.Hash().Bytes()
	cc.logger.Debug(
		"QueryBlock",
		zap.Uint64("height", height),
		zap.String("block_hash", hex.EncodeToString(blockHashBytes)),
	)

	return types.NewBlockInfo(height, blockHashBytes, false), nil
}

// Note: this is specific to the RollupBSNController and only used for testing
// QueryBlock returns the Ethereum block from a RPC call
// nolint:unused
func (cc *RollupBSNController) queryEthBlock(ctx context.Context, height uint64) (*ethtypes.Header, error) {
	header, err := cc.ethClient.HeaderByNumber(ctx, new(big.Int).SetUint64(height))
	if err != nil {
		return nil, fmt.Errorf("failed to get header by number: %w", err)
	}

	return header, nil
}

// QueryIsBlockFinalized returns whether the given the L2 block number has been finalized
func (cc *RollupBSNController) QueryIsBlockFinalized(ctx context.Context, height uint64) (bool, error) {
	l2Block, err := cc.QueryLatestFinalizedBlock(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to query latest finalized block: %w", err)
	}

	if l2Block == nil {
		return false, nil
	}
	if height > l2Block.GetHeight() {
		return false, nil
	}

	return true, nil
}

// QueryLatestBlockHeight gets the latest rollup block number from a RPC call
func (cc *RollupBSNController) QueryLatestBlock(ctx context.Context) (types.BlockDescription, error) {
	l2LatestBlock, err := cc.ethClient.HeaderByNumber(ctx, big.NewInt(ethrpc.LatestBlockNumber.Int64()))
	if err != nil {
		return nil, fmt.Errorf("failed to get latest block header: %w", err)
	}

	return types.NewBlockInfo(l2LatestBlock.Number.Uint64(), l2LatestBlock.Hash().Bytes(), false), nil
}

// QueryFirstPubRandCommit returns the first public randomness commitment
// It is fetched from the state of a CosmWasm contract OP finality gadget.
func (cc *RollupBSNController) QueryFirstPubRandCommit(ctx context.Context, fpPk *btcec.PublicKey) (*RollupPubRandCommit, error) {
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)
	queryMsg := &QueryMsg{
		FirstPubRandCommit: &PubRandCommit{
			BtcPkHex: fpPubKey.MarshalHex(),
		},
	}

	jsonData, err := json.Marshal(queryMsg)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling to JSON: %w", err)
	}

	stateResp, err := cc.QuerySmartContractState(ctx, cc.Cfg.FinalityContractAddress, string(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}
	if len(stateResp.Data) == 0 {
		return nil, nil
	}

	var resp *RollupPubRandCommit
	err = json.Unmarshal(stateResp.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if resp == nil {
		return nil, nil
	}
	if err := resp.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate response: %w", err)
	}

	return resp, nil
}

// QueryLastPublicRandCommit returns the last public randomness commitments
// It is fetched from the state of a CosmWasm contract OP finality gadget.
func (cc *RollupBSNController) QueryLastPublicRandCommit(ctx context.Context, fpPk *btcec.PublicKey) (types.PubRandCommit, error) {
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)
	queryMsg := &QueryMsg{
		LastPubRandCommit: &PubRandCommit{
			BtcPkHex: fpPubKey.MarshalHex(),
		},
	}

	jsonData, err := json.Marshal(queryMsg)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling to JSON: %w", err)
	}

	stateResp, err := cc.QuerySmartContractState(ctx, cc.Cfg.FinalityContractAddress, string(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}
	if len(stateResp.Data) == 0 {
		return nil, nil
	}

	var resp *RollupPubRandCommit
	err = json.Unmarshal(stateResp.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if resp == nil {
		return nil, nil
	}
	if err := resp.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate response: %w", err)
	}

	return resp, nil
}

// QueryPubRandCommitForHeight returns the public randomness commitment that covers a specific height
func (cc *RollupBSNController) QueryPubRandCommitForHeight(ctx context.Context, fpPk *btcec.PublicKey, height uint64) (*RollupPubRandCommit, error) {
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)
	queryMsg := &QueryMsg{
		PubRandCommitForHeight: &PubRandCommitForHeightQuery{
			BtcPkHex: fpPubKey.MarshalHex(),
			Height:   height,
		},
	}

	jsonData, err := json.Marshal(queryMsg)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling to JSON: %w", err)
	}

	stateResp, err := cc.QuerySmartContractState(ctx, cc.Cfg.FinalityContractAddress, string(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}
	if len(stateResp.Data) == 0 {
		return nil, nil
	}

	var resp *RollupPubRandCommit
	err = json.Unmarshal(stateResp.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if resp == nil {
		return nil, nil
	}
	if err := resp.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate response: %w", err)
	}

	return resp, nil
}

// isEligibleForFinalitySignature checks if finality signatures are allowed for the given height
// based on the contract's BSN activation and interval requirements
func (cc *RollupBSNController) isEligibleForFinalitySignature(ctx context.Context, height uint64) (bool, error) {
	config, err := cc.QueryContractConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to query contract config: %w", err)
	}

	// Check BSN activation
	if height < config.BsnActivationHeight {
		cc.logger.Debug(
			"Block height is before BSN activation",
			zap.Uint64("height", height),
			zap.Uint64("bsn_activation_height", config.BsnActivationHeight),
		)

		return false, nil
	}

	// Check finality signature interval
	if (height-config.BsnActivationHeight)%config.FinalitySignatureInterval != 0 {
		cc.logger.Debug(
			"Block height is not at scheduled finality signature interval",
			zap.Uint64("height", height),
			zap.Uint64("bsn_activation_height", config.BsnActivationHeight),
			zap.Uint64("finality_signature_interval", config.FinalitySignatureInterval),
		)

		return false, nil
	}

	return true, nil
}

func (cc *RollupBSNController) QueryFinalityActivationBlockHeight(ctx context.Context) (uint64, error) {
	config, err := cc.QueryContractConfig(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to query contract config: %w", err)
	}

	return config.BsnActivationHeight, nil
}

func (cc *RollupBSNController) QueryFinalityProviderHighestVotedHeight(ctx context.Context, fpPk *btcec.PublicKey) (uint64, error) {
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)

	queryMsg := &QueryMsg{
		HighestVotedHeight: &HighestVotedHeightQuery{
			BtcPkHex: fpPubKey.MarshalHex(),
		},
	}

	jsonData, err := json.Marshal(queryMsg)
	if err != nil {
		return 0, fmt.Errorf("failed marshaling to JSON: %w", err)
	}

	stateResp, err := cc.QuerySmartContractState(ctx, cc.Cfg.FinalityContractAddress, string(jsonData))
	if err != nil {
		return 0, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	// The contract returns Option<u64> as JSON: either a number or null
	var heightResponse *uint64
	err = json.Unmarshal(stateResp.Data, &heightResponse)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// If the response is null (None in Rust), the finality provider has never voted
	if heightResponse == nil {
		return 0, nil
	}

	return *heightResponse, nil
}

func (cc *RollupBSNController) QueryFinalityProviderStatus(_ context.Context, fpPk *btcec.PublicKey) (*api.FinalityProviderStatusResponse, error) {
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)
	res, err := cc.bbnClient.QueryClient.FinalityProvider(fpPubKey.MarshalHex())
	if err != nil {
		return nil, fmt.Errorf("failed to query the finality provider %s: %w", fpPubKey.MarshalHex(), err)
	}

	return api.NewFinalityProviderStatusResponse(
		res.FinalityProvider.SlashedBtcHeight > 0,
		false, // always return false, there is no jail/unjail feature in rollup BSN
	), nil
}

func (cc *RollupBSNController) UnjailFinalityProvider(_ context.Context, _ *btcec.PublicKey) (*types.TxResponse, error) {
	// always return nil, there is no jail/unjail feature in rollup BSN
	return nil, nil
}

// QueryFinalityProviderInAllowlist queries whether the finality provider is in the allowlist
func (cc *RollupBSNController) QueryFinalityProviderInAllowlist(ctx context.Context, fpPk *btcec.PublicKey) (bool, error) {
	// Query the contract for allowed finality providers
	query := QueryMsg{
		AllowedFinalityProviders: &struct{}{},
	}
	jsonData, err := json.Marshal(query)
	if err != nil {
		return false, fmt.Errorf("failed to marshal allowlist query: %w", err)
	}

	stateResp, err := cc.QuerySmartContractState(ctx, cc.Cfg.FinalityContractAddress, string(jsonData))
	if err != nil {
		return false, fmt.Errorf("failed to query smart contract state: %w", err)
	}
	if len(stateResp.Data) == 0 {
		return false, fmt.Errorf("no allowlist data found")
	}

	var allowedFPs AllowedFinalityProvidersResponse
	err = json.Unmarshal(stateResp.Data, &allowedFPs)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal allowlist response: %w", err)
	}

	// Check if the FP public key is in the allowlist
	fpPkHex := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex()
	for _, allowedFpPkHex := range allowedFPs {
		if allowedFpPkHex == fpPkHex {
			cc.logger.Debug("Finality provider found in allowlist",
				zap.String("fp_pk_hex", fpPkHex))

			return true, nil
		}
	}

	cc.logger.Debug("Finality provider not found in allowlist",
		zap.String("fp_pk_hex", fpPkHex))

	return false, nil
}

func convertProof(cmtProof cmtcrypto.Proof) Proof {
	return Proof{
		Total:    uint64(cmtProof.Total), // #nosec G115
		Index:    uint64(cmtProof.Index), // #nosec G115
		LeafHash: cmtProof.LeafHash,
		Aunts:    cmtProof.Aunts,
	}
}

func (cc *RollupBSNController) Close() error {
	cc.ethClient.Close()

	if !cc.bbnClient.IsRunning() {
		return nil
	}

	if err := cc.bbnClient.Stop(); err != nil {
		return fmt.Errorf("failed to stop Babylon client: %w", err)
	}

	return nil
}

// nolint:unparam
func (cc *RollupBSNController) isDelegationActive(
	btcStakingParams *btcstakingtypes.QueryParamsResponse,
	btcDel *btcstakingtypes.BTCDelegationResponse,
) (bool, error) {
	covQuorum := btcStakingParams.GetParams().CovenantQuorum
	ud := btcDel.UndelegationResponse

	if ud.DelegatorUnbondingInfoResponse != nil {
		return false, nil
	}

	if len(btcDel.CovenantSigs) < int(covQuorum) { // #nosec G115
		return false, nil
	}
	if len(ud.CovenantUnbondingSigList) < int(covQuorum) { // #nosec G115
		return false, nil
	}
	if len(ud.CovenantSlashingSigs) < int(covQuorum) { // #nosec G115
		return false, nil
	}

	return true, nil
}

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
	finalitytypes "github.com/babylonlabs-io/babylon/v3/x/finality/types"
	rollupfpconfig "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/config"
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

// queryContractConfig queries the finality contract for its config
// nolint:unused
func (cc *RollupBSNController) queryContractConfig(ctx context.Context) (*Config, error) {
	query := QueryMsg{
		Config: &Config{},
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

	var resp *Config
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

	return types.NewBabylonTxResponse(resp), nil
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
		finalitytypes.ErrDuplicatedFinalitySig,
		finalitytypes.ErrSigHeightOutdated,
	}

	res, err := cc.reliablySendMsgs(ctx, msgs, expectedErrs, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send finality signature messages: %w", err)
	}
	cc.logger.Debug(
		"Successfully submitted finality signatures in a batch",
		zap.String("fp_pk_hex", fpPkHex),
		zap.Uint64("start_height", req.Blocks[0].GetHeight()),
		zap.Uint64("end_height", req.Blocks[len(req.Blocks)-1].GetHeight()),
	)

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

// QueryFinalityProviderHasPower queries whether the finality provider has voting power at a given height
func (cc *RollupBSNController) QueryFinalityProviderHasPower(
	ctx context.Context,
	req *api.QueryFinalityProviderHasPowerRequest,
) (bool, error) {
	pubRand, err := cc.QueryLastPublicRandCommit(ctx, req.FpPk)
	if err != nil {
		return false, fmt.Errorf("failed to query last public rand commit: %w", err)
	}
	if pubRand == nil {
		return false, nil
	}

	// fp has 0 voting power if there is no public randomness at this height
	lastCommittedPubRandHeight := pubRand.EndHeight()
	cc.logger.Debug(
		"FP last committed public randomness",
		zap.Uint64("height", lastCommittedPubRandHeight),
	)
	// TODO: Handle the case where public randomness is not consecutive.
	// For example, if the FP is down for a while and misses some public randomness commits:
	// Assume blocks 1-100 and 200-300 have public randomness, and lastCommittedPubRandHeight is 300.
	// For blockHeight 101 to 199, even though blockHeight < lastCommittedPubRandHeight,
	// the finality provider should have 0 voting power.
	if req.BlockHeight > lastCommittedPubRandHeight {
		cc.logger.Debug(
			"FP has 0 voting power because there is no public randomness at this height",
			zap.Uint64("height", req.BlockHeight),
		)

		return false, nil
	}

	fpBtcPkHex := bbntypes.NewBIP340PubKeyFromBTCPK(req.FpPk).MarshalHex()
	var nextKey []byte
	btcStakingParams, err := cc.bbnClient.QueryClient.BTCStakingParams()
	if err != nil {
		return false, fmt.Errorf("failed to query BTC staking params: %w", err)
	}
	for {
		resp, err := cc.bbnClient.QueryClient.FinalityProviderDelegations(fpBtcPkHex, &sdkquerytypes.PageRequest{Key: nextKey, Limit: 100})
		if err != nil {
			return false, fmt.Errorf("failed to query finality provider delegations: %w", err)
		}

		for _, btcDels := range resp.BtcDelegatorDelegations {
			for _, btcDel := range btcDels.Dels {
				active, err := cc.isDelegationActive(btcStakingParams, btcDel)
				if err != nil {
					continue
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
	cc.logger.Debug(
		"FP has 0 voting power because there is no BTC delegation",
		zap.String("fp_btc_pk", fpBtcPkHex),
		zap.Uint64("height", req.BlockHeight),
	)

	return false, nil
}

// QueryLatestFinalizedBlock returns the finalized L2 block from a RPC call
// TODO: return the BTC finalized L2 block, it is tricky b/c it's not recorded anywhere so we can
// use some exponential strategy to search
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

// QueryLastPublicRandCommit returns the last public randomness commitments
// It is fetched from the state of a CosmWasm contract OP finality gadget.
func (cc *RollupBSNController) QueryLastPublicRandCommit(ctx context.Context, fpPk *btcec.PublicKey) (*types.PubRandCommit, error) {
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

	var resp *types.PubRandCommit
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

func (cc *RollupBSNController) QueryFinalityActivationBlockHeight(_ context.Context) (uint64, error) {
	// TODO: implement finality activation feature in OP stack L2
	return 0, nil
}

func (cc *RollupBSNController) QueryFinalityProviderHighestVotedHeight(_ context.Context, _ *btcec.PublicKey) (uint64, error) {
	// TODO: implement highest voted height feature in OP stack L2
	return 0, nil
}

func (cc *RollupBSNController) QueryFinalityProviderStatus(_ context.Context, _ *btcec.PublicKey) (*api.FinalityProviderStatusResponse, error) {
	// TODO: implement slashed or jailed feature in OP stack L2
	return &api.FinalityProviderStatusResponse{
		Slashed: false,
		Jailed:  false,
	}, nil
}

func (cc *RollupBSNController) UnjailFinalityProvider(_ context.Context, _ *btcec.PublicKey) (*types.TxResponse, error) {
	// TODO: implement unjail feature in OP stack L2
	return nil, nil
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

package clientcontroller

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"go.uber.org/zap"
)

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
func (cc *RollupBSNController) QueryLatestBlockHeight(ctx context.Context) (uint64, error) {
	l2LatestBlock, err := cc.ethClient.HeaderByNumber(ctx, big.NewInt(ethrpc.LatestBlockNumber.Int64()))
	if err != nil {
		return 0, fmt.Errorf("failed to get latest block header: %w", err)
	}

	return l2LatestBlock.Number.Uint64(), nil
}

// QueryActivatedHeight returns the rollup block number at which the finality gadget is activated.
func (cc *RollupBSNController) QueryActivatedHeight(_ context.Context) (uint64, error) {
	// TODO: implement finality activation feature in rollup
	return 0, nil
}

// QueryFinalityActivationBlockHeight returns the rollup block number at which the finality gadget is activated.
func (cc *RollupBSNController) QueryFinalityActivationBlockHeight(_ context.Context) (uint64, error) {
	// TODO: implement finality activation feature in OP stack L2
	return 0, nil
}
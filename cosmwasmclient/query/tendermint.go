package query

import (
	"context"
	"strings"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
)

// GetStatus returns the status of the tendermint node
func (c *QueryClient) GetStatus(ctx context.Context) (*coretypes.ResultStatus, error) {
	return c.RPCClient.Status(ctx)
}

// GetBlock returns the tendermint block at a specific height
func (c *QueryClient) GetBlock(ctx context.Context, height int64) (*coretypes.ResultBlock, error) {
	return c.RPCClient.Block(ctx, &height)
}

// BlockSearch searches for blocks satisfying the events specified on the events list
func (c *QueryClient) BlockSearch(ctx context.Context, events []string, page *int, perPage *int, orderBy string) (*coretypes.ResultBlockSearch, error) {
	return c.RPCClient.BlockSearch(ctx, strings.Join(events, " AND "), page, perPage, orderBy)
}

// TxSearch searches for transactions satisfying the events specified on the events list
func (c *QueryClient) TxSearch(ctx context.Context, events []string, prove bool, page *int, perPage *int, orderBy string) (*coretypes.ResultTxSearch, error) {
	return c.RPCClient.TxSearch(ctx, strings.Join(events, " AND "), prove, page, perPage, orderBy)
}

// GetTx returns the transaction with the specified hash
func (c *QueryClient) GetTx(ctx context.Context, hash []byte) (*coretypes.ResultTx, error) {
	return c.RPCClient.Tx(ctx, hash, false)
}

func (c *QueryClient) Subscribe(ctx context.Context, subscriber, query string, outCapacity ...int) (<-chan coretypes.ResultEvent, error) {
	return c.RPCClient.Subscribe(ctx, subscriber, query, outCapacity...)
}

func (c *QueryClient) Unsubscribe(ctx context.Context, subscriber, query string) error {
	return c.RPCClient.Unsubscribe(ctx, subscriber, query)
}

func (c *QueryClient) UnsubscribeAll(ctx context.Context, subscriber string) error {
	return c.RPCClient.UnsubscribeAll(ctx, subscriber)
}

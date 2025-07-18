package query

import (
	"context"
	"fmt"
	"strings"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
)

// GetStatus returns the status of the tendermint node
func (c *QueryClient) GetStatus(ctx context.Context) (*coretypes.ResultStatus, error) {
	status, err := c.RPCClient.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	return status, nil
}

// GetBlock returns the tendermint block at a specific height
func (c *QueryClient) GetBlock(ctx context.Context, height int64) (*coretypes.ResultBlock, error) {
	block, err := c.RPCClient.Block(ctx, &height)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	return block, nil
}

// BlockSearch searches for blocks satisfying the events specified on the events list
func (c *QueryClient) BlockSearch(ctx context.Context, events []string, page *int, perPage *int, orderBy string) (*coretypes.ResultBlockSearch, error) {
	result, err := c.RPCClient.BlockSearch(ctx, strings.Join(events, " AND "), page, perPage, orderBy)
	if err != nil {
		return nil, fmt.Errorf("failed to search blocks: %w", err)
	}

	return result, nil
}

// TxSearch searches for transactions satisfying the events specified on the events list
func (c *QueryClient) TxSearch(ctx context.Context, events []string, prove bool, page *int, perPage *int, orderBy string) (*coretypes.ResultTxSearch, error) {
	result, err := c.RPCClient.TxSearch(ctx, strings.Join(events, " AND "), prove, page, perPage, orderBy)
	if err != nil {
		return nil, fmt.Errorf("failed to search transactions: %w", err)
	}

	return result, nil
}

// GetTx returns the transaction with the specified hash
func (c *QueryClient) GetTx(ctx context.Context, hash []byte) (*coretypes.ResultTx, error) {
	tx, err := c.RPCClient.Tx(ctx, hash, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	return tx, nil
}

func (c *QueryClient) Subscribe(ctx context.Context, subscriber, query string, outCapacity ...int) (<-chan coretypes.ResultEvent, error) {
	eventChan, err := c.RPCClient.Subscribe(ctx, subscriber, query, outCapacity...)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	return eventChan, nil
}

func (c *QueryClient) Unsubscribe(ctx context.Context, subscriber, query string) error {
	if err := c.RPCClient.Unsubscribe(ctx, subscriber, query); err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}

	return nil
}

func (c *QueryClient) UnsubscribeAll(ctx context.Context, subscriber string) error {
	if err := c.RPCClient.UnsubscribeAll(ctx, subscriber); err != nil {
		return fmt.Errorf("failed to unsubscribe all: %w", err)
	}

	return nil
}

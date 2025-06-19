package service

import (
	"context"
	"github.com/babylonlabs-io/finality-provider/types"
)

type BlockPoller[T types.BlockDescription] interface {
	// NextBlock returns the next block or blocks until one is available
	NextBlock(ctx context.Context) (T, error)

	// SetStartHeight configures where to begin polling
	SetStartHeight(height uint64) error

	// CurrentHeight returns the last successfully returned block height
	CurrentHeight() uint64
}

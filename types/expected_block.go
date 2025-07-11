package types

import (
	"context"
)

type BlockDescription interface {
	GetHeight() uint64
	GetHash() []byte
	IsFinalized() bool
	MsgToSign(signCtx string) []byte // this is the message that will be signed by the eots signer
}

type BlockPoller[T BlockDescription] interface {
	// NextBlock returns the next block
	NextBlock(ctx context.Context) (T, error)

	// TryNextBlock non-blocking version of NextBlock
	TryNextBlock() (T, bool)

	// SetStartHeight configures where to begin polling
	SetStartHeight(ctx context.Context, height uint64) error

	// NextHeight returns the next height to poll for
	NextHeight() uint64

	// Stop stops the poller
	Stop() error
}

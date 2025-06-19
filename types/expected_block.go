package types

import (
	"context"
)

type BlockDescription interface {
	GetHeight() uint64
	GetHash() []byte
	IsFinalized() bool
	MsgToSign() []byte // this is the message that will be signed by the eots signer
}

type BlockPoller[T BlockDescription] interface {
	// TryNextBlock returns the next block if one is available
	TryNextBlock() (T, bool)

	// SetStartHeight configures where to begin polling
	SetStartHeight(ctx context.Context, height uint64) error

	// CurrentHeight returns the last successfully returned block height
	CurrentHeight() uint64

	Stop() error
}

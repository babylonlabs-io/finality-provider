package types

import (
	"context"
	"github.com/babylonlabs-io/babylon/v3/types"
)

// LastVotedHeightProvider provides access to finality provider state
type LastVotedHeightProvider func() (lastVotedHeight uint64, err error)

// HeightDeterminer defines the interface for determining the start height
// when a finality provider instance begins processing blocks.
type HeightDeterminer interface {
	// DetermineStartHeight calculates the appropriate height to start processing blocks.
	DetermineStartHeight(ctx context.Context, btcPk *types.BIP340PubKey, lastVotedHeight LastVotedHeightProvider) (uint64, error)
}

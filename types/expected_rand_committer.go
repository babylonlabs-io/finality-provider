//nolint:revive
package types

import (
	"context"

	"github.com/babylonlabs-io/babylon/v4/types"
)

// RandomnessCommitter defines the interface for randomness commitment operations.
// It abstracts the logic for determining when to commit randomness and performing
// the actual commitment to allow for different implementation strategies.
type RandomnessCommitter interface {
	// ShouldCommit determines whether a new randomness commitment should be made.
	// It returns:
	// - should: whether randomness should be committed
	// - startHeight: the height from which to start the commitment
	// - error: any error that occurred during the determination
	ShouldCommit(ctx context.Context) (should bool, startHeight uint64, err error)

	// Commit performs the actual randomness commitment starting from the given height.
	// It returns the transaction response or an error if the commitment fails.
	Commit(ctx context.Context, startHeight uint64) (*TxResponse, error)

	// GetLastCommittedHeight retrieves the last height at which randomness was committed.
	GetLastCommittedHeight(ctx context.Context) (uint64, error)

	// GetPubRandProofList retrieves a list of public randomness proofs for the given height.
	GetPubRandProofList(height uint64, numPubRand uint64) ([][]byte, error)

	Init(btcPk *types.BIP340PubKey, chainID []byte) error
}

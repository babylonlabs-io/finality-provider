package clientcontroller

import (
	"fmt"
	"github.com/babylonlabs-io/finality-provider/types"
)

var _ types.PubRandCommit = (*RollupPubRandCommit)(nil)

// RollupPubRandCommit represents the Rollup-specific public randomness commitment
type RollupPubRandCommit struct {
	StartHeight  uint64 `json:"start_height"`
	NumPubRand   uint64 `json:"num_pub_rand"`
	Interval     uint64 `json:"interval"`
	BabylonEpoch uint64 `json:"babylon_epoch"`
	Commitment   []byte `json:"commitment"`
}

func (r *RollupPubRandCommit) GetStartHeight() uint64 {
	return r.StartHeight
}

func (r *RollupPubRandCommit) GetNumPubRand() uint64 {
	return r.NumPubRand
}

func (r *RollupPubRandCommit) GetCommitment() []byte {
	return r.Commitment
}

// EndHeight returns the last height for which randomness actually exists in this commitment.
// For sparse commitments, randomness is generated only at specific intervals, not consecutively.
//
// Example with StartHeight=60, NumPubRand=5, Interval=5:
//   - Randomness exists for heights: 60, 65, 70, 75, 80
//   - EndHeight() returns 80 (the last height with actual randomness)
//   - Heights 61-64, 66-69, 71-74, 76-79, 81+ have NO randomness
//
// The ShouldCommit function is responsible for computing the next eligible start height
// and ensuring no gaps or overlaps by using proper alignment logic.
func (r *RollupPubRandCommit) GetEndHeight() uint64 {
	return r.StartHeight + (r.NumPubRand-1)*r.Interval
}

func (r *RollupPubRandCommit) Validate() error {
	if r.NumPubRand < 1 {
		return fmt.Errorf("NumPubRand must be >= 1, got %d", r.NumPubRand)
	}

	return nil
}

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

func (r *RollupPubRandCommit) EndHeight() uint64 { return r.StartHeight + r.NumPubRand - 1 }

func (r *RollupPubRandCommit) Validate() error {
	if r.NumPubRand < 1 {
		return fmt.Errorf("NumPubRand must be >= 1, got %d", r.NumPubRand)
	}

	return nil
}

package babylon

import (
	"fmt"
	"github.com/babylonlabs-io/finality-provider/types"
)

var _ types.PubRandCommit = (*BabylonPubRandCommit)(nil)

// BabylonPubRandCommit represents the Babylon-specific public randomness commitment response
//
//nolint:revive
type BabylonPubRandCommit struct {
	StartHeight uint64 `json:"start_height"`
	NumPubRand  uint64 `json:"num_pub_rand"`
	EpochNum    uint64 `json:"epoch_num"`
	Commitment  []byte `json:"commitment"`
}

func (b *BabylonPubRandCommit) GetStartHeight() uint64 {
	return b.StartHeight
}

func (b *BabylonPubRandCommit) GetNumPubRand() uint64 {
	return b.NumPubRand
}

func (b *BabylonPubRandCommit) GetCommitment() []byte {
	return b.Commitment
}

func (b *BabylonPubRandCommit) EndHeight() uint64 { return b.StartHeight + b.NumPubRand - 1 }

func (b *BabylonPubRandCommit) Validate() error {
	if b.NumPubRand < 1 {
		return fmt.Errorf("NumPubRand must be >= 1, got %d", b.NumPubRand)
	}

	return nil
}

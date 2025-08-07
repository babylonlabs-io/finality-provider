package clientcontroller

import (
	"fmt"
	"github.com/babylonlabs-io/finality-provider/types"
)

var _ types.PubRandCommit = (*CosmosPubRandCommit)(nil)

// CosmosPubRandCommit represents the Cosmos-specific public randomness commitment response
type CosmosPubRandCommit struct {
	StartHeight uint64 `json:"start_height"`
	NumPubRand  uint64 `json:"num_pub_rand"`
	Commitment  []byte `json:"commitment"`
	Height      uint64 `json:"height"`
}

func (c *CosmosPubRandCommit) GetStartHeight() uint64 {
	return c.StartHeight
}

func (c *CosmosPubRandCommit) GetNumPubRand() uint64 {
	return c.NumPubRand
}

func (c *CosmosPubRandCommit) GetCommitment() []byte {
	return c.Commitment
}

func (c *CosmosPubRandCommit) EndHeight() uint64 { return c.StartHeight + c.NumPubRand - 1 }

func (c *CosmosPubRandCommit) Validate() error {
	if c.NumPubRand < 1 {
		return fmt.Errorf("NumPubRand must be >= 1, got %d", c.NumPubRand)
	}

	return nil
}

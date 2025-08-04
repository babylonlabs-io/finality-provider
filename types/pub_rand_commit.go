package types

import (
	bbn "github.com/babylonlabs-io/babylon/v3/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/cometbft/cometbft/crypto/merkle"
)

// PubRandCommit interface abstracts epoch information across different sources
type PubRandCommit interface {
	EndHeight() uint64
	Validate() error
}

// GetPubRandCommitAndProofs commits a list of public randomness and returns
// the commitment (i.e., Merkle root) and all Merkle proofs
func GetPubRandCommitAndProofs(pubRandList []*btcec.FieldVal) ([]byte, []*merkle.Proof) {
	prBytesList := make([][]byte, 0, len(pubRandList))
	for _, pr := range pubRandList {
		prBytesList = append(prBytesList, bbn.NewSchnorrPubRandFromFieldVal(pr).MustMarshal())
	}

	return merkle.ProofsFromByteSlices(prBytesList)
}

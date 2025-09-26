package randgenerator

import (
	"crypto/hmac"
	"crypto/sha256"

	"github.com/babylonlabs-io/babylon/v4/crypto/eots"
	"github.com/btcsuite/btcd/btcec/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// GenerateRandomness generates a random scalar with the given key and src
// the result is deterministic with each given input
func GenerateRandomness(key []byte, chainID []byte, height uint64) (*eots.PrivateRand, *eots.PublicRand) {
	var randScalar btcec.ModNScalar

	// add iteration as part of HMAC generation to get a uniformly random
	// value for randScalar via rejection sampling
	iteration := uint64(0)
	for {
		// calculate the random hash with iteration count
		digest := hmac.New(sha256.New, key)
		digest.Write(append(append(sdk.Uint64ToBigEndian(height), chainID...), sdk.Uint64ToBigEndian(iteration)...))
		randPre := digest.Sum(nil)

		// increase iteration count and sample again until overflow or zero does not
		// happen. It is fine as the chance of overflow or zero is very small (2^-128)
		overflow := randScalar.SetByteSlice(randPre)
		if !overflow && !randScalar.IsZero() {
			break
		}
		iteration++
	}

	// convert the valid scalar into private random
	privRand := secp256k1.NewPrivateKey(&randScalar)
	var j secp256k1.JacobianPoint
	privRand.PubKey().AsJacobian(&j)

	return &privRand.Key, &j.X
}

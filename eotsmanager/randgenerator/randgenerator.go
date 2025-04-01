package randgenerator

import (
	"crypto/hmac"
	"crypto/sha256"

	"github.com/babylonlabs-io/babylon/crypto/eots"
	"github.com/btcsuite/btcd/btcec/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// GenerateRandomness generates a random scalar with the given key and src
// the result is deterministic with each given input
func GenerateRandomness(key []byte, chainID []byte, height uint64) (*eots.PrivateRand, *eots.PublicRand) {
	// calculate the randomn hash of the key concatenated with chainID and height
	digest := hmac.New(sha256.New, key)
	digest.Write(append(sdk.Uint64ToBigEndian(height), chainID...))
	randPre := digest.Sum(nil)

	// convert the hash into private random
	var randScalar btcec.ModNScalar
	randScalar.SetByteSlice(randPre)
	privRand := secp256k1.NewPrivateKey(&randScalar)
	var j secp256k1.JacobianPoint
	privRand.PubKey().AsJacobian(&j)

	return &privRand.Key, &j.X
}

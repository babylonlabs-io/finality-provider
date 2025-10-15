//nolint:revive
package util

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
)

// ValidatePrivKeyBytes validates that the private key bytes are valid for secp256k1.
// It checks that:
// 1. The private key is less than the secp256k1 curve order (no overflow)
// 2. The private key is not zero
//
// This validation is necessary because btcd's PrivKeyFromBytes does not perform
// these checks and passes the responsibility to callers.
func ValidatePrivKeyBytes(keyBytes []byte) error {
	var keyInt btcec.ModNScalar
	overflow := keyInt.SetByteSlice(keyBytes)
	if overflow {
		return fmt.Errorf("private key is greater than or equal to the secp256k1 curve order")
	}
	if keyInt.IsZero() {
		return fmt.Errorf("private key cannot be zero")
	}

	return nil
}

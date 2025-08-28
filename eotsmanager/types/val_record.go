//nolint:revive
package types

import "github.com/btcsuite/btcd/btcec/v2"

type KeyRecord struct {
	Name    string
	PrivKey *btcec.PrivateKey
}

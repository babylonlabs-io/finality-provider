package types

import (
	"github.com/btcsuite/btcd/btcec/v2"
)

type ChainKeyInfo struct {
	Name       string
	Mnemonic   string
	PublicKey  *btcec.PublicKey
	PrivateKey *btcec.PrivateKey
}

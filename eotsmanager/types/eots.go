package types

import "github.com/btcsuite/btcd/btcec/v2"

type EOTSRecord struct {
	Sig     *btcec.ModNScalar
	PubRand *btcec.FieldVal
}

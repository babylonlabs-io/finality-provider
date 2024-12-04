package store

import "github.com/babylonlabs-io/finality-provider/eotsmanager/proto"

type SigningRecord struct {
	BlockHash []byte // The hash of the block.
	PublicKey []byte // The public key used for signing.
	Signature []byte // The signature of the block.
	Timestamp int64  // The timestamp of the signing operation, in Unix seconds.
}

func (s *SigningRecord) FromProto(sr *proto.SigningRecord) {
	s.PublicKey = sr.PublicKey
	s.BlockHash = sr.BlockHash
	s.Timestamp = sr.Timestamp
	s.Signature = sr.Signature
}

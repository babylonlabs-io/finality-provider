package store

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/proto"
)

type SigningRecord struct {
	Msg       []byte // The message that the signature is signed over.
	Signature []byte
	Timestamp int64 // The timestamp of the signing operation, in Unix seconds.
}

func (s *SigningRecord) FromProto(sr *proto.SigningRecord) {
	s.Msg = sr.Msg
	s.Timestamp = sr.Timestamp
	s.Signature = sr.EotsSig
}

// the record key is (chainID || pk || height)
func getSignRecordKey(chainID, pk []byte, height uint64) []byte {
	// Convert height to bytes
	heightBytes := sdk.Uint64ToBigEndian(height)

	// Concatenate all components to create the key
	key := make([]byte, 0, len(pk)+len(chainID)+len(heightBytes))
	key = append(key, chainID...)
	key = append(key, pk...)
	key = append(key, heightBytes...)

	return key
}

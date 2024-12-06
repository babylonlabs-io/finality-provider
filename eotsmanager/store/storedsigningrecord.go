package store

import (
	"crypto/hmac"
	"crypto/sha256"

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

func getSignRecordKey(pk, chainID []byte, height uint64) []byte {
	// calculate the randomn hash of the key concatenated with chainID and height
	digest := hmac.New(sha256.New, pk)
	digest.Write(append(sdk.Uint64ToBigEndian(height), chainID...))
	return digest.Sum(nil)
}

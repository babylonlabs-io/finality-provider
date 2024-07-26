package service

import (
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/cometbft/cometbft/crypto/merkle"
)

type pubRandState struct {
	s *store.PubRandProofStore
}

func NewPubRandState(s *store.PubRandProofStore) *pubRandState {
	return &pubRandState{s: s}
}

func (st *pubRandState) AddPubRandProofList(
	pubRandList []*btcec.FieldVal,
	proofList []*merkle.Proof,
) error {
	return st.s.AddPubRandProofList(pubRandList, proofList)
}

func (st *pubRandState) GetPubRandProof(pubRand *btcec.FieldVal) ([]byte, error) {
	return st.s.GetPubRandProof(pubRand)
}

func (st *pubRandState) GetPubRandProofList(pubRandList []*btcec.FieldVal) ([][]byte, error) {
	return st.s.GetPubRandProofList(pubRandList)
}

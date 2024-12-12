package service

import (
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/cometbft/cometbft/crypto/merkle"
)

type pubRandState struct {
	s *store.PubRandProofStore
}

func newPubRandState(s *store.PubRandProofStore) *pubRandState {
	return &pubRandState{s: s}
}

func (st *pubRandState) addPubRandProofList(
	pubRandList []*btcec.FieldVal,
	proofList []*merkle.Proof,
) error {
	return st.s.AddPubRandProofList(pubRandList, proofList)
}

func (st *pubRandState) getPubRandProof(pubRand *btcec.FieldVal) ([]byte, error) {
	return st.s.GetPubRandProof(pubRand)
}

func (st *pubRandState) getPubRandProofList(pubRandList []*btcec.FieldVal) ([][]byte, error) {
	return st.s.GetPubRandProofList(pubRandList)
}

func (st *pubRandState) removePubRandProofList(pubRandList []*btcec.FieldVal) error {
	return st.s.RemovePubRandProofList(pubRandList)
}

package service

import (
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/cometbft/cometbft/crypto/merkle"
)

type pubRandState struct {
	s *store.PubRandProofStore
}

func newPubRandState(s *store.PubRandProofStore) *pubRandState {
	return &pubRandState{s: s}
}

func (st *pubRandState) addPubRandProofList(
	pk, chainID []byte, height uint64, numPubRand uint64,
	proofList []*merkle.Proof,
) error {
	return st.s.AddPubRandProofList(chainID, pk, height, numPubRand, proofList)
}

func (st *pubRandState) getPubRandProof(pk, chainID []byte, height uint64) ([]byte, error) {
	return st.s.GetPubRandProof(chainID, pk, height)
}

func (st *pubRandState) getPubRandProofList(pk, chainID []byte, height uint64, numPubRand uint64) ([][]byte, error) {
	return st.s.GetPubRandProofList(chainID, pk, height, numPubRand)
}

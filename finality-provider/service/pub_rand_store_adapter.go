package service

import (
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/cometbft/cometbft/crypto/merkle"
)

type PubRandState struct {
	s *store.PubRandProofStore
}

func NewPubRandState(s *store.PubRandProofStore) *PubRandState {
	return &PubRandState{s: s}
}

func (st *PubRandState) addPubRandProofList(
	pk, chainID []byte, height uint64, numPubRand uint64,
	proofList []*merkle.Proof,
) error {
	return st.s.AddPubRandProofList(chainID, pk, height, numPubRand, proofList)
}

func (st *PubRandState) getPubRandProof(pk, chainID []byte, height uint64) ([]byte, error) {
	return st.s.GetPubRandProof(chainID, pk, height)
}

func (st *PubRandState) getPubRandProofList(pk, chainID []byte, height uint64, numPubRand uint64) ([][]byte, error) {
	return st.s.GetPubRandProofList(chainID, pk, height, numPubRand)
}

func (st *PubRandState) close() error {
	return st.s.Close()
}

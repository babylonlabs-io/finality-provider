package service

import (
	"fmt"

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
	if err := st.s.AddPubRandProofList(chainID, pk, height, numPubRand, proofList); err != nil {
		return fmt.Errorf("failed to add pub rand proof list: %w", err)
	}

	return nil
}

func (st *PubRandState) getPubRandProof(pk, chainID []byte, height uint64) ([]byte, error) {
	proof, err := st.s.GetPubRandProof(chainID, pk, height)
	if err != nil {
		return nil, fmt.Errorf("failed to get pub rand proof: %w", err)
	}

	return proof, nil
}

func (st *PubRandState) getPubRandProofList(pk, chainID []byte, height uint64, numPubRand uint64) ([][]byte, error) {
	proofList, err := st.s.GetPubRandProofList(chainID, pk, height, numPubRand)
	if err != nil {
		return nil, fmt.Errorf("failed to get pub rand proof list: %w", err)
	}

	return proofList, nil
}

func (st *PubRandState) close() error {
	if err := st.s.Close(); err != nil {
		return fmt.Errorf("failed to close pub rand store: %w", err)
	}

	return nil
}

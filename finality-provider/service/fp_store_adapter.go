package service

import (
	"sync"

	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
)

type fpState struct {
	mu sync.Mutex
	fp *store.StoredFinalityProvider
	s  *store.FinalityProviderStore
}

func newFpState(
	fp *store.StoredFinalityProvider,
	s *store.FinalityProviderStore,
) *fpState {
	return &fpState{
		fp: fp,
		s:  s,
	}
}

func (fps *fpState) getStoreFinalityProvider() *store.StoredFinalityProvider {
	fps.mu.Lock()
	defer fps.mu.Unlock()

	return fps.fp
}

func (fps *fpState) setStatus(s proto.FinalityProviderStatus) error {
	fps.mu.Lock()
	fps.fp.Status = s
	fps.mu.Unlock()

	return fps.s.SetFpStatus(fps.fp.BtcPk, s)
}

func (fps *fpState) setLastVotedHeight(height uint64) error {
	fps.mu.Lock()
	fps.fp.LastVotedHeight = height
	fps.mu.Unlock()

	return fps.s.SetFpLastVotedHeight(fps.fp.BtcPk, height)
}

func (fp *FinalityProviderInstance) GetStoreFinalityProvider() *store.StoredFinalityProvider {
	return fp.fpState.getStoreFinalityProvider()
}

func (fp *FinalityProviderInstance) GetBtcPkBIP340() *bbntypes.BIP340PubKey {
	return fp.fpState.getStoreFinalityProvider().GetBIP340BTCPK()
}

func (fp *FinalityProviderInstance) GetBtcPk() *btcec.PublicKey {
	return fp.fpState.getStoreFinalityProvider().BtcPk
}

func (fp *FinalityProviderInstance) GetBtcPkHex() string {
	return fp.GetBtcPkBIP340().MarshalHex()
}

func (fp *FinalityProviderInstance) GetStatus() proto.FinalityProviderStatus {
	return fp.fpState.getStoreFinalityProvider().Status
}

func (fp *FinalityProviderInstance) GetLastVotedHeight() uint64 {
	return fp.fpState.getStoreFinalityProvider().LastVotedHeight
}

func (fp *FinalityProviderInstance) GetChainID() []byte {
	return []byte(fp.fpState.getStoreFinalityProvider().ChainID)
}

func (fp *FinalityProviderInstance) SetStatus(s proto.FinalityProviderStatus) error {
	return fp.fpState.setStatus(s)
}

func (fp *FinalityProviderInstance) MustSetStatus(s proto.FinalityProviderStatus) {
	if err := fp.SetStatus(s); err != nil {
		fp.logger.Fatal("failed to set finality-provider status",
			zap.String("pk", fp.GetBtcPkHex()), zap.String("status", s.String()))
	}
}

func (fp *FinalityProviderInstance) updateStateAfterFinalitySigSubmission(height uint64) error {
	return fp.fpState.setLastVotedHeight(height)
}

func (fp *FinalityProviderInstance) MustUpdateStateAfterFinalitySigSubmission(height uint64) {
	if err := fp.updateStateAfterFinalitySigSubmission(height); err != nil {
		fp.logger.Fatal("failed to update state after finality signature submitted",
			zap.String("pk", fp.GetBtcPkHex()), zap.Uint64("height", height))
	}
	fp.metrics.RecordFpLastVotedHeight(fp.GetBtcPkHex(), height)
	fp.metrics.RecordFpLastProcessedHeight(fp.GetBtcPkHex(), height)
}

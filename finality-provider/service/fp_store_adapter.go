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
	mu  sync.Mutex
	sfp *store.StoredFinalityProvider
	s   *store.FinalityProviderStore
}

func newFpState(
	fp *store.StoredFinalityProvider,
	s *store.FinalityProviderStore,
) *fpState {
	return &fpState{
		sfp: fp,
		s:   s,
	}
}

func (fps *fpState) withLock(action func()) {
	fps.mu.Lock()
	defer fps.mu.Unlock()

	action()
}

func (fps *fpState) setStatus(s proto.FinalityProviderStatus) error {
	fps.mu.Lock()
	fps.sfp.Status = s
	fps.mu.Unlock()

	return fps.s.SetFpStatus(fps.sfp.BtcPk, s)
}

func (fps *fpState) setLastVotedHeight(height uint64) error {
	fps.mu.Lock()
	fps.sfp.LastVotedHeight = height
	fps.mu.Unlock()

	return fps.s.SetFpLastVotedHeight(fps.sfp.BtcPk, height)
}

func (fp *FinalityProviderInstance) GetStoreFinalityProvider() *store.StoredFinalityProvider {
	var sfp *store.StoredFinalityProvider
	fp.fpState.withLock(func() {
		// Create a copy of the stored finality provider to prevent data races
		sfpCopy := *fp.fpState.sfp
		sfp = &sfpCopy
	})

	return sfp
}

func (fp *FinalityProviderInstance) GetBtcPkBIP340() *bbntypes.BIP340PubKey {
	var pk *bbntypes.BIP340PubKey
	fp.fpState.withLock(func() {
		pk = fp.fpState.sfp.GetBIP340BTCPK()
	})

	return pk
}

func (fp *FinalityProviderInstance) GetBtcPk() *btcec.PublicKey {
	var pk *btcec.PublicKey
	fp.fpState.withLock(func() {
		pk = fp.fpState.sfp.BtcPk
	})

	return pk
}

func (fp *FinalityProviderInstance) GetBtcPkHex() string {
	return fp.GetBtcPkBIP340().MarshalHex()
}

func (fp *FinalityProviderInstance) GetStatus() proto.FinalityProviderStatus {
	var status proto.FinalityProviderStatus
	fp.fpState.withLock(func() {
		status = fp.fpState.sfp.Status
	})

	return status
}

func (fp *FinalityProviderInstance) GetLastVotedHeight() uint64 {
	var lastVotedHeight uint64
	fp.fpState.withLock(func() {
		lastVotedHeight = fp.fpState.sfp.LastVotedHeight
	})

	return lastVotedHeight
}

func (fp *FinalityProviderInstance) GetChainID() []byte {
	var chainID string
	fp.fpState.withLock(func() {
		chainID = fp.fpState.sfp.ChainID
	})

	return []byte(chainID)
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

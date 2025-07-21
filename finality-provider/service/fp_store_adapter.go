package service

import (
	"fmt"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
	"sync"

	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
)

var _ types.FinalityProviderState = (*FpState)(nil)

type FpState struct {
	mu      sync.Mutex
	sfp     *store.StoredFinalityProvider
	s       *store.FinalityProviderStore
	metrics *metrics.FpMetrics
	logger  *zap.Logger
}

func NewFpState(
	fp *store.StoredFinalityProvider,
	s *store.FinalityProviderStore,
	logger *zap.Logger,
	metrics *metrics.FpMetrics,
) *FpState {
	return &FpState{
		sfp:     fp,
		s:       s,
		metrics: metrics,
		logger:  logger.With(zap.String("module", "fp_state")),
	}
}

func (fps *FpState) withLock(action func()) {
	fps.mu.Lock()
	defer fps.mu.Unlock()

	action()
}

func (fps *FpState) setStatus(s proto.FinalityProviderStatus) error {
	fps.mu.Lock()
	fps.sfp.Status = s
	fps.mu.Unlock()

	if err := fps.s.SetFpStatus(fps.sfp.BtcPk, s); err != nil {
		return fmt.Errorf("failed to set finality provider status: %w", err)
	}

	return nil
}

func (fps *FpState) setLastVotedHeight(height uint64) error {
	fps.mu.Lock()
	fps.sfp.LastVotedHeight = height
	fps.mu.Unlock()

	if err := fps.s.SetFpLastVotedHeight(fps.sfp.BtcPk, height); err != nil {
		return fmt.Errorf("failed to set finality provider last voted height: %w", err)
	}

	return nil
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
	fp.metrics.RecordFpVoteTime(fp.GetBtcPkHex())
	fp.metrics.RecordFpLastVotedHeight(fp.GetBtcPkHex(), height)
	fp.metrics.RecordFpLastProcessedHeight(fp.GetBtcPkHex(), height)
}

func (fps *FpState) GetBtcPk() *btcec.PublicKey {
	var pk *btcec.PublicKey
	fps.withLock(func() {
		pk = fps.sfp.BtcPk
	})

	return pk
}

func (fps *FpState) GetBtcPkBIP340() *bbntypes.BIP340PubKey {
	var pk *bbntypes.BIP340PubKey
	fps.withLock(func() {
		pk = fps.sfp.GetBIP340BTCPK()
	})

	return pk
}

func (fps *FpState) GetBtcPkHex() string {
	return fps.GetBtcPkBIP340().MarshalHex()
}

func (fps *FpState) GetChainID() []byte {
	var chainID string
	fps.withLock(func() {
		chainID = fps.sfp.ChainID
	})

	return []byte(chainID)
}

func (fps *FpState) GetLastVotedHeight() uint64 {
	var lastVotedHeight uint64
	fps.withLock(func() {
		lastVotedHeight = fps.sfp.LastVotedHeight
	})

	return lastVotedHeight
}

func (fps *FpState) GetStatus() proto.FinalityProviderStatus {
	var status proto.FinalityProviderStatus
	fps.withLock(func() {
		status = fps.sfp.Status
	})

	return status
}

func (fps *FpState) SetStatus(s proto.FinalityProviderStatus) error {
	fps.mu.Lock()
	fps.sfp.Status = s
	fps.mu.Unlock()

	if err := fps.s.SetFpStatus(fps.sfp.BtcPk, s); err != nil {
		return fmt.Errorf("failed to set finality provider status: %w", err)
	}

	fps.logger.Debug("finality provider status updated",
		zap.String("pk", fps.GetBtcPkHex()),
		zap.String("status", s.String()))

	return nil
}

func (fps *FpState) SetLastVotedHeight(height uint64) error {
	fps.mu.Lock()
	fps.sfp.LastVotedHeight = height
	fps.mu.Unlock()

	if err := fps.s.SetFpLastVotedHeight(fps.sfp.BtcPk, height); err != nil {
		return fmt.Errorf("failed to set finality provider last voted height: %w", err)
	}

	fps.logger.Debug("finality provider last voted height updated",
		zap.String("pk", fps.GetBtcPkHex()),
		zap.Uint64("height", height))

	// Record metrics if available
	if fps.metrics != nil {
		fps.metrics.RecordFpVoteTime(fps.GetBtcPkHex())
		fps.metrics.RecordFpLastVotedHeight(fps.GetBtcPkHex(), height)
		fps.metrics.RecordFpLastProcessedHeight(fps.GetBtcPkHex(), height)
	}

	return nil
}

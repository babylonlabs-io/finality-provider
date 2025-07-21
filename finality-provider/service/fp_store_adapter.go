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

	if fps.metrics != nil {
		fps.metrics.RecordFpVoteTime(fps.GetBtcPkHex())
		fps.metrics.RecordFpLastVotedHeight(fps.GetBtcPkHex(), height)
		fps.metrics.RecordFpLastProcessedHeight(fps.GetBtcPkHex(), height)
	}

	return nil
}

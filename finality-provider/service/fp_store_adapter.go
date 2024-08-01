package service

import (
	"sync"

	sdkmath "cosmossdk.io/math"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
)

type createFinalityProviderResponse struct {
	FpInfo *proto.FinalityProviderInfo
}

type createFinalityProviderRequest struct {
	keyName         string
	passPhrase      string
	hdPath          string
	chainID         string
	description     *stakingtypes.Description
	commission      *sdkmath.LegacyDec
	errResponse     chan error
	successResponse chan *createFinalityProviderResponse
}

type registerFinalityProviderRequest struct {
	chainID   string
	fpAddr    sdk.AccAddress
	btcPubKey *bbntypes.BIP340PubKey
	// TODO we should have our own representation of PoP
	pop             *btcstakingtypes.ProofOfPossessionBTC
	description     *stakingtypes.Description
	commission      *sdkmath.LegacyDec
	errResponse     chan error
	successResponse chan *RegisterFinalityProviderResponse
}

type finalityProviderRegisteredEvent struct {
	bbnAddress      sdk.AccAddress
	btcPubKey       *bbntypes.BIP340PubKey
	txHash          string
	successResponse chan *RegisterFinalityProviderResponse
}

type RegisterFinalityProviderResponse struct {
	bbnAddress sdk.AccAddress
	btcPubKey  *bbntypes.BIP340PubKey
	TxHash     string
}

type CreateFinalityProviderResult struct {
	FpInfo *proto.FinalityProviderInfo
}

type fpState struct {
	mu  sync.Mutex
	sfp *store.StoredFinalityProvider
	s   *store.FinalityProviderStore
}

func NewFpState(
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
	fps.withLock(func() {
		fps.sfp.Status = s
	})
	return fps.s.SetFpStatus(fps.sfp.BtcPk, s)
}

func (fps *fpState) setLastProcessedHeight(height uint64) error {
	fps.withLock(func() {
		fps.sfp.LastProcessedHeight = height
	})
	return fps.s.SetFpLastProcessedHeight(fps.sfp.BtcPk, height)
}

func (fps *fpState) setLastProcessedAndVotedHeight(height uint64) error {
	fps.withLock(func() {
		fps.sfp.LastVotedHeight = height
		fps.sfp.LastProcessedHeight = height
	})
	return fps.s.SetFpLastVotedHeight(fps.sfp.BtcPk, height)
}

func (fp *FinalityProviderInstance) GetStoreFinalityProvider() *store.StoredFinalityProvider {
	return fp.fpState.sfp
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

func (fp *FinalityProviderInstance) GetLastProcessedHeight() uint64 {
	var lastProcessedHeight uint64
	fp.fpState.withLock(func() {
		lastProcessedHeight = fp.fpState.sfp.LastProcessedHeight
	})
	return lastProcessedHeight
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

func (fp *FinalityProviderInstance) SetLastProcessedHeight(height uint64) error {
	return fp.fpState.setLastProcessedHeight(height)
}

func (fp *FinalityProviderInstance) MustSetLastProcessedHeight(height uint64) {
	if err := fp.SetLastProcessedHeight(height); err != nil {
		fp.logger.Fatal("failed to set last processed height",
			zap.String("pk", fp.GetBtcPkHex()), zap.Uint64("last_processed_height", height))
	}
	fp.metrics.RecordFpLastProcessedHeight(fp.GetBtcPkHex(), height)
}

func (fp *FinalityProviderInstance) updateStateAfterFinalitySigSubmission(height uint64) error {
	return fp.fpState.setLastProcessedAndVotedHeight(height)
}

func (fp *FinalityProviderInstance) MustUpdateStateAfterFinalitySigSubmission(height uint64) {
	if err := fp.updateStateAfterFinalitySigSubmission(height); err != nil {
		fp.logger.Fatal("failed to update state after finality signature submitted",
			zap.String("pk", fp.GetBtcPkHex()), zap.Uint64("height", height))
	}
	fp.metrics.RecordFpLastVotedHeight(fp.GetBtcPkHex(), height)
	fp.metrics.RecordFpLastProcessedHeight(fp.GetBtcPkHex(), height)
}

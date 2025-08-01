package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"

	"github.com/avast/retry-go/v4"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	ccapi "github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
)

type FinalityProviderInstance struct {
	btcPk *bbntypes.BIP340PubKey

	fpState      *FpState
	pubRandState *PubRandState
	cfg          *fpcfg.Config

	logger            *zap.Logger
	em                eotsmanager.EOTSManager
	cc                ccapi.BabylonController
	consumerCon       ccapi.ConsumerController
	poller            types.BlockPoller[types.BlockDescription]
	rndCommitter      types.RandomnessCommitter
	heightDeterminer  types.HeightDeterminer
	finalitySubmitter types.FinalitySignatureSubmitter
	metrics           *metrics.FpMetrics

	criticalErrChan chan<- *CriticalError

	isStarted *atomic.Bool
	wg        sync.WaitGroup
	quit      chan struct{}
}

// NewFinalityProviderInstance returns a FinalityProviderInstance instance with the given Babylon public key
// the finality-provider should be registered before
func NewFinalityProviderInstance(
	fpPk *bbntypes.BIP340PubKey,
	cfg *fpcfg.Config,
	s *store.FinalityProviderStore,
	prStore *store.PubRandProofStore,
	cc ccapi.BabylonController,
	consumerCon ccapi.ConsumerController,
	em eotsmanager.EOTSManager,
	poller types.BlockPoller[types.BlockDescription],
	rndCommitter types.RandomnessCommitter,
	heightDeterminer types.HeightDeterminer,
	finalitySubmitter types.FinalitySignatureSubmitter,
	metrics *metrics.FpMetrics,
	errChan chan<- *CriticalError,
	logger *zap.Logger,
) (*FinalityProviderInstance, error) {
	sfp, err := s.GetFinalityProvider(fpPk.MustToBTCPK())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve the finality provider %s from DB: %w", fpPk.MarshalHex(), err)
	}

	if sfp.Status == proto.FinalityProviderStatus_SLASHED {
		return nil, fmt.Errorf("the finality provider instance is already slashed")
	}

	return newFinalityProviderInstanceFromStore(
		sfp,
		cfg,
		s,
		prStore,
		cc,
		consumerCon,
		em,
		poller,
		rndCommitter,
		heightDeterminer,
		finalitySubmitter,
		metrics,
		errChan,
		logger,
	)
}

// Helper function to create FinalityProviderInstance from store data
func newFinalityProviderInstanceFromStore(
	sfp *store.StoredFinalityProvider,
	cfg *fpcfg.Config,
	s *store.FinalityProviderStore,
	prStore *store.PubRandProofStore,
	cc ccapi.BabylonController,
	consumerCon ccapi.ConsumerController,
	em eotsmanager.EOTSManager,
	poller types.BlockPoller[types.BlockDescription],
	rndCommitter types.RandomnessCommitter,
	heightDeterminer types.HeightDeterminer,
	finalitySubmitter types.FinalitySignatureSubmitter,
	metrics *metrics.FpMetrics,
	errChan chan<- *CriticalError,
	logger *zap.Logger,
) (*FinalityProviderInstance, error) {
	btcPk := bbntypes.NewBIP340PubKeyFromBTCPK(sfp.BtcPk)
	fpState := NewFpState(sfp, s, logger, metrics)

	if err := rndCommitter.Init(btcPk, []byte(sfp.ChainID)); err != nil {
		return nil, fmt.Errorf("failed to initialize randomness committer: %w", err)
	}
	if err := finalitySubmitter.InitState(fpState); err != nil {
		return nil, fmt.Errorf("failed to initialize finality submitter state: %w", err)
	}

	return &FinalityProviderInstance{
		btcPk:             bbntypes.NewBIP340PubKeyFromBTCPK(sfp.BtcPk),
		fpState:           fpState,
		pubRandState:      NewPubRandState(prStore),
		cfg:               cfg,
		logger:            logger,
		isStarted:         atomic.NewBool(false),
		criticalErrChan:   errChan,
		em:                em,
		poller:            poller,
		rndCommitter:      rndCommitter,
		heightDeterminer:  heightDeterminer,
		finalitySubmitter: finalitySubmitter,
		cc:                cc,
		consumerCon:       consumerCon,
		metrics:           metrics,
	}, nil
}

func (fp *FinalityProviderInstance) Start(ctx context.Context) error {
	if fp.isStarted.Swap(true) {
		return fmt.Errorf("the finality-provider instance %s is already started", fp.GetBtcPkHex())
	}

	if fp.IsJailed() {
		fp.logger.Warn("the finality provider is jailed",
			zap.String("pk", fp.GetBtcPkHex()))
	}

	startHeight, err := fp.DetermineStartHeight(ctx)
	if err != nil {
		return fmt.Errorf("failed to get the start height: %w", err)
	}

	fp.logger.Info("starting the finality provider instance",
		zap.String("pk", fp.GetBtcPkHex()), zap.Uint64("height", startHeight))

	if err := fp.poller.SetStartHeight(ctx, startHeight); err != nil {
		return fmt.Errorf("failed to start the poller with start height %d: %w", startHeight, err)
	}

	fp.quit = make(chan struct{})

	fp.wg.Add(2)
	go fp.finalitySigSubmissionLoop(ctx)
	go fp.randomnessCommitmentLoop(ctx)

	return nil
}

func (fp *FinalityProviderInstance) Stop() error {
	if !fp.isStarted.Swap(false) {
		return fmt.Errorf("the finality-provider %s has already stopped", fp.GetBtcPkHex())
	}

	if err := fp.poller.Stop(); err != nil {
		return fmt.Errorf("failed to stop the poller: %w", err)
	}

	fp.logger.Info("stopping finality-provider instance", zap.String("pk", fp.GetBtcPkHex()))

	close(fp.quit)
	fp.wg.Wait()

	fp.logger.Info("the finality-provider instance is successfully stopped", zap.String("pk", fp.GetBtcPkHex()))

	return nil
}

func (fp *FinalityProviderInstance) Shutdown() error {
	if err := fp.Stop(); err != nil {
		return fmt.Errorf("failed to stop finality provider instance: %w", err)
	}

	if err := fp.pubRandState.close(); err != nil {
		return fmt.Errorf("failed to close the pub rand state: %w", err)
	}

	return nil
}

func (fp *FinalityProviderInstance) GetConfig() *fpcfg.Config {
	return fp.cfg
}

func (fp *FinalityProviderInstance) IsRunning() bool {
	return fp.isStarted.Load()
}

// IsJailed returns true if fp is JAILED
// NOTE: it retrieves the the status from the db to
// ensure status is up-to-date
func (fp *FinalityProviderInstance) IsJailed() bool {
	storedFp, err := fp.fpState.s.GetFinalityProvider(fp.GetBtcPk())
	if err != nil {
		panic(fmt.Errorf("failed to retrieve the finality provider %s from db: %w", fp.GetBtcPkHex(), err))
	}

	if storedFp.Status != fp.GetStatus() {
		fp.mustSetStatus(storedFp.Status)
	}

	return fp.GetStatus() == proto.FinalityProviderStatus_JAILED
}

func (fp *FinalityProviderInstance) finalitySigSubmissionLoop(ctx context.Context) {
	defer fp.wg.Done()

	// Process immediately for the first iteration without waiting
	fp.processAndSubmitSignatures(ctx)

	ticker := time.NewTicker(fp.cfg.SignatureSubmissionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fp.processAndSubmitSignatures(ctx)
		case <-fp.quit:
			fp.logger.Info("the finality signature submission loop is closing")

			return
		}
	}
}

// processAndSubmitSignatures handles the logic of fetching blocks, checking jail status,
// processing them, and submitting signatures
func (fp *FinalityProviderInstance) processAndSubmitSignatures(ctx context.Context) {
	pollerBlocks := fp.getBatchBlocksFromPoller()
	if len(pollerBlocks) == 0 {
		return
	}

	if fp.IsJailed() {
		fp.logger.Warn("the finality-provider is jailed",
			zap.String("pk", fp.GetBtcPkHex()),
		)

		return
	}

	targetHeight := pollerBlocks[len(pollerBlocks)-1].GetHeight()
	fp.logger.Debug("the finality-provider received new block(s), start processing",
		zap.String("pk", fp.GetBtcPkHex()),
		zap.Uint64("start_height", pollerBlocks[0].GetHeight()),
		zap.Uint64("end_height", targetHeight),
	)

	res, err := fp.finalitySubmitter.SubmitBatchFinalitySignatures(ctx, pollerBlocks)
	if err != nil {
		fp.metrics.IncrementFpTotalFailedVotes(fp.GetBtcPkHex())

		if errors.Is(err, ErrFinalityProviderJailed) {
			fp.mustSetStatus(proto.FinalityProviderStatus_JAILED)
			fp.logger.Debug("the finality-provider has been jailed",
				zap.String("pk", fp.GetBtcPkHex()))

			return
		}

		if !errors.Is(err, ErrFinalityProviderShutDown) {
			fp.reportCriticalErr(err)
		}

		return
	}

	if res == nil {
		// this can happen when a finality signature is not needed
		// either if the block is already submitted or the signature
		// is already submitted
		return
	}

	fp.logger.Info(
		"successfully submitted the finality signature to the consumer chain",
		zap.String("consumer_id", string(fp.GetChainID())),
		zap.String("pk", fp.GetBtcPkHex()),
		zap.Uint64("start_height", pollerBlocks[0].GetHeight()),
		zap.Uint64("end_height", targetHeight),
		zap.String("tx_hash", res.TxHash),
	)
}

// getBatchBlocksFromPoller retrieves a batch of blocks from the poller, limited by the configured batch size.
func (fp *FinalityProviderInstance) getBatchBlocksFromPoller() []types.BlockDescription {
	var pollerBlocks []types.BlockDescription

	for {
		block, hasBlock := fp.poller.TryNextBlock()
		if !hasBlock {
			// No more blocks immediately available, return what we have
			return pollerBlocks
		}

		pollerBlocks = append(pollerBlocks, block)
		if len(pollerBlocks) == int(fp.cfg.BatchSubmissionSize) {
			return pollerBlocks
		}
	}
}

func (fp *FinalityProviderInstance) randomnessCommitmentLoop(ctx context.Context) {
	defer fp.wg.Done()

	// Process immediately for the first iteration without waiting
	fp.processRandomnessCommitment(ctx)

	ticker := time.NewTicker(fp.cfg.RandomnessCommitInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fp.processRandomnessCommitment(ctx)
		case <-fp.quit:
			fp.logger.Info("the randomness commitment loop is closing")

			return
		}
	}
}

// processRandomnessCommitment handles the logic of checking if randomness should be committed
// and submitting the commitment if needed
func (fp *FinalityProviderInstance) processRandomnessCommitment(ctx context.Context) {
	should, startHeight, err := fp.rndCommitter.ShouldCommit(ctx)
	if err != nil {
		fp.reportCriticalErr(err)

		return
	}

	if !should {
		return
	}

	txRes, err := fp.rndCommitter.Commit(ctx, startHeight)
	if err != nil {
		fp.metrics.IncrementFpTotalFailedRandomness(fp.GetBtcPkHex())
		fp.reportCriticalErr(err)

		return
	}

	// txRes could be nil if no need to commit more randomness
	if txRes != nil {
		fp.logger.Info(
			"successfully committed public randomness to the consumer chain",
			zap.String("consumer_id", string(fp.GetChainID())),
			zap.String("pk", fp.GetBtcPkHex()),
			zap.String("tx_hash", txRes.TxHash),
		)
	}
}

// reportCriticalErr reports a critical error by sending it to the criticalErrChan for further handling.
func (fp *FinalityProviderInstance) reportCriticalErr(err error) {
	select {
	case fp.criticalErrChan <- &CriticalError{
		err:     err,
		fpBtcPk: fp.GetBtcPkBIP340(),
	}:
	case <-fp.quit:
		fp.logger.Debug("skipping error report due to context cancellation", zap.Error(err))
	default:
		fp.logger.Error("failed to report critical error (channel full)", zap.Error(err))
	}
}

// CommitPubRand commits a list of randomness from given start height
func (fp *FinalityProviderInstance) CommitPubRand(ctx context.Context, startHeight uint64) (*types.TxResponse, error) {
	txRes, err := fp.rndCommitter.Commit(ctx, startHeight)
	if err != nil {
		return nil, fmt.Errorf("failed to commit public randomness: %w", err)
	}

	return txRes, nil
}

func (fp *FinalityProviderInstance) NewTestHelper() *FinalityProviderTestHelper {
	return NewFinalityProviderTestHelper(fp)
}

// DetermineStartHeight determines start height for block processing
func (fp *FinalityProviderInstance) DetermineStartHeight(ctx context.Context) (uint64, error) {
	startHeight, err := fp.heightDeterminer.DetermineStartHeight(ctx, fp.btcPk, func() (uint64, error) {
		return fp.GetLastVotedHeight(), nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to determine start height: %w", err)
	}

	return startHeight, nil
}

func (fp *FinalityProviderInstance) GetLastCommittedHeight(ctx context.Context) (uint64, error) {
	height, err := fp.rndCommitter.GetLastCommittedHeight(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get last committed height: %w", err)
	}

	return height, nil
}

// nolint:unused
func (fp *FinalityProviderInstance) getLatestBlockHeightWithRetry() (uint64, error) {
	var (
		latestBlock types.BlockDescription
		err         error
	)

	if err := retry.Do(func() error {
		latestBlock, err = fp.consumerCon.QueryLatestBlock(context.Background())
		if latestBlock == nil || err != nil {
			return fmt.Errorf("failed to query latest block height: %w", err)
		}

		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		fp.logger.Debug(
			"failed to query the consumer chain for the latest block",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return 0, fmt.Errorf("failed to get latest block height after retries: %w", err)
	}
	fp.metrics.RecordBabylonTipHeight(latestBlock.GetHeight())

	return latestBlock.GetHeight(), nil
}

func (fp *FinalityProviderInstance) GetVotingPowerWithRetry(height uint64) (bool, error) {
	var (
		hasPower bool
		err      error
	)

	if err := retry.Do(func() error {
		hasPower, err = fp.consumerCon.QueryFinalityProviderHasPower(context.Background(), ccapi.NewQueryFinalityProviderHasPowerRequest(
			fp.GetBtcPk(),
			height,
		))
		if err != nil {
			return fmt.Errorf("failed to query finality provider voting power: %w", err)
		}

		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		fp.logger.Debug(
			"failed to query the voting power",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return false, fmt.Errorf("failed to get voting power after retries: %w", err)
	}

	return hasPower, nil
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
	return fp.fpState.GetBtcPkBIP340()
}

func (fp *FinalityProviderInstance) GetBtcPk() *btcec.PublicKey {
	return fp.fpState.GetBtcPk()
}

func (fp *FinalityProviderInstance) GetBtcPkHex() string {
	return fp.GetBtcPkBIP340().MarshalHex()
}

func (fp *FinalityProviderInstance) GetStatus() proto.FinalityProviderStatus {
	return fp.fpState.GetStatus()
}

func (fp *FinalityProviderInstance) GetLastVotedHeight() uint64 {
	return fp.fpState.GetLastVotedHeight()
}

func (fp *FinalityProviderInstance) GetChainID() []byte {
	return fp.fpState.GetChainID()
}

func (fp *FinalityProviderInstance) mustSetStatus(s proto.FinalityProviderStatus) {
	if err := fp.fpState.SetStatus(s); err != nil {
		fp.logger.Fatal("failed to set finality-provider status",
			zap.String("pk", fp.GetBtcPkHex()), zap.String("status", s.String()))
	}
}

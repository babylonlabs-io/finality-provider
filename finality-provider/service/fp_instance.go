package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
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

	logger           *zap.Logger
	em               eotsmanager.EOTSManager
	cc               ccapi.ClientController
	consumerCon      ccapi.ConsumerController
	poller           types.BlockPoller[types.BlockDescription]
	rndCommitter     types.RandomnessCommitter
	heightDeterminer types.HeightDeterminer
	metrics          *metrics.FpMetrics

	criticalErrChan chan<- *CriticalError

	isStarted *atomic.Bool

	wg   sync.WaitGroup
	quit chan struct{}
}

// NewFinalityProviderInstance returns a FinalityProviderInstance instance with the given Babylon public key
// the finality-provider should be registered before
func NewFinalityProviderInstance(
	fpPk *bbntypes.BIP340PubKey,
	cfg *fpcfg.Config,
	s *store.FinalityProviderStore,
	prStore *store.PubRandProofStore,
	cc ccapi.ClientController,
	consumerCon ccapi.ConsumerController,
	em eotsmanager.EOTSManager,
	poller types.BlockPoller[types.BlockDescription],
	rndCommitter types.RandomnessCommitter,
	heightDeterminer types.HeightDeterminer,
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

	return newFinalityProviderInstanceFromStore(sfp, cfg, s, prStore, cc, consumerCon, em, poller, rndCommitter, heightDeterminer, metrics, errChan, logger)
}

// Helper function to create FinalityProviderInstance from store data
func newFinalityProviderInstanceFromStore(
	sfp *store.StoredFinalityProvider,
	cfg *fpcfg.Config,
	s *store.FinalityProviderStore,
	prStore *store.PubRandProofStore,
	cc ccapi.ClientController,
	consumerCon ccapi.ConsumerController,
	em eotsmanager.EOTSManager,
	poller types.BlockPoller[types.BlockDescription],
	rndCommitter types.RandomnessCommitter,
	heightDeterminer types.HeightDeterminer,
	metrics *metrics.FpMetrics,
	errChan chan<- *CriticalError,
	logger *zap.Logger,
) (*FinalityProviderInstance, error) {
	btcPk := bbntypes.NewBIP340PubKeyFromBTCPK(sfp.BtcPk)

	rndCommitter.SetBtcPk(btcPk)
	rndCommitter.SetChainID([]byte(sfp.ChainID))

	return &FinalityProviderInstance{
		btcPk:            bbntypes.NewBIP340PubKeyFromBTCPK(sfp.BtcPk),
		fpState:          NewFpState(sfp, s),
		pubRandState:     NewPubRandState(prStore),
		cfg:              cfg,
		logger:           logger,
		isStarted:        atomic.NewBool(false),
		criticalErrChan:  errChan,
		em:               em,
		poller:           poller,
		rndCommitter:     rndCommitter,
		heightDeterminer: heightDeterminer,
		cc:               cc,
		consumerCon:      consumerCon,
		metrics:          metrics,
	}, nil
}

func (fp *FinalityProviderInstance) Start() error {
	if fp.isStarted.Swap(true) {
		return fmt.Errorf("the finality-provider instance %s is already started", fp.GetBtcPkHex())
	}

	if fp.IsJailed() {
		fp.logger.Warn("the finality provider is jailed",
			zap.String("pk", fp.GetBtcPkHex()))
	}

	// todo(lazar): will fix ctx in next PRs
	startHeight, err := fp.DetermineStartHeight(context.Background())

	if err != nil {
		return fmt.Errorf("failed to get the start height: %w", err)
	}

	fp.logger.Info("starting the finality provider instance",
		zap.String("pk", fp.GetBtcPkHex()), zap.Uint64("height", startHeight))

	// todo(lazar): will fix this in next PRs
	if err := fp.poller.SetStartHeight(context.Background(), startHeight); err != nil {
		return fmt.Errorf("failed to start the poller with start height %d: %w", startHeight, err)
	}

	fp.quit = make(chan struct{})

	fp.wg.Add(2)
	// todo(lazar): should have ctx passed in
	go fp.finalitySigSubmissionLoop()
	// todo(lazar): will fix ctx in next PRs
	go fp.randomnessCommitmentLoop(context.Background())

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
		return err
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
		fp.MustSetStatus(storedFp.Status)
	}

	return fp.GetStatus() == proto.FinalityProviderStatus_JAILED
}

func (fp *FinalityProviderInstance) finalitySigSubmissionLoop() {
	defer fp.wg.Done()

	// Process immediately for the first iteration without waiting
	fp.processAndSubmitSignatures()

	ticker := time.NewTicker(fp.cfg.SignatureSubmissionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fp.processAndSubmitSignatures()
		case <-fp.quit:
			fp.logger.Info("the finality signature submission loop is closing")

			return
		}
	}
}

// processAndSubmitSignatures handles the logic of fetching blocks, checking jail status,
// processing them, and submitting signatures
func (fp *FinalityProviderInstance) processAndSubmitSignatures() {
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

	processedBlocks, err := fp.processBlocksToVote(pollerBlocks)
	if err != nil {
		fp.reportCriticalErr(err)

		return
	}

	if len(processedBlocks) == 0 {
		return
	}

	res, err := fp.retrySubmitSigsUntilFinalized(processedBlocks)
	if err != nil {
		fp.metrics.IncrementFpTotalFailedVotes(fp.GetBtcPkHex())

		if errors.Is(err, ErrFinalityProviderJailed) {
			fp.MustSetStatus(proto.FinalityProviderStatus_JAILED)
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

// processBlocksToVote processes a batch a blocks and picks ones that need to vote
// it also updates the fp instance status according to the block's voting power
func (fp *FinalityProviderInstance) processBlocksToVote(blocks []types.BlockDescription) ([]types.BlockDescription, error) {
	processedBlocks := make([]types.BlockDescription, 0, len(blocks))

	var hasPower bool
	var err error
	for _, b := range blocks {
		blk := b
		if blk.GetHeight() <= fp.GetLastVotedHeight() {
			fp.logger.Debug(
				"the block height is lower than last processed height",
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint64("block_height", blk.GetHeight()),
				zap.Uint64("last_voted_height", fp.GetLastVotedHeight()),
			)

			continue
		}

		// check whether the finality provider has voting power
		hasPower, err = fp.GetVotingPowerWithRetry(blk.GetHeight())
		if err != nil {
			return nil, fmt.Errorf("failed to get voting power for height %d: %w", blk.GetHeight(), err)
		}
		if !hasPower {
			fp.logger.Debug(
				"the finality-provider does not have voting power",
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint64("block_height", blk.GetHeight()),
			)

			// the finality provider does not have voting power
			// and it will never will at this block, so continue
			fp.metrics.IncrementFpTotalBlocksWithoutVotingPower(fp.GetBtcPkHex())

			continue
		}

		processedBlocks = append(processedBlocks, blk)
	}

	// update fp status according to the power for the last block
	if hasPower && fp.GetStatus() != proto.FinalityProviderStatus_ACTIVE {
		fp.MustSetStatus(proto.FinalityProviderStatus_ACTIVE)
	}

	if !hasPower && fp.GetStatus() == proto.FinalityProviderStatus_ACTIVE {
		fp.MustSetStatus(proto.FinalityProviderStatus_INACTIVE)
	}

	return processedBlocks, nil
}

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

func (fp *FinalityProviderInstance) reportCriticalErr(err error) {
	fp.criticalErrChan <- &CriticalError{
		err:     err,
		fpBtcPk: fp.GetBtcPkBIP340(),
	}
}

// retrySubmitSigsUntilFinalized periodically tries to submit finality signature until success or the block is finalized
// error will be returned if maximum retries have been reached or the query to the consumer chain fails
func (fp *FinalityProviderInstance) retrySubmitSigsUntilFinalized(targetBlocks []types.BlockDescription) (*types.TxResponse, error) {
	if len(targetBlocks) == 0 {
		return nil, fmt.Errorf("cannot send signatures for empty blocks")
	}

	var failedCycles uint32
	targetHeight := targetBlocks[len(targetBlocks)-1].GetHeight()

	// First iteration happens before the loop
	for {
		// Attempt submission immediately
		// error will be returned if max retries have been reached
		var res *types.TxResponse
		var err error
		res, err = fp.SubmitBatchFinalitySignatures(targetBlocks)
		if err != nil {
			fp.logger.Debug(
				"failed to submit finality signature to the consumer chain",
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint32("current_failures", failedCycles),
				zap.Uint64("target_start_height", targetBlocks[0].GetHeight()),
				zap.Uint64("target_end_height", targetHeight),
				zap.Error(err),
			)

			if fpcc.IsUnrecoverable(err) {
				return nil, err
			}

			if fpcc.IsExpected(err) {
				return nil, nil
			}

			failedCycles++
			if failedCycles > fp.cfg.MaxSubmissionRetries {
				return nil, fmt.Errorf("reached max failed cycles with err: %w", err)
			}
		} else {
			// The signature has been successfully submitted
			return res, nil
		}

		// periodically query the index block to be later checked whether it is Finalized
		finalized, err := fp.checkBlockFinalization(context.Background(), targetHeight)
		if err != nil {
			return nil, fmt.Errorf("failed to query block finalization at height %v: %w", targetHeight, err)
		}
		if finalized {
			fp.logger.Debug(
				"the block is already finalized, skip submission",
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint64("target_height", targetHeight),
			)

			fp.metrics.IncrementFpTotalFailedVotes(fp.GetBtcPkHex())

			// TODO: returning nil here is to safely break the loop
			//  the error still exists
			return nil, nil
		}

		// Wait for the retry interval
		select {
		case <-time.After(fp.cfg.SubmissionRetryInterval):
			// Continue to next retry iteration
			continue
		case <-fp.quit:
			fp.logger.Debug("the finality-provider instance is closing", zap.String("pk", fp.GetBtcPkHex()))

			return nil, ErrFinalityProviderShutDown
		}
	}
}

func (fp *FinalityProviderInstance) checkBlockFinalization(ctx context.Context, height uint64) (bool, error) {
	b, err := fp.consumerCon.QueryBlock(ctx, height)
	if err != nil {
		return false, err
	}

	return b.IsFinalized(), nil
}

// CommitPubRand commits a list of randomness from given start height
func (fp *FinalityProviderInstance) CommitPubRand(ctx context.Context, startHeight uint64) (*types.TxResponse, error) {
	return fp.rndCommitter.Commit(ctx, startHeight)
}

// SubmitBatchFinalitySignatures builds and sends a finality signature over the given block to the consumer chain
// Contract:
//  1. the input blocks should be in the ascending order of height
//  2. the returned response could be nil due to no transactions might be made in the end
func (fp *FinalityProviderInstance) SubmitBatchFinalitySignatures(blocks []types.BlockDescription) (*types.TxResponse, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("should not submit batch finality signature with zero block")
	}

	if len(blocks) > math.MaxUint32 {
		return nil, fmt.Errorf("should not submit batch finality signature with too many blocks")
	}

	// get public randomness list
	numPubRand := len(blocks)
	// #nosec G115 -- performed the conversion check above
	prList, err := fp.GetPubRandList(blocks[0].GetHeight(), uint32(numPubRand))
	if err != nil {
		return nil, fmt.Errorf("failed to get public randomness list: %w", err)
	}
	// get proof list
	// TODO: how to recover upon having an error in getPubRandProofList?
	proofBytesList, err := fp.pubRandState.getPubRandProofList(
		fp.btcPk.MustMarshal(),
		fp.GetChainID(),
		blocks[0].GetHeight(),
		uint64(numPubRand),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get public randomness inclusion proof list: %w\nplease recover the randomness proof from db", err)
	}

	// Create slices to store only the valid items
	validBlocks := make([]*types.BlockInfo, 0, len(blocks))
	validPrList := make([]*btcec.FieldVal, 0, len(blocks))
	validProofList := make([][]byte, 0, len(blocks))
	validSigList := make([]*btcec.ModNScalar, 0, len(blocks))

	// Process each block and collect only valid items
	// (skip ones encountering double sign error)
	for i, b := range blocks {
		eotsSig, err := fp.SignFinalitySig(b)
		if err != nil {
			if !errors.Is(err, ErrFailedPrecondition) {
				return nil, err
			}
			// Skip this block and its corresponding items if we encounter FailedPrecondition
			fp.logger.Warn("encountered FailedPrecondition error, skipping block",
				zap.Uint64("height", b.GetHeight()),
				zap.String("hash", hex.EncodeToString(b.GetHash())),
				zap.Error(err))

			continue
		}
		// If signature is valid, append all corresponding items
		// TODO(Lazar955): will change this type to interface BlockDescription but for now if we do it we need to
		// change signature of SubmitBatchFinalitySigs and all implementing methods
		validBlocks = append(validBlocks, types.NewBlockInfo(b.GetHeight(), b.GetHash(), b.IsFinalized()))
		validPrList = append(validPrList, prList[i])
		validProofList = append(validProofList, proofBytesList[i])
		validSigList = append(validSigList, eotsSig.ToModNScalar())
	}

	// If all blocks were skipped, return early
	if len(validBlocks) == 0 {
		fp.logger.Info("all blocks were skipped due to double sign errors")

		return nil, nil
	}

	// send finality signature to the consumer chain with only valid items
	res, err := fp.consumerCon.SubmitBatchFinalitySigs(context.Background(), ccapi.NewSubmitBatchFinalitySigsRequest(
		fp.GetBtcPk(),
		validBlocks,
		validPrList,
		validProofList,
		validSigList,
	))

	if err != nil {
		if strings.Contains(err.Error(), "jailed") {
			return nil, ErrFinalityProviderJailed
		}
		if strings.Contains(err.Error(), "slashed") {
			return nil, ErrFinalityProviderSlashed
		}

		return nil, err
	}

	// update the metrics with voted blocks
	for _, b := range validBlocks {
		fp.metrics.RecordFpVotedHeight(fp.GetBtcPkHex(), b.GetHeight())
	}

	// update state with the highest height of this batch even though
	// some of the votes are skipped due to double sign error
	highBlock := blocks[len(blocks)-1]
	fp.MustUpdateStateAfterFinalitySigSubmission(highBlock.GetHeight())

	return res, nil
}

func (fp *FinalityProviderInstance) NewTestHelper() *FinalityProviderTestHelper {
	return NewFinalityProviderTestHelper(fp)
}

// DetermineStartHeight determines start height for block processing
func (fp *FinalityProviderInstance) DetermineStartHeight(ctx context.Context) (uint64, error) {
	return fp.heightDeterminer.DetermineStartHeight(ctx, fp.btcPk, func() (uint64, error) {
		return fp.GetLastVotedHeight(), nil
	})
}

func (fp *FinalityProviderInstance) GetLastCommittedHeight(ctx context.Context) (uint64, error) {
	return fp.rndCommitter.GetLastCommittedHeight(ctx)
}

// nolint:unused
func (fp *FinalityProviderInstance) getLatestBlockHeightWithRetry() (uint64, error) {
	var (
		latestBlockHeight uint64
		err               error
	)

	if err := retry.Do(func() error {
		latestBlockHeight, err = fp.consumerCon.QueryLatestBlockHeight(context.Background())
		if err != nil {
			return err
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
		return 0, err
	}
	fp.metrics.RecordBabylonTipHeight(latestBlockHeight)

	return latestBlockHeight, nil
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
			return err
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
		return false, err
	}

	return hasPower, nil
}

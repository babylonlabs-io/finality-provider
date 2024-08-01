package service

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	bbntypes "github.com/babylonchain/babylon/types"
	bstypes "github.com/babylonchain/babylon/x/btcstaking/types"
	ftypes "github.com/babylonchain/babylon/x/finality/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/cosmos/relayer/v2/relayer/provider"
	"github.com/gogo/protobuf/jsonpb"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	fpcc "github.com/babylonchain/finality-provider/clientcontroller"
	ccapi "github.com/babylonchain/finality-provider/clientcontroller/api"
	"github.com/babylonchain/finality-provider/eotsmanager"
	fpcfg "github.com/babylonchain/finality-provider/finality-provider/config"
	"github.com/babylonchain/finality-provider/finality-provider/proto"
	"github.com/babylonchain/finality-provider/finality-provider/store"
	"github.com/babylonchain/finality-provider/metrics"
	"github.com/babylonchain/finality-provider/types"
)

type FinalityProviderInstance struct {
	btcPk *bbntypes.BIP340PubKey

	fpState      *fpState
	pubRandState *pubRandState
	cfg          *fpcfg.Config

	logger      *zap.Logger
	em          eotsmanager.EOTSManager
	cc          ccapi.ClientController
	consumerCon ccapi.ConsumerController
	poller      *ChainPoller
	metrics     *metrics.FpMetrics

	// passphrase is used to unlock private keys
	passphrase string

	laggingTargetChan chan uint64
	criticalErrChan   chan<- *CriticalError

	isStarted *atomic.Bool
	inSync    *atomic.Bool
	isLagging *atomic.Bool

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
	metrics *metrics.FpMetrics,
	passphrase string,
	errChan chan<- *CriticalError,
	logger *zap.Logger,
) (*FinalityProviderInstance, error) {
	sfp, err := s.GetFinalityProvider(fpPk.MustToBTCPK())
	if err != nil {
		return nil, fmt.Errorf("failed to retrive the finality-provider %s from DB: %w", fpPk.MarshalHex(), err)
	}

	// ensure the finality-provider has been registered
	if sfp.Status < proto.FinalityProviderStatus_REGISTERED {
		return nil, fmt.Errorf("the finality-provider %s has not been registered", sfp.KeyName)
	}

	return &FinalityProviderInstance{
		btcPk:           bbntypes.NewBIP340PubKeyFromBTCPK(sfp.BtcPk),
		fpState:         NewFpState(sfp, s),
		pubRandState:    NewPubRandState(prStore),
		cfg:             cfg,
		logger:          logger,
		isStarted:       atomic.NewBool(false),
		inSync:          atomic.NewBool(false),
		isLagging:       atomic.NewBool(false),
		criticalErrChan: errChan,
		passphrase:      passphrase,
		em:              em,
		cc:              cc,
		consumerCon:     consumerCon,
		metrics:         metrics,
	}, nil
}

func (fp *FinalityProviderInstance) Start() error {
	if fp.isStarted.Swap(true) {
		return fmt.Errorf("the finality-provider instance %s is already started", fp.GetBtcPkHex())
	}

	fp.logger.Info("Starting finality-provider instance", zap.String("pk", fp.GetBtcPkHex()))

	startHeight, err := fp.bootstrap()
	if err != nil {
		return fmt.Errorf("failed to bootstrap the finality-provider %s: %w", fp.GetBtcPkHex(), err)
	}

	fp.logger.Info("the finality-provider has been bootstrapped",
		zap.String("pk", fp.GetBtcPkHex()), zap.Uint64("height", startHeight))

	poller := NewChainPoller(fp.logger, fp.cfg.PollerConfig, fp.cc, fp.consumerCon, fp.metrics)

	if err := poller.Start(startHeight); err != nil {
		return fmt.Errorf("failed to start the poller: %w", err)
	}

	fp.poller = poller

	fp.laggingTargetChan = make(chan uint64, 1)

	fp.quit = make(chan struct{})

	fp.wg.Add(1)
	go fp.finalitySigSubmissionLoop()
	fp.wg.Add(1)
	go fp.randomnessCommitmentLoop(startHeight)
	fp.wg.Add(1)
	go fp.checkLaggingLoop()

	return nil
}

func (fp *FinalityProviderInstance) bootstrap() (uint64, error) {
	latestBlockHeight, err := fp.getLatestBlockHeightWithRetry()
	if err != nil {
		return 0, err
	}

	if fp.checkLagging(latestBlockHeight) {
		_, err := fp.tryFastSync(latestBlockHeight)
		if err != nil && !fpcc.IsExpected(err) {
			return 0, err
		}
	}

	startHeight, err := fp.getPollerStartingHeight()
	if err != nil {
		return 0, err
	}

	return startHeight, nil
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

func (fp *FinalityProviderInstance) IsRunning() bool {
	return fp.isStarted.Load()
}

func (fp *FinalityProviderInstance) finalitySigSubmissionLoop() {
	defer fp.wg.Done()

	for {
		select {
		case b := <-fp.poller.GetBlockInfoChan():
			fp.logger.Debug(
				"the finality-provider received a new block, start processing",
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint64("height", b.Height),
				zap.String("block_hash", hex.EncodeToString(b.Hash)),
			)

			// check whether the block has been processed before
			if fp.hasProcessed(b.Height) {
				continue
			}
			// check whether the finality provider has voting power
			hasVp, err := fp.hasVotingPower(b.Height)
			if err != nil {
				fp.reportCriticalErr(err)
				continue
			}
			if !hasVp {
				// the finality provider does not have voting power
				// and it will never will at this block
				fp.MustSetLastProcessedHeight(b.Height)
				fp.metrics.IncrementFpTotalBlocksWithoutVotingPower(fp.GetBtcPkHex())
				continue
			}
			// check whether the randomness has been committed
			// the retry will end if max retry times is reached
			// or the target block is finalized
			isFinalized, err := fp.retryCheckRandomnessUntilBlockFinalized(b)
			if err != nil {
				if !errors.Is(err, ErrFinalityProviderShutDown) {
					fp.reportCriticalErr(err)
				}
				break
			}
			// the block is finalized, no need to submit finality signature
			if isFinalized {
				fp.MustSetLastProcessedHeight(b.Height)
				continue
			}

			// use the copy of the block to avoid the impact to other receivers
			nextBlock := *b
			res, err := fp.retrySubmitFinalitySignatureUntilBlockFinalized(&nextBlock)
			if err != nil {
				fp.metrics.IncrementFpTotalFailedVotes(fp.GetBtcPkHex())
				if !errors.Is(err, ErrFinalityProviderShutDown) {
					fp.reportCriticalErr(err)
				}
				continue
			}
			if res == nil {
				// this can happen when a finality signature is not needed
				// either if the block is already submitted or the signature
				// is already submitted
				continue
			}
			fp.logger.Info(
				"successfully submitted a finality signature to the consumer chain",
				zap.String("consumer_id", string(fp.GetChainID())),
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint64("height", b.Height),
				zap.String("tx_hash", res.TxHash),
			)

		case targetBlock := <-fp.laggingTargetChan:
			res, err := fp.tryFastSync(targetBlock)
			fp.isLagging.Store(false)
			if err != nil {
				if errors.Is(err, bstypes.ErrFpAlreadySlashed) {
					fp.reportCriticalErr(err)
					continue
				}
				fp.logger.Debug(
					"failed to sync up, will try again later",
					zap.String("pk", fp.GetBtcPkHex()),
					zap.Error(err),
				)
				continue
			}
			// response might be nil if sync is not needed
			if res != nil {
				fp.logger.Info(
					"fast sync is finished",
					zap.String("pk", fp.GetBtcPkHex()),
					zap.Uint64("synced_height", res.SyncedHeight),
					zap.Uint64("last_processed_height", res.LastProcessedHeight),
				)

				// inform the poller to skip to the next block of the last
				// processed one
				err := fp.poller.SkipToHeight(fp.GetLastProcessedHeight() + 1)
				if err != nil {
					fp.logger.Debug(
						"failed to skip heights from the poller",
						zap.Error(err),
					)
				}
			}
		case <-fp.quit:
			fp.logger.Info("the finality signature submission loop is closing")
			return
		}
	}
}

func (fp *FinalityProviderInstance) randomnessCommitmentLoop(startHeight uint64) {
	defer fp.wg.Done()

	commitRandTicker := time.NewTicker(fp.cfg.RandomnessCommitInterval)
	defer commitRandTicker.Stop()

	lastCommittedHeight, err := fp.GetLastCommittedHeight()
	if err != nil {
		fp.logger.Fatal("Error getting last committed height while starting the randomness commitment loop", zap.Error(err))
		return
	}

	// if there is no committed randomness, we need to commit the first randomness
	if lastCommittedHeight == uint64(0) {
		txRes, err := fp.retryCommitPubRandUntilMaxRetry(startHeight)
		if err != nil {
			fp.metrics.IncrementFpTotalFailedRandomness(fp.GetBtcPkHex())
			fp.reportCriticalErr(err)
			return
		}
		if txRes == nil {
			fp.logger.Fatal(
				"Error submitting the first randomness",
				zap.String("consumer_id", string(fp.GetChainID())),
				zap.String("pk", fp.GetBtcPkHex()),
			)
			return
		}
		fp.logger.Info(
			"successfully committed public randomness to the consumer chain",
			zap.String("consumer_id", string(fp.GetChainID())),
			zap.String("pk", fp.GetBtcPkHex()),
		)
	}

	for {
		select {
		case <-commitRandTicker.C:
			tipBlockHeight, err := fp.getLatestBlockHeightWithRetry()
			if err != nil {
				fp.reportCriticalErr(err)
				continue
			}
			txRes, err := fp.retryCommitPubRandUntilBlockFinalized(tipBlockHeight)
			if err != nil {
				fp.metrics.IncrementFpTotalFailedRandomness(fp.GetBtcPkHex())
				fp.reportCriticalErr(err)
				continue
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

		case <-fp.quit:
			fp.logger.Info("the randomness commitment loop is closing")
			return
		}
	}
}

func (fp *FinalityProviderInstance) checkLaggingLoop() {
	defer fp.wg.Done()

	if fp.cfg.FastSyncInterval == 0 {
		fp.logger.Info("the fast sync is disabled")
		return
	}

	fastSyncTicker := time.NewTicker(fp.cfg.FastSyncInterval)
	defer fastSyncTicker.Stop()

	for {
		select {
		case <-fastSyncTicker.C:
			if fp.isLagging.Load() {
				// we are in fast sync mode, skip do not do checks
				continue
			}

			latestBlockHeight, err := fp.getLatestBlockHeightWithRetry()
			if err != nil {
				fp.logger.Debug(
					"failed to get the latest block of the consumer chain",
					zap.String("pk", fp.GetBtcPkHex()),
					zap.Error(err),
				)
				continue
			}

			if fp.checkLagging(latestBlockHeight) {
				fp.isLagging.Store(true)
				fp.laggingTargetChan <- latestBlockHeight
			}
		case <-fp.quit:
			fp.logger.Debug("the fast sync loop is closing")
			return
		}
	}
}

func (fp *FinalityProviderInstance) tryFastSync(targetBlockHeight uint64) (*FastSyncResult, error) {
	if fp.inSync.Load() {
		return nil, fmt.Errorf("the finality-provider %s is already in sync", fp.GetBtcPkHex())
	}

	// get the last finalized height
	lastFinalizedBlock, err := fp.latestFinalizedBlockWithRetry()
	if err != nil {
		return nil, err
	}
	if lastFinalizedBlock == nil {
		fp.logger.Debug(
			"no finalized blocks yet, no need to catch up",
			zap.String("pk", fp.GetBtcPkHex()),
			zap.Uint64("height", targetBlockHeight),
		)
		return nil, nil
	}

	lastFinalizedHeight := lastFinalizedBlock.Height
	lastProcessedHeight := fp.GetLastProcessedHeight()

	// get the startHeight from the maximum of the lastVotedHeight and
	// the lastFinalizedHeight plus 1
	var startHeight uint64
	if lastFinalizedHeight < lastProcessedHeight {
		startHeight = lastProcessedHeight + 1
	} else {
		startHeight = lastFinalizedHeight + 1
	}

	if startHeight > targetBlockHeight {
		return nil, fmt.Errorf("the start height %v should not be higher than the current block %v", startHeight, targetBlockHeight)
	}

	fp.logger.Debug("the finality-provider is entering fast sync",
		zap.Uint64("start_height", startHeight), zap.Uint64("target_block_height", targetBlockHeight))

	return fp.FastSync(startHeight, targetBlockHeight)
}

func (fp *FinalityProviderInstance) hasProcessed(blockHeight uint64) bool {
	if blockHeight <= fp.GetLastProcessedHeight() {
		fp.logger.Debug(
			"the block has been processed before, skip processing",
			zap.String("pk", fp.GetBtcPkHex()),
			zap.Uint64("block_height", blockHeight),
			zap.Uint64("last_processed_height", fp.GetLastProcessedHeight()),
		)
		return true
	}

	return false
}

// hasVotingPower checks whether the finality provider has voting power for the given block
func (fp *FinalityProviderInstance) hasVotingPower(blockHeight uint64) (bool, error) {
	hasPower, err := fp.GetVotingPowerWithRetry(blockHeight)
	if err != nil {
		return false, err
	}
	if !hasPower {
		fp.logger.Debug(
			"the finality-provider does not have voting power",
			zap.String("pk", fp.GetBtcPkHex()),
			zap.Uint64("block_height", blockHeight),
		)

		return false, nil
	}

	return true, nil
}

func (fp *FinalityProviderInstance) hasRandomness(b *types.BlockInfo) (bool, error) {
	lastCommittedHeight, err := fp.GetLastCommittedHeight()
	if err != nil {
		return false, err
	}
	if b.Height > lastCommittedHeight {
		fp.logger.Debug(
			"the finality provider has not committed public randomness for the height",
			zap.String("pk", fp.GetBtcPkHex()),
			zap.Uint64("block_height", b.Height),
			zap.Uint64("last_committed_height", lastCommittedHeight),
		)
		return false, nil
	}

	return true, nil
}

func (fp *FinalityProviderInstance) reportCriticalErr(err error) {
	fp.criticalErrChan <- &CriticalError{
		err:     err,
		fpBtcPk: fp.GetBtcPkBIP340(),
	}
}

// checkLagging returns true if the lasted voted height is behind by a configured gap
func (fp *FinalityProviderInstance) checkLagging(currentBlockHeight uint64) bool {
	return currentBlockHeight >= fp.GetLastProcessedHeight()+fp.cfg.FastSyncGap
}

// retryQueryingRandomnessUntilBlockFinalized periodically checks whether
// the randomness has been committed to the target block until the block is
// finalized
// error will be returned if maximum retries have been reached or the query to
// the consumer chain fails
func (fp *FinalityProviderInstance) retryCheckRandomnessUntilBlockFinalized(targetBlock *types.BlockInfo) (bool, error) {
	var numRetries uint32

	// we break the for loop if the block is finalized or the randomness is successfully committed
	// error will be returned if maximum retries have been reached or the query to the consumer chain fails
	for {
		fp.logger.Debug(
			"checking randomness",
			zap.String("pk", fp.GetBtcPkHex()),
			zap.Uint64("target_block_height", targetBlock.Height),
		)
		hasRand, err := fp.hasRandomness(targetBlock)
		if err != nil {
			fp.logger.Debug(
				"failed to check last committed randomness",
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint32("current_failures", numRetries),
				zap.Uint64("target_block_height", targetBlock.Height),
				zap.Error(err),
			)

			numRetries += 1
			if numRetries > uint32(fp.cfg.MaxSubmissionRetries) {
				return false, fmt.Errorf("reached max failed cycles with err: %w", err)
			}
		} else if !hasRand {
			fp.logger.Debug(
				"randomness does not exist",
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint32("current_retries", numRetries),
				zap.Uint64("target_block_height", targetBlock.Height),
			)

			numRetries += 1
			if numRetries > uint32(fp.cfg.MaxSubmissionRetries) {
				return false, fmt.Errorf("reached max retries but randomness still not existed")
			}
		} else {
			// the randomness has been successfully committed
			return false, nil
		}
		select {
		case <-time.After(fp.cfg.SubmissionRetryInterval):
			// periodically query the index block to be later checked whether it is Finalized
			finalized, err := fp.consumerCon.QueryIsBlockFinalized(targetBlock.Height)
			if err != nil {
				return false, fmt.Errorf("failed to query block finalization at height %v: %w", targetBlock.Height, err)
			}
			if finalized {
				fp.logger.Debug(
					"the block is already finalized, skip checking randomness",
					zap.String("pk", fp.GetBtcPkHex()),
					zap.Uint64("target_height", targetBlock.Height),
				)
				// TODO: returning nil here is to safely break the loop
				//  the error still exists
				return true, nil
			}

		case <-fp.quit:
			fp.logger.Debug("the finality-provider instance is closing", zap.String("pk", fp.GetBtcPkHex()))
			return false, ErrFinalityProviderShutDown
		}
	}
}

// retrySubmitFinalitySignatureUntilBlockFinalized periodically tries to submit finality signature until success or the block is finalized
// error will be returned if maximum retries have been reached or the query to the consumer chain fails
func (fp *FinalityProviderInstance) retrySubmitFinalitySignatureUntilBlockFinalized(targetBlock *types.BlockInfo) (*types.TxResponse, error) {
	var failedCycles uint32

	// we break the for loop if the block is finalized or the signature is successfully submitted
	// error will be returned if maximum retries have been reached or the query to the consumer chain fails
	for {
		// error will be returned if max retries have been reached
		res, err := fp.SubmitFinalitySignature(targetBlock)
		if err != nil {

			fp.logger.Debug(
				"failed to submit finality signature to the consumer chain",
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint32("current_failures", failedCycles),
				zap.Uint64("target_block_height", targetBlock.Height),
				zap.Error(err),
			)

			if fpcc.IsUnrecoverable(err) {
				return nil, err
			}

			if fpcc.IsExpected(err) {
				return nil, nil
			}

			failedCycles += 1
			if failedCycles > uint32(fp.cfg.MaxSubmissionRetries) {
				return nil, fmt.Errorf("reached max failed cycles with err: %w", err)
			}
		} else {
			// the signature has been successfully submitted
			return res, nil
		}
		select {
		case <-time.After(fp.cfg.SubmissionRetryInterval):
			// periodically query the index block to be later checked whether it is Finalized
			finalized, err := fp.consumerCon.QueryIsBlockFinalized(targetBlock.Height)
			if err != nil {
				return nil, fmt.Errorf("failed to query block finalization at height %v: %w", targetBlock.Height, err)
			}
			if finalized {
				fp.logger.Debug(
					"the block is already finalized, skip submission",
					zap.String("pk", fp.GetBtcPkHex()),
					zap.Uint64("target_height", targetBlock.Height),
				)
				// TODO: returning nil here is to safely break the loop
				//  the error still exists
				return nil, nil
			}

		case <-fp.quit:
			fp.logger.Debug("the finality-provider instance is closing", zap.String("pk", fp.GetBtcPkHex()))
			return nil, ErrFinalityProviderShutDown
		}
	}
}

// retryCommitPubRandUntilBlockFinalized periodically tries to commit public rand until success or the block is finalized
// error will be returned if maximum retries have been reached or the query to the consumer chain fails
func (fp *FinalityProviderInstance) retryCommitPubRandUntilBlockFinalized(targetBlockHeight uint64) (*types.TxResponse, error) {
	var failedCycles uint32

	// we break the for loop if the block is finalized or the public rand is successfully committed
	// error will be returned if maximum retries have been reached or the query to the consumer chain fails
	for {
		// error will be returned if max retries have been reached
		// TODO: CommitPubRand also includes saving all inclusion proofs of public randomness
		// this part should not be retried here. We need to separate the function into
		// 1) determining the starting height to commit, 2) generating pub rand and inclusion
		//  proofs, and 3) committing public randomness.
		// TODO: make 3) a part of `select` statement. The function terminates upon either the block
		// is finalised or the pub rand is committed successfully
		res, err := fp.CommitPubRand(targetBlockHeight)
		if err != nil {
			if fpcc.IsUnrecoverable(err) {
				return nil, err
			}
			fp.logger.Debug(
				"failed to commit public randomness to the consumer chain",
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint32("current_failures", failedCycles),
				zap.Uint64("target_block_height", targetBlockHeight),
				zap.Error(err),
			)

			failedCycles += 1
			if failedCycles > uint32(fp.cfg.MaxSubmissionRetries) {
				return nil, fmt.Errorf("reached max failed cycles with err: %w", err)
			}
		} else {
			// the public randomness has been successfully submitted
			return res, nil
		}
		select {
		case <-time.After(fp.cfg.SubmissionRetryInterval):
			// periodically query the index block to be later checked whether it is Finalized
			finalized, err := fp.consumerCon.QueryIsBlockFinalized(targetBlockHeight)
			if err != nil {
				return nil, fmt.Errorf("failed to query block finalization at height %v: %w", targetBlockHeight, err)
			}
			if finalized {
				fp.logger.Debug(
					"the block is already finalized, skip submission",
					zap.String("pk", fp.GetBtcPkHex()),
					zap.Uint64("target_height", targetBlockHeight),
				)
				// TODO: returning nil here is to safely break the loop
				//  the error still exists
				return nil, nil
			}

		case <-fp.quit:
			fp.logger.Debug("the finality-provider instance is closing", zap.String("pk", fp.GetBtcPkHex()))
			return nil, nil
		}
	}
}

func (fp *FinalityProviderInstance) retryCommitPubRandUntilMaxRetry(targetBlockHeight uint64) (*types.TxResponse, error) {
	var failedCycles uint32

	// we break the for loop if the public rand is successfully committed
	// error will be returned if maximum retries have been reached
	for {
		// error will be returned if max retries have been reached
		// TODO: CommitPubRand also includes saving all inclusion proofs of public randomness
		// this part should not be retried here. We need to separate the function into
		// 1) determining the starting height to commit, 2) generating pub rand and inclusion
		//  proofs, and 3) committing public randomness.
		// TODO: make 3) a part of `select` statement. The function terminates upon either the block
		// is finalised or the pub rand is committed successfully
		res, err := fp.CommitPubRand(targetBlockHeight)
		if err != nil {
			if fpcc.IsUnrecoverable(err) {
				return nil, err
			}
			fp.logger.Debug(
				"failed to commit public randomness to the consumer chain",
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint32("current_failures", failedCycles),
				zap.Uint64("target_block_height", targetBlockHeight),
				zap.Error(err),
			)

			failedCycles += 1
			if failedCycles > uint32(fp.cfg.MaxSubmissionRetries) {
				return nil, fmt.Errorf("reached max failed cycles with err: %w", err)
			}
		} else {
			// the public randomness has been successfully submitted
			return res, nil
		}
		select {
		case <-time.After(fp.cfg.SubmissionRetryInterval):
			continue
		case <-fp.quit:
			fp.logger.Debug("the finality-provider instance is closing", zap.String("pk", fp.GetBtcPkHex()))
			return nil, nil
		}
	}
}

// CommitPubRand generates a list of Schnorr rand pairs,
// commits the public randomness for the managed finality providers,
// and save the randomness pair to DB
func (fp *FinalityProviderInstance) CommitPubRand(targetBlockHeight uint64) (*types.TxResponse, error) {
	lastCommittedHeight, err := fp.GetLastCommittedHeight()
	if err != nil {
		return nil, err
	}

	var startHeight uint64
	if lastCommittedHeight == uint64(0) {
		// the finality-provider has never submitted public rand before
		startHeight = targetBlockHeight
	} else if lastCommittedHeight < fp.cfg.MinRandHeightGap+targetBlockHeight {
		// (should not use subtraction because they are in the type of uint64)
		// we are running out of the randomness
		startHeight = lastCommittedHeight + 1
	} else {
		fp.logger.Debug(
			"the finality-provider has sufficient public randomness, skip committing more",
			zap.String("pk", fp.GetBtcPkHex()),
			zap.Uint64("block_height", targetBlockHeight),
			zap.Uint64("last_committed_height", lastCommittedHeight),
		)
		return nil, nil
	}

	// generate a list of Schnorr randomness pairs
	// NOTE: currently, calling this will create and save a list of randomness
	// in case of failure, randomness that has been created will be overwritten
	// for safety reason as the same randomness must not be used twice
	pubRandList, err := fp.GetPubRandList(startHeight, fp.cfg.NumPubRand)
	if err != nil {
		return nil, fmt.Errorf("failed to generate randomness: %w", err)
	}
	numPubRand := uint64(len(pubRandList))

	// generate commitment and proof for each public randomness
	commitment, proofList := types.GetPubRandCommitAndProofs(pubRandList)

	// store them to database
	if err := fp.pubRandState.AddPubRandProofList(pubRandList, proofList); err != nil {
		return nil, fmt.Errorf("failed to save public randomness to DB: %w", err)
	}

	// sign the commitment
	schnorrSig, err := fp.SignPubRandCommit(startHeight, numPubRand, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the Schnorr signature: %w", err)
	}

	res, err := fp.consumerCon.CommitPubRandList(fp.GetBtcPk(), startHeight, numPubRand, commitment, schnorrSig)
	if err != nil {
		return nil, fmt.Errorf("failed to commit public randomness to the consumer chain: %w", err)
	}

	// Update metrics
	fp.metrics.RecordFpRandomnessTime(fp.GetBtcPkHex())
	fp.metrics.RecordFpLastCommittedRandomnessHeight(fp.GetBtcPkHex(), lastCommittedHeight)
	fp.metrics.AddToFpTotalCommittedRandomness(fp.GetBtcPkHex(), float64(len(pubRandList)))

	return res, nil
}

// SubmitFinalitySignature builds and sends a finality signature over the given block to the consumer chain
func (fp *FinalityProviderInstance) SubmitFinalitySignature(b *types.BlockInfo) (*types.TxResponse, error) {
	sig, err := fp.SignFinalitySig(b)
	if err != nil {
		return nil, err
	}

	// get public randomness at the height
	prList, err := fp.GetPubRandList(b.Height, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to get public randomness list: %v", err)
	}
	pubRand := prList[0]

	// get inclusion proof
	proofBytes, err := fp.pubRandState.GetPubRandProof(pubRand)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get inclusion proof of public randomness %s for FP %s for block %d: %w",
			pubRand.String(),
			fp.btcPk.MarshalHex(),
			b.Height,
			err,
		)
	}

	// send finality signature to the consumer chain
	res, err := fp.consumerCon.SubmitFinalitySig(fp.GetBtcPk(), b, pubRand, proofBytes, sig.ToModNScalar())
	if err != nil {
		return nil, fmt.Errorf("failed to send finality signature to the consumer chain: %w", err)
	}

	// update DB
	fp.MustUpdateStateAfterFinalitySigSubmission(b.Height)

	// update metrics
	fp.metrics.RecordFpVoteTime(fp.GetBtcPkHex())
	fp.metrics.IncrementFpTotalVotedBlocks(fp.GetBtcPkHex())

	return res, nil
}

// SubmitBatchFinalitySignatures builds and sends a finality signature over the given block to the consumer chain
// NOTE: the input blocks should be in the ascending order of height
func (fp *FinalityProviderInstance) SubmitBatchFinalitySignatures(blocks []*types.BlockInfo) (*types.TxResponse, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("should not submit batch finality signature with zero block")
	}

	// get public randomness list
	prList, err := fp.GetPubRandList(blocks[0].Height, uint64(len(blocks)))
	if err != nil {
		return nil, fmt.Errorf("failed to get public randomness list: %v", err)
	}
	// get proof list
	// TODO: how to recover upon having an error in GetPubRandProofList?
	proofBytesList, err := fp.pubRandState.GetPubRandProofList(prList)
	if err != nil {
		return nil, fmt.Errorf("failed to get public randomness inclusion proof list: %v", err)
	}

	// sign blocks
	sigList := make([]*btcec.ModNScalar, 0, len(blocks))
	for _, b := range blocks {
		eotsSig, err := fp.SignFinalitySig(b)
		if err != nil {
			return nil, err
		}
		sigList = append(sigList, eotsSig.ToModNScalar())
	}

	// send finality signature to the consumer chain
	res, err := fp.consumerCon.SubmitBatchFinalitySigs(fp.GetBtcPk(), blocks, prList, proofBytesList, sigList)
	if err != nil {
		return nil, fmt.Errorf("failed to send a batch of finality signatures to the consumer chain: %w", err)
	}

	// update DB
	highBlock := blocks[len(blocks)-1]
	fp.MustUpdateStateAfterFinalitySigSubmission(highBlock.Height)

	return res, nil
}

// TestSubmitFinalitySignatureAndExtractPrivKey is exposed for presentation/testing purpose to allow manual sending finality signature
// this API is the same as SubmitFinalitySignature except that we don't constraint the voting height and update status
// Note: this should not be used in the submission loop
func (fp *FinalityProviderInstance) TestSubmitFinalitySignatureAndExtractPrivKey(b *types.BlockInfo) (*types.TxResponse, *btcec.PrivateKey, error) {
	// check last committed height
	lastCommittedHeight, err := fp.GetLastCommittedHeight()
	if err != nil {
		return nil, nil, err
	}
	if lastCommittedHeight < b.Height {
		return nil, nil, fmt.Errorf("the finality-provider's last committed height %v is lower than the current block height %v",
			lastCommittedHeight, b.Height)
	}

	// get public randomness
	prList, err := fp.GetPubRandList(b.Height, 1)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get public randomness list: %v", err)
	}
	pubRand := prList[0]

	// get proof
	proofBytes, err := fp.pubRandState.GetPubRandProof(pubRand)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get public randomness inclusion proof: %v", err)
	}

	// sign block
	eotsSig, err := fp.SignFinalitySig(b)
	if err != nil {
		return nil, nil, err
	}

	// send finality signature to the consumer chain
	res, err := fp.consumerCon.SubmitFinalitySig(fp.GetBtcPk(), b, pubRand, proofBytes, eotsSig.ToModNScalar())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send finality signature to the consumer chain: %w", err)
	}

	// try to extract the private key
	var privKey *btcec.PrivateKey
	events, err := parseCosmosEvents(res.Events)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode bytes to RelayerEvent: %s", err.Error())
	}
	for _, ev := range events {
		if strings.Contains(ev.EventType, "EventSlashedFinalityProvider") {
			evidenceStr := ev.Attributes["evidence"]
			fp.logger.Debug("found slashing evidence")
			var evidence ftypes.Evidence
			if err := jsonpb.UnmarshalString(evidenceStr, &evidence); err != nil {
				return nil, nil, fmt.Errorf("failed to decode evidence bytes to evidence: %s", err.Error())
			}
			privKey, err = evidence.ExtractBTCSK()
			if err != nil {
				return nil, nil, fmt.Errorf("failed to extract private key: %s", err.Error())
			}
			break
		}
	}

	return res, privKey, nil
}

func parseCosmosEvents(eventsData []byte) ([]provider.RelayerEvent, error) {
	var events []provider.RelayerEvent
	if err := json.Unmarshal(eventsData, &events); err != nil {
		return nil, fmt.Errorf("failed to decode bytes to RelayerEvent: %s", err.Error())
	}
	return events, nil
}

func (fp *FinalityProviderInstance) getPollerStartingHeight() (uint64, error) {
	if !fp.cfg.PollerConfig.AutoChainScanningMode {
		return fp.cfg.PollerConfig.StaticChainScanningStartHeight, nil
	}

	// Set initial block to the maximum of
	//    - last processed height + 1
	//    - the latest Babylon finalised height + 1
	// The above is to ensure that:
	//
	//	(1) Any finality-provider that is eligible to vote for a block,
	//	 doesn't miss submitting a vote for it.
	//	(2) The finality providers do not submit signatures for any already
	//	 finalised blocks.
	initialBlockToGet := fp.GetLastProcessedHeight()
	latestFinalizedBlock, err := fp.latestFinalizedBlockWithRetry()
	if err != nil {
		return 0, err
	}

	// find max(initialBlockToGet, latestFinalizedBlock.Height)
	maxHeight := initialBlockToGet
	if latestFinalizedBlock != nil && latestFinalizedBlock.Height > initialBlockToGet {
		maxHeight = latestFinalizedBlock.Height
	}

	return maxHeight + 1, nil
}

func (fp *FinalityProviderInstance) GetLastCommittedHeight() (uint64, error) {
	pubRandCommit, err := fp.lastCommittedPublicRandWithRetry()
	if err != nil {
		return 0, err
	}

	// no committed randomness yet
	if pubRandCommit == nil {
		return 0, nil
	}

	lastCommittedHeight := pubRandCommit.StartHeight + pubRandCommit.NumPubRand - 1

	return lastCommittedHeight, nil
}

func (fp *FinalityProviderInstance) lastCommittedPublicRandWithRetry() (*types.PubRandCommit, error) {
	var response *types.PubRandCommit
	if err := retry.Do(func() error {
		resp, err := fp.consumerCon.QueryLastPublicRandCommit(fp.GetBtcPk())
		if err != nil {
			return err
		}
		if resp != nil {
			if err := resp.Validate(); err != nil {
				return err
			}
		}
		response = resp
		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		fp.logger.Debug(
			"failed to query babylon for the last committed public randomness",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return nil, err
	}
	return response, nil
}

// nil will be returned if the finalized block does not exist
func (fp *FinalityProviderInstance) latestFinalizedBlockWithRetry() (*types.BlockInfo, error) {
	var response *types.BlockInfo
	if err := retry.Do(func() error {
		latestFinalizedBlock, err := fp.consumerCon.QueryLatestFinalizedBlock()
		if err != nil {
			return err
		}
		response = latestFinalizedBlock
		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		fp.logger.Debug(
			"failed to query babylon for the latest finalised blocks",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return nil, err
	}
	return response, nil
}

func (fp *FinalityProviderInstance) getLatestBlockHeightWithRetry() (uint64, error) {
	var (
		latestBlockHeight uint64
		err               error
	)

	if err := retry.Do(func() error {
		latestBlockHeight, err = fp.consumerCon.QueryLatestBlockHeight()
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
		hasPower, err = fp.consumerCon.QueryFinalityProviderHasPower(fp.GetBtcPk(), height)
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

func (fp *FinalityProviderInstance) GetFinalityProviderSlashedWithRetry() (bool, error) {
	var (
		slashed bool
		err     error
	)

	if err := retry.Do(func() error {
		slashed, err = fp.cc.QueryFinalityProviderSlashed(fp.GetBtcPk())
		if err != nil {
			return err
		}
		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		fp.logger.Debug(
			"failed to query the finality-provider",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return false, err
	}

	return slashed, nil
}

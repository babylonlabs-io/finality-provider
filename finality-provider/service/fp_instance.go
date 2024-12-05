package service

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	ftypes "github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/gogo/protobuf/jsonpb"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
)

type FinalityProviderInstance struct {
	btcPk *bbntypes.BIP340PubKey

	fpState      *fpState
	pubRandState *pubRandState
	cfg          *fpcfg.Config

	logger  *zap.Logger
	em      eotsmanager.EOTSManager
	cc      clientcontroller.ClientController
	poller  *ChainPoller
	metrics *metrics.FpMetrics

	// passphrase is used to unlock private keys
	passphrase string

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
	cc clientcontroller.ClientController,
	em eotsmanager.EOTSManager,
	metrics *metrics.FpMetrics,
	passphrase string,
	errChan chan<- *CriticalError,
	logger *zap.Logger,
) (*FinalityProviderInstance, error) {
	sfp, err := s.GetFinalityProvider(fpPk.MustToBTCPK())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve the finality provider %s from DB: %w", fpPk.MarshalHex(), err)
	}

	if !sfp.ShouldStart() {
		return nil, fmt.Errorf("the finality provider instance cannot be initiated with status %s", sfp.Status.String())
	}

	return newFinalityProviderInstanceFromStore(sfp, cfg, s, prStore, cc, em, metrics, passphrase, errChan, logger)
}

// Helper function to create FinalityProviderInstance from store data
func newFinalityProviderInstanceFromStore(
	sfp *store.StoredFinalityProvider,
	cfg *fpcfg.Config,
	s *store.FinalityProviderStore,
	prStore *store.PubRandProofStore,
	cc clientcontroller.ClientController,
	em eotsmanager.EOTSManager,
	metrics *metrics.FpMetrics,
	passphrase string,
	errChan chan<- *CriticalError,
	logger *zap.Logger,
) (*FinalityProviderInstance, error) {
	return &FinalityProviderInstance{
		btcPk:           bbntypes.NewBIP340PubKeyFromBTCPK(sfp.BtcPk),
		fpState:         newFpState(sfp, s),
		pubRandState:    newPubRandState(prStore),
		cfg:             cfg,
		logger:          logger,
		isStarted:       atomic.NewBool(false),
		criticalErrChan: errChan,
		passphrase:      passphrase,
		em:              em,
		cc:              cc,
		metrics:         metrics,
	}, nil
}

func (fp *FinalityProviderInstance) Start() error {
	if fp.isStarted.Swap(true) {
		return fmt.Errorf("the finality-provider instance %s is already started", fp.GetBtcPkHex())
	}

	if fp.IsJailed() {
		return fmt.Errorf("%w: %s", ErrFinalityProviderJailed, fp.GetBtcPkHex())
	}

	fp.logger.Info("Starting finality-provider instance", zap.String("pk", fp.GetBtcPkHex()))

	startHeight, err := fp.DetermineStartHeight()
	if err != nil {
		return fmt.Errorf("failed to get the start height: %w", err)
	}

	fp.logger.Info("starting the finality provider",
		zap.String("pk", fp.GetBtcPkHex()), zap.Uint64("height", startHeight))

	poller := NewChainPoller(fp.logger, fp.cfg.PollerConfig, fp.cc, fp.metrics)

	if err := poller.Start(startHeight); err != nil {
		return fmt.Errorf("failed to start the poller with start height %d: %w", startHeight, err)
	}

	fp.poller = poller
	fp.quit = make(chan struct{})

	fp.wg.Add(2)
	go fp.finalitySigSubmissionLoop()
	go fp.randomnessCommitmentLoop()

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

func (fp *FinalityProviderInstance) IsRunning() bool {
	return fp.isStarted.Load()
}

func (fp *FinalityProviderInstance) IsJailed() bool {
	return fp.GetStatus() == proto.FinalityProviderStatus_JAILED
}

func (fp *FinalityProviderInstance) finalitySigSubmissionLoop() {
	defer fp.wg.Done()

	for {
		select {
		case <-time.After(fp.cfg.SignatureSubmissionInterval):
			pollerBlocks := fp.getAllBlocksFromChan()
			if len(pollerBlocks) == 0 {
				continue
			}
			targetHeight := pollerBlocks[len(pollerBlocks)-1].Height
			fp.logger.Debug("the finality-provider received new block(s), start processing",
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint64("start_height", pollerBlocks[0].Height),
				zap.Uint64("end_height", targetHeight),
			)
			res, err := fp.retrySubmitSigsUntilFinalized(pollerBlocks)
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
				"successfully submitted the finality signature to the consumer chain",
				zap.String("consumer_id", string(fp.GetChainID())),
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint64("start_height", pollerBlocks[0].Height),
				zap.Uint64("end_height", targetHeight),
				zap.String("tx_hash", res.TxHash),
			)

		case <-fp.quit:
			fp.logger.Info("the finality signature submission loop is closing")
			return
		}
	}
}

func (fp *FinalityProviderInstance) getAllBlocksFromChan() []*types.BlockInfo {
	var pollerBlocks []*types.BlockInfo
	for {
		select {
		case b := <-fp.poller.GetBlockInfoChan():
			// TODO: in cases of catching up, this could issue frequent RPC calls
			shouldProcess, err := fp.shouldProcessBlock(b)
			if err != nil {
				if !errors.Is(err, ErrFinalityProviderShutDown) {
					fp.reportCriticalErr(err)
				}
				break
			}
			if shouldProcess {
				pollerBlocks = append(pollerBlocks, b)
			}
			if len(pollerBlocks) == int(fp.cfg.BatchSubmissionSize) {
				return pollerBlocks
			}
		case <-fp.quit:
			fp.logger.Info("the get all blocks loop is closing")
			return nil
		default:
			return pollerBlocks
		}
	}
}

func (fp *FinalityProviderInstance) shouldProcessBlock(b *types.BlockInfo) (bool, error) {
	if b.Height <= fp.GetLastVotedHeight() {
		fp.logger.Debug(
			"the block height is lower than last processed height",
			zap.String("pk", fp.GetBtcPkHex()),
			zap.Uint64("block_height", b.Height),
			zap.Uint64("last_voted_height", fp.GetLastVotedHeight()),
		)
		return false, nil
	}

	// check whether the finality provider has voting power
	hasVp, err := fp.hasVotingPower(b)
	if err != nil {
		return false, err
	}
	if !hasVp {
		// the finality provider does not have voting power
		// and it will never will at this block
		fp.metrics.IncrementFpTotalBlocksWithoutVotingPower(fp.GetBtcPkHex())
		return false, nil
	}

	return true, nil
}

func (fp *FinalityProviderInstance) randomnessCommitmentLoop() {
	defer fp.wg.Done()

	commitRandTicker := time.NewTicker(fp.cfg.RandomnessCommitInterval)
	defer commitRandTicker.Stop()

	for {
		select {
		case <-commitRandTicker.C:
			tipBlock, err := fp.getLatestBlockWithRetry()
			if err != nil {
				fp.reportCriticalErr(err)
				continue
			}
			txRes, err := fp.retryCommitPubRandUntilBlockFinalized(tipBlock)
			if err != nil {
				fp.metrics.IncrementFpTotalFailedRandomness(fp.GetBtcPkHex())
				fp.reportCriticalErr(err)
				continue
			}
			// txRes could be nil if no need to commit more randomness
			if txRes != nil {
				fp.logger.Info(
					"successfully committed public randomness to the consumer chain",
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

func (fp *FinalityProviderInstance) hasVotingPower(b *types.BlockInfo) (bool, error) {
	power, err := fp.GetVotingPowerWithRetry(b.Height)
	if err != nil {
		return false, err
	}
	if power == 0 {
		fp.logger.Debug(
			"the finality-provider does not have voting power",
			zap.String("pk", fp.GetBtcPkHex()),
			zap.Uint64("block_height", b.Height),
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

// retrySubmitSigsUntilFinalized periodically tries to submit finality signature until success or the block is finalized
// error will be returned if maximum retries have been reached or the query to the consumer chain fails
func (fp *FinalityProviderInstance) retrySubmitSigsUntilFinalized(targetBlocks []*types.BlockInfo) (*types.TxResponse, error) {
	if len(targetBlocks) == 0 {
		return nil, fmt.Errorf("cannot send signatures for empty blocks")
	}

	var failedCycles uint32
	targetHeight := targetBlocks[len(targetBlocks)-1].Height

	// we break the for loop if the block is finalized or the signature is successfully submitted
	// error will be returned if maximum retries have been reached or the query to the consumer chain fails
	for {
		select {
		case <-time.After(fp.cfg.SubmissionRetryInterval):
			// error will be returned if max retries have been reached
			var res *types.TxResponse
			var err error
			res, err = fp.SubmitBatchFinalitySignatures(targetBlocks)
			if err != nil {
				fp.logger.Debug(
					"failed to submit finality signature to the consumer chain",
					zap.String("pk", fp.GetBtcPkHex()),
					zap.Uint32("current_failures", failedCycles),
					zap.Uint64("target_start_height", targetBlocks[0].Height),
					zap.Uint64("target_end_height", targetHeight),
					zap.Error(err),
				)

				if clientcontroller.IsUnrecoverable(err) {
					return nil, err
				}

				if clientcontroller.IsExpected(err) {
					return nil, nil
				}

				failedCycles++
				if failedCycles > fp.cfg.MaxSubmissionRetries {
					return nil, fmt.Errorf("reached max failed cycles with err: %w", err)
				}
			} else {
				// the signature has been successfully submitted
				return res, nil
			}

			// periodically query the index block to be later checked whether it is Finalized
			finalized, err := fp.checkBlockFinalization(targetHeight)
			if err != nil {
				return nil, fmt.Errorf("failed to query block finalization at height %v: %w", targetHeight, err)
			}
			if finalized {
				fp.logger.Debug(
					"the block is already finalized, skip submission",
					zap.String("pk", fp.GetBtcPkHex()),
					zap.Uint64("target_height", targetHeight),
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

func (fp *FinalityProviderInstance) checkBlockFinalization(height uint64) (bool, error) {
	b, err := fp.cc.QueryBlock(height)
	if err != nil {
		return false, err
	}

	return b.Finalized, nil
}

// retryCommitPubRandUntilBlockFinalized periodically tries to commit public rand until success or the block is finalized
// error will be returned if maximum retries have been reached or the query to the consumer chain fails
func (fp *FinalityProviderInstance) retryCommitPubRandUntilBlockFinalized(targetBlock *types.BlockInfo) (*types.TxResponse, error) {
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
		res, err := fp.CommitPubRand(targetBlock.Height)
		if err != nil {
			if clientcontroller.IsUnrecoverable(err) {
				return nil, err
			}
			fp.logger.Debug(
				"failed to commit public randomness to the consumer chain",
				zap.String("pk", fp.GetBtcPkHex()),
				zap.Uint32("current_failures", failedCycles),
				zap.Uint64("target_block_height", targetBlock.Height),
				zap.Error(err),
			)

			failedCycles++
			if failedCycles > fp.cfg.MaxSubmissionRetries {
				return nil, fmt.Errorf("reached max failed cycles with err: %w", err)
			}
		} else {
			// the public randomness has been successfully submitted
			return res, nil
		}
		select {
		case <-time.After(fp.cfg.SubmissionRetryInterval):
			// periodically query the index block to be later checked whether it is Finalized
			finalized, err := fp.checkBlockFinalization(targetBlock.Height)
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
			return nil, nil
		}
	}
}

// CommitPubRand generates a list of Schnorr rand pairs,
// commits the public randomness for the managed finality providers,
// and save the randomness pair to DB
// Note:
// - if there is no pubrand committed before, it will start from the tipHeight
// - if the tipHeight is too large, it will only commit fp.cfg.NumPubRand pairs
func (fp *FinalityProviderInstance) CommitPubRand(tipHeight uint64) (*types.TxResponse, error) {
	lastCommittedHeight, err := fp.GetLastCommittedHeight()
	if err != nil {
		return nil, err
	}

	var startHeight uint64
	switch {
	case lastCommittedHeight == uint64(0):
		// the finality-provider has never submitted public rand before
		startHeight = tipHeight + 1
	case lastCommittedHeight < uint64(fp.cfg.MinRandHeightGap)+tipHeight:
		// (should not use subtraction because they are in the type of uint64)
		// we are running out of the randomness
		startHeight = lastCommittedHeight + 1
	default:
		fp.logger.Debug(
			"the finality-provider has sufficient public randomness, skip committing more",
			zap.String("pk", fp.GetBtcPkHex()),
			zap.Uint64("block_height", tipHeight),
			zap.Uint64("last_committed_height", lastCommittedHeight),
		)
		return nil, nil
	}

	return fp.commitPubRandPairs(startHeight)
}

// it will commit fp.cfg.NumPubRand pairs of public randomness starting from startHeight
func (fp *FinalityProviderInstance) commitPubRandPairs(startHeight uint64) (*types.TxResponse, error) {
	activationBlkHeight, err := fp.cc.QueryFinalityActivationBlockHeight()
	if err != nil {
		return nil, err
	}

	// make sure that the start height is at least the finality activation height
	// and updated to generate the list with the same as the committed height.
	startHeight = max(startHeight, activationBlkHeight)
	// generate a list of Schnorr randomness pairs
	// NOTE: currently, calling this will create and save a list of randomness
	// in case of failure, randomness that has been created will be overwritten
	// for safety reason as the same randomness must not be used twice
	pubRandList, err := fp.getPubRandList(startHeight, fp.cfg.NumPubRand)
	if err != nil {
		return nil, fmt.Errorf("failed to generate randomness: %w", err)
	}
	numPubRand := uint64(len(pubRandList))

	// generate commitment and proof for each public randomness
	commitment, proofList := types.GetPubRandCommitAndProofs(pubRandList)

	// store them to database
	if err := fp.pubRandState.addPubRandProofList(pubRandList, proofList); err != nil {
		return nil, fmt.Errorf("failed to save public randomness to DB: %w", err)
	}

	// sign the commitment
	schnorrSig, err := fp.signPubRandCommit(startHeight, numPubRand, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the Schnorr signature: %w", err)
	}

	res, err := fp.cc.CommitPubRandList(fp.GetBtcPk(), startHeight, numPubRand, commitment, schnorrSig)
	if err != nil {
		return nil, fmt.Errorf("failed to commit public randomness to the consumer chain: %w", err)
	}

	// Update metrics
	fp.metrics.RecordFpRandomnessTime(fp.GetBtcPkHex())
	fp.metrics.RecordFpLastCommittedRandomnessHeight(fp.GetBtcPkHex(), startHeight+numPubRand-1)
	fp.metrics.AddToFpTotalCommittedRandomness(fp.GetBtcPkHex(), float64(len(pubRandList)))

	return res, nil
}

// TestCommitPubRand is exposed for devops/testing purpose to allow manual committing public randomness in cases
// where FP is stuck due to lack of public randomness.
//
// Note:
// - this function is similar to `CommitPubRand` but should not be used in the main pubrand submission loop.
// - it will always start from the last committed height + 1
// - if targetBlockHeight is too large, it will commit multiple fp.cfg.NumPubRand pairs in a loop until reaching the targetBlockHeight
func (fp *FinalityProviderInstance) TestCommitPubRand(targetBlockHeight uint64) error {
	var startHeight, lastCommittedHeight uint64

	lastCommittedHeight, err := fp.GetLastCommittedHeight()
	if err != nil {
		return err
	}

	if lastCommittedHeight >= targetBlockHeight {
		return fmt.Errorf(
			"finality provider has already committed pubrand to target block height (pk: %s, target: %d, last committed: %d)",
			fp.GetBtcPkHex(),
			targetBlockHeight,
			lastCommittedHeight,
		)
	}

	if lastCommittedHeight == uint64(0) {
		// Note: it can also be the case that the finality-provider has committed 1 pubrand before (but in practice, we
		// will never set cfg.NumPubRand to 1. so we can safely assume it has never committed before)
		startHeight = 0
	} else {
		startHeight = lastCommittedHeight + 1
	}

	return fp.TestCommitPubRandWithStartHeight(startHeight, targetBlockHeight)
}

// TestCommitPubRandWithStartHeight is exposed for devops/testing purpose to allow manual committing public randomness
// in cases where FP is stuck due to lack of public randomness.
func (fp *FinalityProviderInstance) TestCommitPubRandWithStartHeight(startHeight uint64, targetBlockHeight uint64) error {
	if startHeight > targetBlockHeight {
		return fmt.Errorf("start height should not be greater than target block height")
	}

	var lastCommittedHeight uint64
	lastCommittedHeight, err := fp.GetLastCommittedHeight()
	if err != nil {
		return err
	}
	if lastCommittedHeight >= startHeight {
		return fmt.Errorf(
			"finality provider has already committed pubrand at the start height (pk: %s, startHeight: %d, lastCommittedHeight: %d)",
			fp.GetBtcPkHex(),
			startHeight,
			lastCommittedHeight,
		)
	}

	fp.logger.Info("Start committing pubrand from block height", zap.Uint64("start_height", startHeight))

	// TODO: instead of sending multiple txs, a better way is to bundle all the commit messages into
	// one like we do for batch finality signatures. see discussion https://bit.ly/3OmbjkN
	for startHeight <= targetBlockHeight {
		_, err = fp.commitPubRandPairs(startHeight)
		if err != nil {
			return err
		}
		lastCommittedHeight = startHeight + uint64(fp.cfg.NumPubRand) - 1
		startHeight = lastCommittedHeight + 1
		fp.logger.Info("Committed pubrand to block height", zap.Uint64("height", lastCommittedHeight))
	}

	// no error. success
	return nil
}

// SubmitFinalitySignature builds and sends a finality signature over the given block to the consumer chain
func (fp *FinalityProviderInstance) SubmitFinalitySignature(b *types.BlockInfo) (*types.TxResponse, error) {
	return fp.SubmitBatchFinalitySignatures([]*types.BlockInfo{b})
}

// SubmitBatchFinalitySignatures builds and sends a finality signature over the given block to the consumer chain
// NOTE: the input blocks should be in the ascending order of height
func (fp *FinalityProviderInstance) SubmitBatchFinalitySignatures(blocks []*types.BlockInfo) (*types.TxResponse, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("should not submit batch finality signature with zero block")
	}

	if len(blocks) > math.MaxUint32 {
		return nil, fmt.Errorf("should not submit batch finality signature with too many blocks")
	}

	// get public randomness list
	// #nosec G115 -- performed the conversion check above
	prList, err := fp.getPubRandList(blocks[0].Height, uint32(len(blocks)))
	if err != nil {
		return nil, fmt.Errorf("failed to get public randomness list: %w", err)
	}
	// get proof list
	// TODO: how to recover upon having an error in getPubRandProofList?
	proofBytesList, err := fp.pubRandState.getPubRandProofList(prList)
	if err != nil {
		return nil, fmt.Errorf("failed to get public randomness inclusion proof list: %w", err)
	}

	// sign blocks
	sigList := make([]*btcec.ModNScalar, 0, len(blocks))
	for _, b := range blocks {
		eotsSig, err := fp.signFinalitySig(b)
		if err != nil {
			return nil, err
		}
		sigList = append(sigList, eotsSig.ToModNScalar())
	}

	// send finality signature to the consumer chain
	res, err := fp.cc.SubmitBatchFinalitySigs(fp.GetBtcPk(), blocks, prList, proofBytesList, sigList)
	if err != nil {
		if strings.Contains(err.Error(), "jailed") {
			return nil, ErrFinalityProviderJailed
		}
		if strings.Contains(err.Error(), "slashed") {
			return nil, ErrFinalityProviderSlashed
		}
		return nil, err
	}

	// update DB
	highBlock := blocks[len(blocks)-1]
	fp.MustUpdateStateAfterFinalitySigSubmission(highBlock.Height)

	return res, nil
}

// TestSubmitFinalitySignatureAndExtractPrivKey is exposed for presentation/testing purpose to allow manual sending finality signature
// this API is the same as SubmitBatchFinalitySignatures except that we don't constraint the voting height and update status
// Note: this should not be used in the submission loop
func (fp *FinalityProviderInstance) TestSubmitFinalitySignatureAndExtractPrivKey(b *types.BlockInfo) (*types.TxResponse, *btcec.PrivateKey, error) {
	// get public randomness
	prList, err := fp.getPubRandList(b.Height, 1)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get public randomness list: %w", err)
	}
	pubRand := prList[0]

	// get proof
	proofBytes, err := fp.pubRandState.getPubRandProof(pubRand)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get public randomness inclusion proof: %w", err)
	}

	// sign block
	eotsSig, err := fp.signFinalitySig(b)
	if err != nil {
		return nil, nil, err
	}

	// send finality signature to the consumer chain
	res, err := fp.cc.SubmitFinalitySig(fp.GetBtcPk(), b, pubRand, proofBytes, eotsSig.ToModNScalar())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send finality signature to the consumer chain: %w", err)
	}

	if res.TxHash == "" {
		return res, nil, nil
	}

	// try to extract the private key
	var privKey *btcec.PrivateKey
	for _, ev := range res.Events {
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

// DetermineStartHeight determines start height for block processing by:
//
// If AutoChainScanningMode is disabled:
//   - Returns StaticChainScanningStartHeight from config
//
// If AutoChainScanningMode is enabled:
//   - Gets finalityActivationHeight from chain
//   - Gets lastFinalizedHeight from chain
//   - Gets lastVotedHeight from local state
//   - If fp.GetLastVotedHeight() is 0, sets lastVotedHeight = lastFinalizedHeight
//   - Gets highestVotedHeight from chain
//   - Sets lastVotedHeight = max(lastVotedHeight, highestVotedHeight)
//   - Returns max(finalityActivationHeight, lastVotedHeight + 1)
//
// This ensures that:
// 1. The FP will not vote for heights below the finality activation height
// 2. The FP will resume from its last voting position or the chain's last finalized height
// 3. The FP will not process blocks it has already voted on
//
// Note: Starting from lastFinalizedHeight when there's a gap to the last processed height
// may result in missed rewards, depending on the consumer chain's reward distribution mechanism.
func (fp *FinalityProviderInstance) DetermineStartHeight() (uint64, error) {
	// start from a height from config if AutoChainScanningMode is disabled
	if !fp.cfg.PollerConfig.AutoChainScanningMode {
		fp.logger.Info("using static chain scanning mode",
			zap.String("pk", fp.GetBtcPkHex()),
			zap.Uint64("start_height", fp.cfg.PollerConfig.StaticChainScanningStartHeight))
		return fp.cfg.PollerConfig.StaticChainScanningStartHeight, nil
	}

	lastFinalizedHeight, err := fp.latestFinalizedHeightWithRetry()
	if err != nil {
		return 0, fmt.Errorf("failed to get the last finalized height: %w", err)
	}

	// determine an effective lastVotedHeight
	var lastVotedHeight uint64
	if fp.GetLastVotedHeight() == 0 {
		lastVotedHeight = lastFinalizedHeight
	} else {
		lastVotedHeight = fp.GetLastVotedHeight()
	}

	highestVotedHeight, err := fp.highestVotedHeightWithRetry()
	if err != nil {
		return 0, fmt.Errorf("failed to get the highest voted height: %w", err)
	}

	// TODO: if highestVotedHeight > lastVotedHeight, using highestVotedHeight could lead
	// to issues when there are missed blocks between the gap due to bugs.
	// A proper solution is to check if the fp has voted for each block within the gap
	lastVotedHeight = max(lastVotedHeight, highestVotedHeight)

	finalityActivationHeight, err := fp.getFinalityActivationHeightWithRetry()
	if err != nil {
		return 0, fmt.Errorf("failed to get finality activation height: %w", err)
	}

	// determine the final starting height
	startHeight := max(finalityActivationHeight, lastVotedHeight+1)

	// log how start height is determined
	fp.logger.Info("determined poller starting height",
		zap.String("pk", fp.GetBtcPkHex()),
		zap.Uint64("start_height", startHeight),
		zap.Uint64("finality_activation_height", finalityActivationHeight),
		zap.Uint64("last_voted_height", fp.GetLastVotedHeight()),
		zap.Uint64("last_finalized_height", lastFinalizedHeight),
		zap.Uint64("highest_voted_height", highestVotedHeight))

	return startHeight, nil
}

func (fp *FinalityProviderInstance) GetLastCommittedHeight() (uint64, error) {
	pubRandCommitMap, err := fp.lastCommittedPublicRandWithRetry(1)
	if err != nil {
		return 0, err
	}

	// no committed randomness yet
	if len(pubRandCommitMap) == 0 {
		return 0, nil
	}

	if len(pubRandCommitMap) > 1 {
		return 0, fmt.Errorf("got more than one last committed public randomness")
	}
	var lastCommittedHeight uint64
	for startHeight, resp := range pubRandCommitMap {
		lastCommittedHeight = startHeight + resp.NumPubRand - 1
	}

	return lastCommittedHeight, nil
}

func (fp *FinalityProviderInstance) lastCommittedPublicRandWithRetry(count uint64) (map[uint64]*ftypes.PubRandCommitResponse, error) {
	var response map[uint64]*ftypes.PubRandCommitResponse
	if err := retry.Do(func() error {
		resp, err := fp.cc.QueryLastCommittedPublicRand(fp.GetBtcPk(), count)
		if err != nil {
			return err
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

func (fp *FinalityProviderInstance) latestFinalizedHeightWithRetry() (uint64, error) {
	var height uint64
	if err := retry.Do(func() error {
		blocks, err := fp.cc.QueryLatestFinalizedBlocks(1)
		if err != nil {
			return err
		}
		if len(blocks) == 0 {
			// no finalized block yet
			return nil
		}
		height = blocks[0].Height
		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		fp.logger.Debug(
			"failed to query babylon for the latest finalised height",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return 0, err
	}

	return height, nil
}

func (fp *FinalityProviderInstance) highestVotedHeightWithRetry() (uint64, error) {
	var height uint64
	if err := retry.Do(func() error {
		h, err := fp.cc.QueryFinalityProviderHighestVotedHeight(fp.GetBtcPk())
		if err != nil {
			return err
		}
		height = h
		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		fp.logger.Debug(
			"failed to query babylon for the highest voted height",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return 0, err
	}

	return height, nil
}

func (fp *FinalityProviderInstance) getFinalityActivationHeightWithRetry() (uint64, error) {
	var response uint64
	if err := retry.Do(func() error {
		finalityActivationHeight, err := fp.cc.QueryFinalityActivationBlockHeight()
		if err != nil {
			return err
		}
		response = finalityActivationHeight
		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		fp.logger.Debug(
			"failed to query babylon for the finality activation height",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return 0, err
	}
	return response, nil
}

func (fp *FinalityProviderInstance) getLatestBlockWithRetry() (*types.BlockInfo, error) {
	var (
		latestBlock *types.BlockInfo
		err         error
	)

	if err := retry.Do(func() error {
		latestBlock, err = fp.cc.QueryBestBlock()
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
		return nil, err
	}
	fp.metrics.RecordBabylonTipHeight(latestBlock.Height)

	return latestBlock, nil
}

func (fp *FinalityProviderInstance) GetVotingPowerWithRetry(height uint64) (uint64, error) {
	var (
		power uint64
		err   error
	)

	if err := retry.Do(func() error {
		power, err = fp.cc.QueryFinalityProviderVotingPower(fp.GetBtcPk(), height)
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
		return 0, err
	}

	return power, nil
}

func (fp *FinalityProviderInstance) GetFinalityProviderSlashedOrJailedWithRetry() (bool, bool, error) {
	var (
		slashed bool
		jailed  bool
		err     error
	)

	if err := retry.Do(func() error {
		slashed, jailed, err = fp.cc.QueryFinalityProviderSlashedOrJailed(fp.GetBtcPk())
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
		return false, false, err
	}

	return slashed, jailed, nil
}

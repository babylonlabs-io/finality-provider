package service

import (
	"context"
	"fmt"
	"github.com/avast/retry-go/v4"
	ccapi "github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	cfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"sync"
	"time"
)

var (
	RtyAttNum = uint(5)
	RtyAtt    = retry.Attempts(RtyAttNum)
	RtyDel    = retry.Delay(time.Millisecond * 400)
	RtyErr    = retry.LastErrorOnly(true)
)

const (
	maxFailedCycles = 20
)

// Ensure ChainPoller implements BlockPoller interface
var _ types.BlockPoller[types.BlockDescription] = (*ChainPoller)(nil)

type ChainPoller struct {
	mu            sync.RWMutex
	wg            sync.WaitGroup
	isStarted     *atomic.Bool
	quit          chan struct{}
	consumerCon   ccapi.ConsumerController
	cfg           *cfg.ChainPollerConfig
	metrics       *metrics.FpMetrics
	logger        *zap.Logger
	nextHeight    uint64
	currentHeight uint64
	blockBuffer   []*types.BlockInfo
	bufferMu      sync.RWMutex
	blockReady    chan struct{}
}

func NewChainPoller(
	logger *zap.Logger,
	cfg *cfg.ChainPollerConfig,
	consumerCon ccapi.ConsumerController,
	metrics *metrics.FpMetrics,
) *ChainPoller {
	return &ChainPoller{
		isStarted:   atomic.NewBool(false),
		logger:      logger,
		cfg:         cfg,
		consumerCon: consumerCon,
		metrics:     metrics,
		quit:        make(chan struct{}),
		blockReady:  make(chan struct{}, 1),
		blockBuffer: make([]*types.BlockInfo, 0),
	}
}

// TryNextBlock implements BlockPoller interface - non-blocking version
func (cp *ChainPoller) TryNextBlock() (types.BlockDescription, bool) {
	if !cp.isStarted.Load() {
		return nil, false
	}

	cp.mu.RLock()
	if len(cp.blockBuffer) > 0 {
		block := cp.blockBuffer[0]
		cp.blockBuffer = cp.blockBuffer[1:]
		cp.currentHeight = block.Height
		cp.mu.RUnlock()

		return block, true
	}
	cp.mu.RUnlock()

	return nil, false
}

// SetStartHeight implements BlockPoller interface
func (cp *ChainPoller) SetStartHeight(ctx context.Context, height uint64) error {
	if cp.isStarted.Swap(true) {
		return fmt.Errorf("the chain poller has already started")
	}

	cp.logger.Info("starting the chain poller", zap.Uint64("start_height", height))

	cp.mu.Lock()
	cp.nextHeight = height
	cp.mu.Unlock()

	cp.wg.Add(1)
	go cp.pollChain(ctx)

	cp.metrics.RecordPollerStartingHeight(height)
	cp.logger.Info("the chain poller is successfully started")

	return nil
}

// CurrentHeight implements BlockPoller interface
func (cp *ChainPoller) CurrentHeight() uint64 {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	return cp.currentHeight
}

func (cp *ChainPoller) Stop() error {
	if !cp.isStarted.Swap(false) {
		return fmt.Errorf("the chain poller has already stopped")
	}

	cp.logger.Info("stopping the chain poller")
	if err := cp.consumerCon.Close(); err != nil {
		return err
	}

	close(cp.quit)
	cp.wg.Wait()

	cp.logger.Info("the chain poller is successfully stopped")

	return nil
}

func (cp *ChainPoller) IsRunning() bool {
	return cp.isStarted.Load()
}

func (cp *ChainPoller) blocksWithRetry(start, end uint64, limit uint32) ([]*types.BlockInfo, error) {
	var (
		block []*types.BlockInfo
		err   error
	)
	if err := retry.Do(func() error {
		block, err = cp.consumerCon.QueryBlocks(start, end, limit)
		if err != nil {
			return err
		}

		// no need return error when just no found new blocks,
		// the chain to poller may not produce new blocks.
		if len(block) == 0 {
			cp.logger.Debug(
				"no blocks found for range",
				zap.Uint64("start_height", start),
				zap.Uint64("end_height", end),
			)
		}

		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		cp.logger.Debug(
			"failed to query the consumer chain for block range",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Uint64("start_height", start),
			zap.Uint64("end_height", end),
			zap.Uint32("limit", limit),
			zap.Error(err),
		)
	})); err != nil {
		return nil, err
	}

	return block, nil
}

func (cp *ChainPoller) latestBlockHeightWithRetry() (uint64, error) {
	var (
		latestBlockHeight uint64
		err               error
	)

	if err := retry.Do(func() error {
		latestBlockHeight, err = cp.consumerCon.QueryLatestBlockHeight()
		if err != nil {
			return err
		}

		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		cp.logger.Debug(
			"failed to query the consumer chain for the latest block",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return 0, err
	}

	return latestBlockHeight, nil
}

// waitForActivation waits until BTC staking is activated
func (cp *ChainPoller) waitForActivation(ctx context.Context) {
	// ensure that the startHeight is no lower than the activated height
	for {
		activatedHeight, err := cp.consumerCon.QueryActivatedHeight()
		if err != nil {
			// TODO: distinguish between "BTC staking is not activated" and other errors
			cp.logger.Debug("failed to query the consumer chain for the activated height", zap.Error(err))
		} else {
			cp.mu.Lock()
			if cp.nextHeight < activatedHeight {
				cp.nextHeight = activatedHeight
			}
			cp.mu.Unlock()

			return
		}
		select {
		case <-time.After(cp.cfg.PollInterval):
			continue
		case <-cp.quit:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (cp *ChainPoller) pollChain(ctx context.Context) {
	defer cp.wg.Done()

	cp.waitForActivation(ctx)

	ticker := time.NewTicker(cp.cfg.PollInterval)
	defer ticker.Stop()

	var failedCycles uint32

	for {
		select {
		case <-cp.quit:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Continue with polling logic
		}

		latestBlockHeight, err := cp.latestBlockHeightWithRetry()
		if err != nil {
			failedCycles++
			cp.logger.Debug(
				"failed to query the consumer chain for the latest block",
				zap.Uint32("current_failures", failedCycles),
				zap.Error(err),
			)
		} else {
			// start polling in the first iteration
			blockToRetrieve := cp.getNextHeight()
			err := cp.tryPollChain(latestBlockHeight, blockToRetrieve)

			// update the failed cycles
			if err != nil {
				failedCycles++
				cp.logger.Debug(
					"failed to query the consumer chain for the block range",
					zap.Uint32("current_failures", failedCycles),
					zap.Uint64("start_height", blockToRetrieve),
					zap.Uint64("end_height", latestBlockHeight),
					zap.Error(err),
				)
			} else {
				// no error and we got the header we wanted to get, bump the state and push
				// notification about data
				if failedCycles > 0 {
					cp.logger.Debug(
						"query the consumer chain for the block range success from an error",
						zap.Uint32("current_failures", failedCycles),
						zap.Uint64("start_height", blockToRetrieve),
						zap.Uint64("end_height", latestBlockHeight),
					)
				}
				failedCycles = 0
			}
		}

		if failedCycles > maxFailedCycles {
			cp.logger.Fatal("the poller has reached the max failed cycles, exiting")
		}
	}
}

func (cp *ChainPoller) tryPollChain(latestBlockHeight, blockToRetrieve uint64) error {
	var blocks []*types.BlockInfo
	var err error

	switch {
	case blockToRetrieve > latestBlockHeight:
		cp.logger.Debug(
			"skipping block query as there is no new block",
			zap.Uint64("next_height", blockToRetrieve),
			zap.Uint64("latest_height", latestBlockHeight),
		)

		return nil
	case blockToRetrieve == latestBlockHeight:
		var latestBlock *types.BlockInfo
		latestBlock, err = cp.consumerCon.QueryBlock(latestBlockHeight)
		if err != nil {
			return err
		}
		blocks = []*types.BlockInfo{latestBlock}
	default:
		blocks, err = cp.blocksWithRetry(blockToRetrieve, latestBlockHeight, cp.cfg.PollSize)
		if err != nil {
			return err
		}
	}

	// no error and we got the blocks we wanted to get
	if len(blocks) == 0 {
		return nil
	}

	lb := blocks[len(blocks)-1]
	cp.setNextHeight(lb.Height + 1)

	cp.metrics.RecordLastPolledHeight(lb.Height)

	cp.logger.Info("the poller retrieved the blocks from the consumer chain",
		zap.Uint64("start_height", blockToRetrieve),
		zap.Uint64("end_height", lb.Height))

	// Add blocks to buffer
	cp.bufferMu.Lock()
	cp.blockBuffer = append(cp.blockBuffer, blocks...)
	cp.bufferMu.Unlock()

	// Signal that new blocks are available
	select {
	case cp.blockReady <- struct{}{}:
	default:
	}

	return nil
}

func (cp *ChainPoller) getNextHeight() uint64 {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	return cp.nextHeight
}

func (cp *ChainPoller) setNextHeight(height uint64) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.nextHeight = height
}

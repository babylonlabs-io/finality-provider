package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	ccapi "github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	cfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

var (
	RtyAttNum = uint(5)
	RtyAtt    = retry.Attempts(RtyAttNum)
	RtyDel    = retry.Delay(time.Millisecond * 400)
	RtyErr    = retry.LastErrorOnly(true)
)

const (
	maxFailedCycles   = 20
	defaultBufferSize = 100
)

var _ types.BlockPoller[types.BlockDescription] = (*ChainPoller)(nil)

// ChainPoller is responsible for polling the blockchain for new blocks and sending them to a processing channel.
type ChainPoller struct {
	mu sync.RWMutex

	wg        sync.WaitGroup
	isStarted *atomic.Bool
	quit      chan struct{}

	consumerCon ccapi.ConsumerController
	cfg         *cfg.ChainPollerConfig
	metrics     *metrics.FpMetrics
	logger      *zap.Logger

	nextHeight uint64

	blockChan     chan types.BlockDescription
	blockChanSize int
}

func NewChainPoller(
	logger *zap.Logger,
	cfg *cfg.ChainPollerConfig,
	consumerCon ccapi.ConsumerController,
	metrics *metrics.FpMetrics,
) *ChainPoller {
	bufferSize := defaultBufferSize
	if cfg.BufferSize > 0 {
		bufferSize = int(cfg.BufferSize)
	}

	return &ChainPoller{
		isStarted:     atomic.NewBool(false),
		logger:        logger,
		cfg:           cfg,
		consumerCon:   consumerCon,
		metrics:       metrics,
		quit:          make(chan struct{}),
		blockChan:     make(chan types.BlockDescription, bufferSize),
		blockChanSize: bufferSize,
	}
}

// TryNextBlock - non-blocking return of the next block
func (cp *ChainPoller) TryNextBlock() (types.BlockDescription, bool) {
	if !cp.isStarted.Load() {
		return nil, false
	}

	select {
	case block := <-cp.blockChan:
		if block == nil {
			return nil, false
		}

		return block, true
	default:
		return nil, false
	}
}

// NextBlock - blocking version that waits for the next block
func (cp *ChainPoller) NextBlock(ctx context.Context) (types.BlockDescription, error) {
	if !cp.isStarted.Load() {
		return nil, fmt.Errorf("chain poller is not running")
	}

	select {
	case block := <-cp.blockChan:
		if block == nil {
			return nil, fmt.Errorf("received nil block from channel")
		}

		return block, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-cp.quit:
		return nil, fmt.Errorf("chain poller is shutting down")
	}
}

// SetStartHeight configures the starting block height for the chain poller and begins polling from this height.
func (cp *ChainPoller) SetStartHeight(ctx context.Context, height uint64) error {
	if cp.isStarted.Swap(true) {
		return fmt.Errorf("the chain poller has already started")
	}

	cp.logger.Info("starting the chain poller",
		zap.Uint64("start_height", height),
		zap.Int("buffer_size", cp.blockChanSize))

	cp.mu.Lock()
	cp.nextHeight = height
	cp.quit = make(chan struct{})
	cp.blockChan = make(chan types.BlockDescription, cp.blockChanSize)
	cp.mu.Unlock()

	cp.wg.Add(1)
	go cp.pollChain(ctx)

	cp.metrics.RecordPollerStartingHeight(height)
	cp.logger.Info("the chain poller is successfully started")

	return nil
}

// Stop stops the chain poller, waits for the polling goroutine to finish, and closes the block channel.
func (cp *ChainPoller) Stop() error {
	if !cp.isStarted.Swap(false) {
		return fmt.Errorf("the chain poller has already stopped")
	}

	cp.logger.Info("stopping the chain poller")
	close(cp.quit)

	cp.wg.Wait()

	close(cp.blockChan)

	// Close connection
	if err := cp.consumerCon.Close(); err != nil {
		cp.logger.Error("failed to close consumer connection", zap.Error(err))

		return err
	}

	cp.logger.Info("the chain poller is successfully stopped")

	return nil
}

// waitForActivation waits until BTC staking is activated, adjusting the start height if necessary.
func (cp *ChainPoller) waitForActivation(ctx context.Context) error {
	cp.logger.Info("waiting for BTC staking activation")
	ticker := time.NewTicker(cp.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-cp.quit:
			return fmt.Errorf("poller shutting down")
		case <-ticker.C:
			activatedHeight, err := cp.consumerCon.QueryActivatedHeight(ctx)
			if err != nil {
				cp.logger.Debug("BTC staking not yet activated", zap.Error(err))

				continue
			}

			cp.mu.Lock()
			if cp.nextHeight < activatedHeight {
				cp.logger.Info("adjusting start height to activation height",
					zap.Uint64("old_height", cp.nextHeight),
					zap.Uint64("activation_height", activatedHeight))
				cp.nextHeight = activatedHeight
			}
			cp.mu.Unlock()

			cp.logger.Info("BTC staking is activated", zap.Uint64("activation_height", activatedHeight))

			return nil
		}
	}
}

// pollChain continuously polls the blockchain for new blocks and processes them until the context or quit signal is triggered.
func (cp *ChainPoller) pollChain(ctx context.Context) {
	defer cp.wg.Done()

	if err := cp.waitForActivation(ctx); err != nil {
		cp.logger.Error("failed to wait for activation", zap.Error(err))

		return
	}

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
			if err := cp.pollCycle(ctx); err != nil {
				failedCycles++
				cp.logger.Debug("poll cycle failed",
					zap.Uint32("current_failures", failedCycles),
					zap.Error(err))

				if failedCycles > maxFailedCycles {
					cp.logger.Fatal("the poller has reached the max failed cycles, exiting")

					return
				}
			} else {
				if failedCycles > 0 {
					cp.logger.Debug("poll cycle recovered from errors",
						zap.Uint32("recovered_from_failures", failedCycles))
				}
				failedCycles = 0
			}
		}
	}
}

func (cp *ChainPoller) pollCycle(ctx context.Context) error {
	latestBlockHeight, err := cp.latestBlockHeightWithRetry(ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest block height: %w", err)
	}

	blockToRetrieve := cp.getNextHeight()

	return cp.tryPollChain(ctx, latestBlockHeight, blockToRetrieve)
}

// tryPollChain attempts to fetch a range of blocks from the chain and sends them to a processing channel with backpressure handling.
// It handles cases where the block range starts beyond the latest height, matches the latest height, or spans multiple heights.
// If the poller or context is terminated, it ensures proper cleanup and error handling.
func (cp *ChainPoller) tryPollChain(ctx context.Context, latestBlockHeight, blockToRetrieve uint64) error {
	var blocks []types.BlockDescription
	var err error

	switch {
	case blockToRetrieve > latestBlockHeight:
		cp.logger.Debug("no new blocks available",
			zap.Uint64("next_height", blockToRetrieve),
			zap.Uint64("latest_height", latestBlockHeight))

		return nil

	case blockToRetrieve == latestBlockHeight:
		var latestBlock types.BlockDescription
		latestBlock, err = cp.consumerCon.QueryBlock(ctx, latestBlockHeight)
		if err != nil {
			return fmt.Errorf("failed to query latest block: %w", err)
		}
		blocks = []types.BlockDescription{latestBlock}

	default:
		blocks, err = cp.blocksWithRetry(ctx, blockToRetrieve, latestBlockHeight, cp.cfg.PollSize)
		if err != nil {
			return fmt.Errorf("failed to query block range: %w", err)
		}
	}

	if len(blocks) == 0 {
		return nil
	}

	// Send blocks to a channel with backpressure handling
	for _, block := range blocks {
		select {
		case <-cp.quit:
			return fmt.Errorf("poller shutting down")
		case cp.blockChan <- block:
			// Block sent successfully
		case <-ctx.Done():
			return ctx.Err()
		default:
			cp.logger.Warn("block channel is full, consumer may be slow",
				zap.Int("buffer_used", len(cp.blockChan)),
				zap.Int("buffer_capacity", cp.blockChanSize),
				zap.Uint64("block_height", block.GetHeight()))

			select {
			case cp.blockChan <- block:
				cp.logger.Debug("block sent after backpressure delay",
					zap.Uint64("block_height", block.GetHeight()))
			case <-time.After(time.Second * 30):
				return fmt.Errorf("failed to send block %d: channel full for too long", block.GetHeight())
			case <-ctx.Done():
				return ctx.Err()
			case <-cp.quit:
				return fmt.Errorf("poller shutting down")
			}
		}
	}

	lastBlock := blocks[len(blocks)-1]
	cp.setNextHeight(lastBlock.GetHeight() + 1)
	cp.metrics.RecordLastPolledHeight(lastBlock.GetHeight())

	cp.logger.Debug("sent blocks to channel",
		zap.Uint64("start_height", blockToRetrieve),
		zap.Uint64("end_height", lastBlock.GetHeight()),
		zap.Int("block_count", len(blocks)))

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

// Retry helper methods remain the same
func (cp *ChainPoller) blocksWithRetry(ctx context.Context, start, end uint64, limit uint32) ([]types.BlockDescription, error) {
	var blocks []types.BlockDescription
	var err error

	retryErr := retry.Do(func() error {
		blocks, err = cp.consumerCon.QueryBlocks(ctx, ccapi.NewQueryBlocksRequest(start, end, limit))
		if err != nil {
			return err
		}

		if len(blocks) == 0 {
			cp.logger.Debug("no blocks found for range",
				zap.Uint64("start_height", start),
				zap.Uint64("end_height", end))
		}

		return nil
	}, RtyAtt, RtyDel, RtyErr,
		retry.OnRetry(func(n uint, err error) {
			cp.logger.Debug("retrying block query",
				zap.Uint("attempt", n+1),
				zap.Uint64("start_height", start),
				zap.Uint64("end_height", end),
				zap.Error(err))
		}))

	return blocks, retryErr
}

func (cp *ChainPoller) latestBlockHeightWithRetry(ctx context.Context) (uint64, error) {
	var latestBlock types.BlockDescription
	var err error

	retryErr := retry.Do(func() error {
		latestBlock, err = cp.consumerCon.QueryLatestBlock(ctx)

		return err
	}, RtyAtt, RtyDel, RtyErr,
		retry.OnRetry(func(n uint, err error) {
			cp.logger.Debug("retrying latest block height query",
				zap.Uint("attempt", n+1),
				zap.Error(err))
		}))

	return latestBlock.GetHeight(), retryErr
}

func (cp *ChainPoller) NextHeight() uint64 {
	return cp.getNextHeight()
}

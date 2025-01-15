package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/clientcontroller"
	cfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
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

type skipHeightRequest struct {
	height uint64
	resp   chan *skipHeightResponse
}

type skipHeightResponse struct {
	err error
}

type ChainPoller struct {
	isStarted *atomic.Bool
	wg        sync.WaitGroup
	quit      chan struct{}

	cc             clientcontroller.ClientController
	cfg            *cfg.ChainPollerConfig
	metrics        *metrics.FpMetrics
	blockInfoChan  chan *types.BlockInfo
	skipHeightChan chan *skipHeightRequest
	nextHeight     uint64
	logger         *zap.Logger
	mu             sync.RWMutex
}

func NewChainPoller(
	logger *zap.Logger,
	cfg *cfg.ChainPollerConfig,
	cc clientcontroller.ClientController,
	metrics *metrics.FpMetrics,
) *ChainPoller {
	return &ChainPoller{
		isStarted:      atomic.NewBool(false),
		logger:         logger,
		cfg:            cfg,
		cc:             cc,
		metrics:        metrics,
		blockInfoChan:  make(chan *types.BlockInfo, cfg.BufferSize),
		skipHeightChan: make(chan *skipHeightRequest),
		quit:           make(chan struct{}),
	}
}

func (cp *ChainPoller) Start(startHeight uint64) error {
	if cp.isStarted.Swap(true) {
		return fmt.Errorf("the poller is already started")
	}

	cp.logger.Info("starting the chain poller")

	cp.nextHeight = startHeight

	cp.wg.Add(1)

	go cp.pollChain()

	cp.metrics.RecordPollerStartingHeight(startHeight)
	cp.logger.Info("the chain poller is successfully started")

	return nil
}

func (cp *ChainPoller) Stop() error {
	if !cp.isStarted.Swap(false) {
		return fmt.Errorf("the chain poller has already stopped")
	}

	cp.logger.Info("stopping the chain poller")
	err := cp.cc.Close()
	if err != nil {
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

// GetBlockInfoChan returns the read-only channel for incoming blocks
func (cp *ChainPoller) GetBlockInfoChan() <-chan *types.BlockInfo {
	return cp.blockInfoChan
}

func (cp *ChainPoller) blocksWithRetry(start, end, limit uint64) ([]*types.BlockInfo, error) {
	var (
		block []*types.BlockInfo
		err   error
	)
	if err := retry.Do(func() error {
		block, err = cp.cc.QueryBlocks(start, end, limit)
		if err != nil {
			return err
		}

		if len(block) == 0 {
			return fmt.Errorf("no blocks found for range %d-%d", start, end)
		}

		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		cp.logger.Debug(
			"failed to query the consumer chain for block range",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Uint64("start_height", start),
			zap.Uint64("end_height", end),
			zap.Uint64("limit", limit),
			zap.Error(err),
		)
	})); err != nil {
		return nil, err
	}

	return block, nil
}

func (cp *ChainPoller) getLatestBlockWithRetry() (*types.BlockInfo, error) {
	var (
		latestBlock *types.BlockInfo
		err         error
	)

	if err := retry.Do(func() error {
		latestBlock, err = cp.cc.QueryBestBlock()
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
		return nil, err
	}

	return latestBlock, nil
}

// waitForActivation waits until BTC staking is activated
func (cp *ChainPoller) waitForActivation() {
	// ensure that the startHeight is no lower than the activated height
	for {
		activatedHeight, err := cp.cc.QueryActivatedHeight()
		if err != nil {
			cp.logger.Debug("failed to query the consumer chain for the activated height", zap.Error(err))
		} else {
			if cp.nextHeight < activatedHeight {
				cp.nextHeight = activatedHeight
			}

			return
		}
		select {
		case <-time.After(cp.cfg.PollInterval):
			continue
		case <-cp.quit:
			return
		}
	}
}

func (cp *ChainPoller) pollChain() {
	defer cp.wg.Done()

	cp.waitForActivation()

	var failedCycles uint32

	for {
		latestBlock, err := cp.getLatestBlockWithRetry()
		if err != nil {
			failedCycles++
			cp.logger.Debug(
				"failed to query the consumer chain for the latest block",
				zap.Uint32("current_failures", failedCycles),
				zap.Error(err),
			)
		} else {
			// start polling in the first iteration
			blockToRetrieve := cp.NextHeight()

			blocks, err := cp.blocksWithRetry(blockToRetrieve, latestBlock.Height, latestBlock.Height)
			if err != nil {
				failedCycles++
				cp.logger.Debug(
					"failed to query the consumer chain for the block range",
					zap.Uint32("current_failures", failedCycles),
					zap.Uint64("start_height", blockToRetrieve),
					zap.Uint64("end_height", latestBlock.Height),
					zap.Error(err),
				)
			} else {
				// no error and we got the header we wanted to get, bump the state and push
				// notification about data
				failedCycles = 0
				if len(blocks) == 0 {
					continue
				}

				lb := blocks[len(blocks)-1]
				cp.setNextHeight(lb.Height + 1)

				cp.metrics.RecordLastPolledHeight(lb.Height)

				cp.logger.Info("the poller retrieved the blocks from the consumer chain",
					zap.Uint64("start_height", blockToRetrieve),
					zap.Uint64("end_height", lb.Height))

				// push the data to the channel
				// Note: if the consumer is too slow -- the buffer is full
				// the channel will block, and we will stop retrieving data from the node
				for _, block := range blocks {
					cp.blockInfoChan <- block
				}
			}
		}

		if failedCycles > maxFailedCycles {
			cp.logger.Fatal("the poller has reached the max failed cycles, exiting")
		}
		select {
		case <-time.After(cp.cfg.PollInterval):
			continue
		case req := <-cp.skipHeightChan:
			// no need to skip heights if the target height is not higher
			// than the next height to retrieve
			targetHeight := req.height
			if targetHeight <= cp.nextHeight {
				resp := &skipHeightResponse{
					err: fmt.Errorf(
						"the target height %d is not higher than the next height %d to retrieve",
						targetHeight, cp.nextHeight)}
				req.resp <- resp

				continue
			}

			// drain blocks that can be skipped from blockInfoChan
			cp.clearChanBufferUpToHeight(targetHeight)

			// set the next height to the skip height
			cp.setNextHeight(targetHeight)

			cp.logger.Debug("the poller has skipped height(s)",
				zap.Uint64("next_height", req.height))

			req.resp <- &skipHeightResponse{}

		case <-cp.quit:
			return
		}
	}
}

func (cp *ChainPoller) SkipToHeight(height uint64) error {
	if !cp.IsRunning() {
		return fmt.Errorf("the chain poller is stopped")
	}

	respChan := make(chan *skipHeightResponse, 1)

	// this handles the case when the poller is stopped before the
	// skip height request is sent
	select {
	case <-cp.quit:
		return fmt.Errorf("the chain poller is stopped")
	case cp.skipHeightChan <- &skipHeightRequest{height: height, resp: respChan}:
	}

	// this handles the case when the poller is stopped before
	// the skip height request is returned
	select {
	case <-cp.quit:
		return fmt.Errorf("the chain poller is stopped")
	case resp := <-respChan:
		return resp.err
	}
}

func (cp *ChainPoller) NextHeight() uint64 {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	return cp.nextHeight
}

func (cp *ChainPoller) setNextHeight(height uint64) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	cp.nextHeight = height
}

func (cp *ChainPoller) clearChanBufferUpToHeight(upToHeight uint64) {
	for len(cp.blockInfoChan) > 0 {
		block := <-cp.blockInfoChan
		if block.Height+1 >= upToHeight {
			break
		}
	}
}

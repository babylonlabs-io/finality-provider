package service

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/types"
)

type FastSyncResult struct {
	Responses           []*types.TxResponse
	SyncedHeight        uint64
	LastProcessedHeight uint64
}

// FastSync attempts to send a batch of finality signatures
// from the maximum of the last voted height and the last finalized height
// to the current height
func (fp *FinalityProviderInstance) FastSync(startHeight, endHeight uint64) (*FastSyncResult, error) {
	if fp.inSync.Swap(true) {
		return nil, fmt.Errorf("the finality-provider has already been in fast sync")
	}
	defer fp.inSync.Store(false)

	if startHeight > endHeight {
		return nil, fmt.Errorf("the start height %v should not be higher than the end height %v",
			startHeight, endHeight)
	}

	var syncedHeight uint64
	responses := make([]*types.TxResponse, 0)
	// we may need several rounds to catch-up as we need to limit
	// the catch-up distance for each round to avoid memory overflow
	for startHeight <= endHeight {
		blocks, err := fp.cc.QueryBlocks(startHeight, endHeight, fp.cfg.FastSyncLimit)
		if err != nil {
			return nil, err
		}

		if len(blocks) < 1 {
			break
		}

		startHeight = blocks[len(blocks)-1].Height + 1

		// Note: not all the blocks in the range will have votes cast
		// due to lack of voting power or public randomness, so we may
		// have gaps during sync
		catchUpBlocks := make([]*types.BlockInfo, 0, len(blocks))
		for _, b := range blocks {
			// check whether the block has been processed before
			if fp.hasProcessed(b) {
				continue
			}
			// check whether the finality provider has voting power
			hasVp, err := fp.hasVotingPower(b)
			if err != nil {
				return nil, err
			}
			if !hasVp {
				fp.metrics.IncrementFpTotalBlocksWithoutVotingPower(fp.GetBtcPkHex())
				continue
			}
			// check whether the randomness has been committed
			hasRand, err := fp.hasRandomness(b)
			if err != nil {
				return nil, err
			}
			if !hasRand {
				break
			}
			// all good, add the block for catching up
			catchUpBlocks = append(catchUpBlocks, b)
		}

		if len(catchUpBlocks) < 1 {
			continue
		}

		syncedHeight = catchUpBlocks[len(catchUpBlocks)-1].Height

		res, err := fp.SubmitBatchFinalitySignatures(catchUpBlocks)
		if err != nil {
			return nil, err
		}
		fp.metrics.AddToFpTotalVotedBlocks(fp.GetBtcPkHex(), float64(len(catchUpBlocks)))

		responses = append(responses, res)

		fp.logger.Debug(
			"the finality-provider is catching up by sending finality signatures in a batch",
			zap.String("pk", fp.GetBtcPkHex()),
			zap.Uint64("start_height", catchUpBlocks[0].Height),
			zap.Uint64("synced_height", syncedHeight),
		)
	}

	// update the processed height
	fp.MustSetLastProcessedHeight(syncedHeight)

	return &FastSyncResult{
		Responses:           responses,
		SyncedHeight:        syncedHeight,
		LastProcessedHeight: fp.GetLastProcessedHeight(),
	}, nil
}

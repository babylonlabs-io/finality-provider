package service_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/babylonlabs-io/finality-provider/testutil/mocks"
	"github.com/babylonlabs-io/finality-provider/types"
)

// FuzzChainPoller_Start tests the poller polling blocks
// in sequence
func FuzzChainPoller_Start(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		currentHeight := uint64(r.Int63n(100) + 1)
		startHeight := currentHeight + 1
		endHeight := startHeight + uint64(r.Int63n(10)+1)

		currentBlockRes := &types.BlockInfo{
			Height: endHeight,
		}
		ctl := gomock.NewController(t)
		mockConsumerController := mocks.NewMockConsumerController(ctl)
		mockConsumerController.EXPECT().Close().Return(nil).AnyTimes()
		mockConsumerController.EXPECT().QueryActivatedHeight().Return(uint64(1), nil).AnyTimes()
		mockConsumerController.EXPECT().QueryLatestBlockHeight().Return(endHeight, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryBlock(endHeight).Return(currentBlockRes, nil).AnyTimes()
		pollerCfg := fpcfg.DefaultChainPollerConfig()

		for i := startHeight; i <= endHeight; i++ {
			resBlocks := []*types.BlockInfo{{
				Height: i,
			}}

			mockConsumerController.EXPECT().QueryBlocks(i, endHeight, pollerCfg.PollSize).Return(resBlocks, nil).AnyTimes()
		}

		m := metrics.NewFpMetrics()
		pollerCfg.PollInterval = 10 * time.Millisecond
		poller := service.NewChainPoller(testutil.GetTestLogger(t), &pollerCfg, mockConsumerController, m)
		err := poller.SetStartHeight(startHeight)
		require.NoError(t, err)
		defer func() {
			err := poller.Stop()
			require.NoError(t, err)
		}()

		for i := startHeight; i <= endHeight; i++ {
			info, err := poller.NextBlock(context.Background())
			require.NoError(t, err)
			require.Equal(t, i, info.Height)
		}
	})
}

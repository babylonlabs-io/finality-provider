package service_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/babylonlabs-io/finality-provider/testutil/mocks"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// FuzzChainPoller_Start tests the poller polling blocks
// in sequence
func FuzzChainPoller_Start(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		ctx := t.Context()

		currentHeight := uint64(r.Int63n(100) + 1)
		startHeight := currentHeight + 1
		endHeight := startHeight + uint64(r.Int63n(10)+1)

		currentBlockRes := types.NewBlockInfo(endHeight, nil, false)
		ctl := gomock.NewController(t)
		mockConsumerController := mocks.NewMockConsumerController(ctl)
		mockConsumerController.EXPECT().Close().Return(nil).AnyTimes()
		mockConsumerController.EXPECT().QueryLatestBlock(ctx).Return(currentBlockRes, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryFinalityActivationBlockHeight(ctx).Return(uint64(1), nil).AnyTimes()
		mockConsumerController.EXPECT().QueryBlock(ctx, endHeight).Return(currentBlockRes, nil).AnyTimes()
		pollerCfg := fpcfg.DefaultChainPollerConfig()

		for i := startHeight; i <= endHeight; i++ {
			resBlocks := []types.BlockDescription{
				types.NewBlockInfo(i, nil, false),
			}
			mockConsumerController.EXPECT().QueryBlocks(ctx, api.NewQueryBlocksRequest(i, endHeight, pollerCfg.PollSize)).Return(resBlocks, nil).AnyTimes()
		}

		m := metrics.NewFpMetrics()
		pollerCfg.PollInterval = 10 * time.Millisecond
		poller := service.NewChainPoller(testutil.GetTestLogger(t), &pollerCfg, mockConsumerController, m)
		err := poller.SetStartHeight(t.Context(), startHeight)
		require.NoError(t, err)
		defer func() {
			err := poller.Stop()
			require.NoError(t, err)
		}()

		for i := startHeight; i <= endHeight; i++ {
			info, errNxt := poller.NextBlock(ctx)
			require.NoError(t, errNxt)
			require.Equal(t, i, info.GetHeight())
		}
	})
}

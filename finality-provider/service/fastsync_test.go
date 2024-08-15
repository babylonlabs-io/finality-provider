package service_test

import (
	"math/rand"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/babylonlabs-io/finality-provider/types"
)

// FuzzFastSync_SufficientRandomness tests a case where we have sufficient
// randomness timestamped and voting power when the finality provider enters
// fast-sync it is expected that the finality provider could catch up to
// the current height through fast-sync
func FuzzFastSync_SufficientRandomnessTimestamped(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		randomStartingHeight := uint64(r.Int63n(100) + 1)
		finalizedHeight := randomStartingHeight + uint64(r.Int63n(10)+2)
		currentHeight := finalizedHeight + uint64(r.Int63n(10)+1)
		mockClientController := testutil.PrepareMockedClientController(t, r, randomStartingHeight, currentHeight)
		mockClientController.EXPECT().QueryLatestFinalizedBlocks(uint64(1)).Return(nil, nil).AnyTimes()
		_, fpIns, cleanUp := startFinalityProviderAppWithRegisteredFp(t, r, mockClientController, randomStartingHeight)
		defer cleanUp()

		// commit pub rand
		mockClientController.EXPECT().QueryLastCommittedPublicRand(gomock.Any(), uint64(1)).Return(nil, nil).Times(1)
		mockClientController.EXPECT().CommitPubRandList(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
		_, err := fpIns.CommitPubRand(randomStartingHeight)
		require.NoError(t, err)

		mockClientController.EXPECT().QueryFinalityProviderVotingPower(fpIns.GetBtcPk(), gomock.Any()).
			Return(uint64(1), nil).AnyTimes()
		// make sure all the public randomness is timestamped
		mockClientController.EXPECT().QueryIsPubRandTimestamped(gomock.Any()).Return(true, nil).AnyTimes()

		catchUpBlocks := testutil.GenBlocks(r, finalizedHeight+1, currentHeight)
		expectedTxHash := testutil.GenRandomHexStr(r, 32)
		finalizedBlock := &types.BlockInfo{Height: finalizedHeight, Hash: testutil.GenRandomByteArray(r, 32)}
		mockClientController.EXPECT().QueryLatestFinalizedBlocks(uint64(1)).Return([]*types.BlockInfo{finalizedBlock}, nil).AnyTimes()
		mockClientController.EXPECT().QueryBlocks(finalizedHeight+1, currentHeight, uint64(10)).
			Return(catchUpBlocks, nil)
		mockClientController.EXPECT().SubmitBatchFinalitySigs(fpIns.GetBtcPk(), catchUpBlocks, gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&types.TxResponse{TxHash: expectedTxHash}, nil).AnyTimes()
		result, err := fpIns.FastSync(finalizedHeight+1, currentHeight)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, expectedTxHash, result.Responses[0].TxHash)
		require.Equal(t, currentHeight, fpIns.GetLastVotedHeight())
		require.Equal(t, currentHeight, fpIns.GetLastProcessedHeight())
	})
}

// FuzzFastSync_NoTimestampedRandomness tests a case where we have sufficient
// randomness submitted and voting power when the finality provider enters fast-sync
// but the randomness is not BTC-timestamped
// it is expected that the finality provider cannot catch up
func FuzzFastSync_NoTimestampedRandomness(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		randomStartingHeight := uint64(r.Int63n(100) + 1)
		finalizedHeight := randomStartingHeight + uint64(r.Int63n(10)+2)
		currentHeight := finalizedHeight + uint64(r.Int63n(10)+1)
		mockClientController := testutil.PrepareMockedClientController(t, r, randomStartingHeight, currentHeight)
		mockClientController.EXPECT().QueryLatestFinalizedBlocks(uint64(1)).Return(nil, nil).AnyTimes()
		_, fpIns, cleanUp := startFinalityProviderAppWithRegisteredFp(t, r, mockClientController, randomStartingHeight)
		defer cleanUp()

		// commit pub rand
		mockClientController.EXPECT().QueryLastCommittedPublicRand(gomock.Any(), uint64(1)).Return(nil, nil).Times(1)
		mockClientController.EXPECT().CommitPubRandList(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
		_, err := fpIns.CommitPubRand(randomStartingHeight)
		require.NoError(t, err)

		mockClientController.EXPECT().QueryFinalityProviderVotingPower(fpIns.GetBtcPk(), gomock.Any()).
			Return(uint64(1), nil).AnyTimes()
		// make sure no randomness is timestamped
		mockClientController.EXPECT().QueryIsPubRandTimestamped(gomock.Any()).Return(false, nil).AnyTimes()

		catchUpBlocks := testutil.GenBlocks(r, finalizedHeight+1, currentHeight)
		expectedTxHash := testutil.GenRandomHexStr(r, 32)
		finalizedBlock := &types.BlockInfo{Height: finalizedHeight, Hash: testutil.GenRandomByteArray(r, 32)}
		mockClientController.EXPECT().QueryLatestFinalizedBlocks(uint64(1)).Return([]*types.BlockInfo{finalizedBlock}, nil).AnyTimes()
		mockClientController.EXPECT().QueryBlocks(finalizedHeight+1, currentHeight, uint64(10)).
			Return(catchUpBlocks, nil)
		mockClientController.EXPECT().SubmitBatchFinalitySigs(fpIns.GetBtcPk(), catchUpBlocks, gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&types.TxResponse{TxHash: expectedTxHash}, nil).AnyTimes()
		result, err := fpIns.FastSync(finalizedHeight+1, currentHeight)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, 0, len(result.Responses))
		require.Equal(t, uint64(0), fpIns.GetLastVotedHeight())
		require.Equal(t, uint64(0), fpIns.GetLastProcessedHeight())
	})
}

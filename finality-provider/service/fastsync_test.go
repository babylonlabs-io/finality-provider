package service_test

import (
	"math/rand"
	"testing"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	ftypes "github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/babylonlabs-io/finality-provider/types"
)

// FuzzFastSync_SufficientRandomness tests a case where we have sufficient
// randomness and voting power when the finality provider enters fast-sync
// it is expected that the finality provider could catch up to the current
// height through fast-sync
func FuzzFastSync_SufficientRandomness(f *testing.F) {
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
		// the last committed height is higher than the current height
		// to make sure the randomness is sufficient
		lastCommittedHeight := randomStartingHeight + testutil.TestPubRandNum
		lastCommittedPubRandMap := make(map[uint64]*ftypes.PubRandCommitResponse)
		lastCommittedPubRandMap[lastCommittedHeight] = &ftypes.PubRandCommitResponse{
			NumPubRand: 1000,
			Commitment: datagen.GenRandomByteArray(r, 32),
		}
		mockClientController.EXPECT().QueryLastCommittedPublicRand(gomock.Any(), uint64(1)).Return(lastCommittedPubRandMap, nil).AnyTimes()

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

// FuzzFastSync_NoRandomness tests a case where we have insufficient
// randomness but with voting power when the finality provider enters fast-sync
// it is expected that the finality provider could catch up to the last
// committed height
func FuzzFastSync_NoRandomness(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		randomStartingHeight := uint64(r.Int63n(100) + 100)
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
		// the last height with pub rand is a random value inside [finalizedHeight+1, currentHeight]
		lastHeightWithPubRand := uint64(rand.Intn(int(currentHeight)-int(finalizedHeight))) + finalizedHeight + 1
		lastCommittedPubRandMap := make(map[uint64]*ftypes.PubRandCommitResponse)
		lastCommittedPubRandMap[lastHeightWithPubRand-10] = &ftypes.PubRandCommitResponse{
			NumPubRand: 10 + 1,
			Commitment: datagen.GenRandomByteArray(r, 32),
		}
		mockClientController.EXPECT().QueryLastCommittedPublicRand(gomock.Any(), uint64(1)).Return(lastCommittedPubRandMap, nil).AnyTimes()

		catchUpBlocks := testutil.GenBlocks(r, finalizedHeight+1, currentHeight)
		expectedTxHash := testutil.GenRandomHexStr(r, 32)
		finalizedBlock := &types.BlockInfo{Height: finalizedHeight, Hash: testutil.GenRandomByteArray(r, 32)}
		mockClientController.EXPECT().QueryLatestFinalizedBlocks(uint64(1)).Return([]*types.BlockInfo{finalizedBlock}, nil).AnyTimes()
		mockClientController.EXPECT().QueryBlocks(finalizedHeight+1, currentHeight, uint64(10)).
			Return(catchUpBlocks, nil)
		mockClientController.EXPECT().SubmitBatchFinalitySigs(fpIns.GetBtcPk(), catchUpBlocks[:lastHeightWithPubRand-finalizedHeight], gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&types.TxResponse{TxHash: expectedTxHash}, nil).AnyTimes()
		result, err := fpIns.FastSync(finalizedHeight+1, currentHeight)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, expectedTxHash, result.Responses[0].TxHash)
		require.Equal(t, lastHeightWithPubRand, fpIns.GetLastVotedHeight())
		require.Equal(t, lastHeightWithPubRand, fpIns.GetLastProcessedHeight())
	})
}

package testutil

import (
	"math/rand"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/golang/mock/gomock"

	"github.com/babylonlabs-io/finality-provider/testutil/mocks"
	"github.com/babylonlabs-io/finality-provider/types"
)

const TestPubRandNum = 25

func ZeroCommissionRate() *sdkmath.LegacyDec {
	zeroCom := sdkmath.LegacyZeroDec()
	return &zeroCom
}

func PrepareMockedConsumerController(t *testing.T, r *rand.Rand, startHeight, currentHeight uint64) *mocks.MockConsumerController {
	return PrepareMockedConsumerControllerWithTxHash(t, r, startHeight, currentHeight, GenRandomHexStr(r, 32))
}

func PrepareMockedConsumerControllerWithTxHash(t *testing.T, r *rand.Rand, startHeight, currentHeight uint64, txHash string) *mocks.MockConsumerController {
	ctl := gomock.NewController(t)
	mockConsumerController := mocks.NewMockConsumerController(ctl)

	for i := startHeight; i <= currentHeight; i++ {
		resBlock := &types.BlockInfo{
			Height: i,
			Hash:   GenRandomByteArray(r, 32),
		}
		mockConsumerController.EXPECT().QueryBlock(i).Return(resBlock, nil).AnyTimes()
	}

	mockConsumerController.EXPECT().Close().Return(nil).AnyTimes()
	mockConsumerController.EXPECT().QueryLatestBlockHeight().Return(currentHeight, nil).AnyTimes()
	mockConsumerController.EXPECT().QueryActivatedHeight().Return(uint64(1), nil).AnyTimes()

	// can't return (nil, nil) or `randomnessCommitmentLoop` will fatal (logic added in #454)
	mockConsumerController.EXPECT().
		CommitPubRandList(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&types.TxResponse{TxHash: txHash}, nil).
		AnyTimes()
	return mockConsumerController
}

func PrepareMockedBabylonController(t *testing.T) *mocks.MockClientController {
	ctl := gomock.NewController(t)
	mockBabylonController := mocks.NewMockClientController(ctl)
	mockBabylonController.EXPECT().Close().Return(nil).AnyTimes()

	return mockBabylonController
}

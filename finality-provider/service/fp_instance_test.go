package service_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	ftypes "github.com/babylonlabs-io/babylon/x/finality/types"

	"github.com/babylonlabs-io/finality-provider/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/babylonlabs-io/finality-provider/types"
)

func FuzzCommitPubRandList(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		randomStartingHeight := uint64(r.Int63n(100) + 1)
		currentHeight := randomStartingHeight + uint64(r.Int63n(10)+2)
		startingBlock := &types.BlockInfo{Height: randomStartingHeight, Hash: testutil.GenRandomByteArray(r, 32)}
		mockClientController := testutil.PrepareMockedClientController(t, r, randomStartingHeight, currentHeight, 0)
		mockClientController.EXPECT().QueryFinalityProviderVotingPower(gomock.Any(), gomock.Any()).
			Return(uint64(0), nil).AnyTimes()
		_, fpIns, cleanUp := startFinalityProviderAppWithRegisteredFp(t, r, mockClientController, randomStartingHeight)
		defer cleanUp()

		expectedTxHash := testutil.GenRandomHexStr(r, 32)
		mockClientController.EXPECT().
			CommitPubRandList(fpIns.GetBtcPk(), startingBlock.Height+1, gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&types.TxResponse{TxHash: expectedTxHash}, nil).AnyTimes()
		mockClientController.EXPECT().QueryLastCommittedPublicRand(gomock.Any(), uint64(1)).Return(nil, nil).AnyTimes()
		res, err := fpIns.CommitPubRand(startingBlock.Height)
		require.NoError(t, err)
		require.Equal(t, expectedTxHash, res.TxHash)
	})
}

func FuzzSubmitFinalitySigs(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		randomStartingHeight := uint64(r.Int63n(100) + 1)
		currentHeight := randomStartingHeight + uint64(r.Int63n(10)+1)
		startingBlock := &types.BlockInfo{Height: randomStartingHeight, Hash: testutil.GenRandomByteArray(r, 32)}
		mockClientController := testutil.PrepareMockedClientController(t, r, randomStartingHeight, currentHeight, 0)
		mockClientController.EXPECT().QueryLatestFinalizedBlocks(gomock.Any()).Return(nil, nil).AnyTimes()
		_, fpIns, cleanUp := startFinalityProviderAppWithRegisteredFp(t, r, mockClientController, randomStartingHeight)
		defer cleanUp()

		// commit pub rand
		mockClientController.EXPECT().QueryLastCommittedPublicRand(gomock.Any(), uint64(1)).Return(nil, nil).Times(1)
		mockClientController.EXPECT().CommitPubRandList(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
		_, err := fpIns.CommitPubRand(startingBlock.Height)
		require.NoError(t, err)

		// mock committed pub rand
		lastCommittedHeight := randomStartingHeight + 25
		lastCommittedPubRandMap := make(map[uint64]*ftypes.PubRandCommitResponse)
		lastCommittedPubRandMap[lastCommittedHeight] = &ftypes.PubRandCommitResponse{
			NumPubRand: 1000,
			Commitment: datagen.GenRandomByteArray(r, 32),
		}
		mockClientController.EXPECT().QueryLastCommittedPublicRand(gomock.Any(), uint64(1)).Return(lastCommittedPubRandMap, nil).AnyTimes()
		// mock voting power and commit pub rand
		mockClientController.EXPECT().QueryFinalityProviderVotingPower(fpIns.GetBtcPk(), gomock.Any()).
			Return(uint64(1), nil).AnyTimes()

		// submit finality sig
		nextBlock := &types.BlockInfo{
			Height: startingBlock.Height + 1,
			Hash:   testutil.GenRandomByteArray(r, 32),
		}
		expectedTxHash := testutil.GenRandomHexStr(r, 32)
		mockClientController.EXPECT().
			SubmitBatchFinalitySigs(fpIns.GetBtcPk(), []*types.BlockInfo{nextBlock}, gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&types.TxResponse{TxHash: expectedTxHash}, nil).AnyTimes()
		providerRes, err := fpIns.SubmitBatchFinalitySignatures([]*types.BlockInfo{nextBlock})
		require.NoError(t, err)
		require.Equal(t, expectedTxHash, providerRes.TxHash)

		// check the last_voted_height
		require.Equal(t, nextBlock.Height, fpIns.GetLastVotedHeight())
	})
}

func startFinalityProviderAppWithRegisteredFp(t *testing.T, r *rand.Rand, cc clientcontroller.ClientController, startingHeight uint64) (*service.FinalityProviderApp, *service.FinalityProviderInstance, func()) {
	logger := zap.NewNop()
	// create an EOTS manager
	eotsHomeDir := filepath.Join(t.TempDir(), "eots-home")
	eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
	eotsdb, err := eotsCfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)
	em, err := eotsmanager.NewLocalEOTSManager(eotsHomeDir, eotsCfg.KeyringBackend, eotsdb, logger)
	require.NoError(t, err)

	// create finality-provider app with randomized config
	fpHomeDir := filepath.Join(t.TempDir(), "fp-home")
	fpCfg := config.DefaultConfigWithHome(fpHomeDir)
	fpCfg.NumPubRand = testutil.TestPubRandNum
	fpCfg.PollerConfig.AutoChainScanningMode = false
	fpCfg.PollerConfig.StaticChainScanningStartHeight = startingHeight
	db, err := fpCfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)
	app, err := service.NewFinalityProviderApp(&fpCfg, cc, em, db, logger)
	require.NoError(t, err)
	err = app.Start()
	require.NoError(t, err)

	// create registered finality-provider
	fp := testutil.GenStoredFinalityProvider(r, t, app, passphrase, hdPath, nil)
	pubRandProofStore := app.GetPubRandProofStore()
	fpStore := app.GetFinalityProviderStore()
	err = fpStore.SetFpStatus(fp.BtcPk, proto.FinalityProviderStatus_REGISTERED)
	require.NoError(t, err)
	// TODO: use mock metrics
	m := metrics.NewFpMetrics()
	fpIns, err := service.NewFinalityProviderInstance(fp.GetBIP340BTCPK(), &fpCfg, fpStore, pubRandProofStore, cc, em, m, passphrase, make(chan *service.CriticalError), logger)
	require.NoError(t, err)

	cleanUp := func() {
		err = app.Stop()
		require.NoError(t, err)
		err = eotsdb.Close()
		require.NoError(t, err)
		err = db.Close()
		require.NoError(t, err)
		err = os.RemoveAll(eotsHomeDir)
		require.NoError(t, err)
		err = os.RemoveAll(fpHomeDir)
		require.NoError(t, err)
	}

	return app, fpIns, cleanUp
}

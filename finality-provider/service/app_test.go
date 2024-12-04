package service_test

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	finalitytypes "github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	fpstore "github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/keyring"
	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/babylonlabs-io/finality-provider/testutil/mocks"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/babylonlabs-io/finality-provider/util"
)

const (
	passphrase            = "testpass"
	hdPath                = ""
	eventuallyWaitTimeOut = 5 * time.Second
	eventuallyPollTime    = 10 * time.Millisecond
)

func FuzzRegisterFinalityProvider(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		logger := zap.NewNop()
		// create an EOTS manager
		eotsHomeDir := filepath.Join(t.TempDir(), "eots-home")
		eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
		dbBackend, err := eotsCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)
		em, err := eotsmanager.NewLocalEOTSManager(eotsHomeDir, eotsCfg.KeyringBackend, dbBackend, logger)
		require.NoError(t, err)
		defer func() {
			dbBackend.Close()
			err = os.RemoveAll(eotsHomeDir)
			require.NoError(t, err)
		}()

		// Create mocked babylon client
		randomStartingHeight := uint64(r.Int63n(100) + 1)
		currentHeight := randomStartingHeight + uint64(r.Int63n(10)+2)
		mockClientController := testutil.PrepareMockedClientController(t, r, randomStartingHeight, currentHeight, 0)
		mockClientController.EXPECT().QueryLatestFinalizedBlocks(gomock.Any()).Return(nil, nil).AnyTimes()
		mockClientController.EXPECT().QueryFinalityProviderVotingPower(gomock.Any(),
			gomock.Any()).Return(uint64(0), nil).AnyTimes()

		// Create randomized config
		fpHomeDir := filepath.Join(t.TempDir(), "fp-home")
		fpCfg := config.DefaultConfigWithHome(fpHomeDir)
		fpCfg.PollerConfig.AutoChainScanningMode = false
		fpCfg.PollerConfig.StaticChainScanningStartHeight = randomStartingHeight
		fpdb, err := fpCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)
		app, err := service.NewFinalityProviderApp(&fpCfg, mockClientController, em, fpdb, logger)
		require.NoError(t, err)
		defer func() {
			err = fpdb.Close()
			require.NoError(t, err)
			err = os.RemoveAll(fpHomeDir)
			require.NoError(t, err)
		}()

		err = app.Start()
		require.NoError(t, err)
		defer func() {
			err = app.Stop()
			require.NoError(t, err)
		}()

		var eotsPk *bbntypes.BIP340PubKey
		eotsKeyName := testutil.GenRandomHexStr(r, 4)
		require.NoError(t, err)
		eotsPkBz, err := em.CreateKey(eotsKeyName, passphrase, hdPath)
		require.NoError(t, err)
		eotsPk, err = bbntypes.NewBIP340PubKey(eotsPkBz)
		require.NoError(t, err)

		// create a finality-provider object and save it to db
		fp := testutil.GenStoredFinalityProvider(r, t, app, passphrase, hdPath, eotsPk)
		require.Equal(t, eotsPk, bbntypes.NewBIP340PubKeyFromBTCPK(fp.BtcPk))

		btcSig := new(bbntypes.BIP340Signature)
		err = btcSig.Unmarshal(fp.Pop.BtcSig)
		require.NoError(t, err)
		pop := &bstypes.ProofOfPossessionBTC{
			BtcSig:     btcSig.MustMarshal(),
			BtcSigType: bstypes.BTCSigType_BIP340,
		}
		popBytes, err := pop.Marshal()
		require.NoError(t, err)
		fpInfo, err := app.GetFinalityProviderInfo(fp.GetBIP340BTCPK())
		require.NoError(t, err)
		require.Equal(t, proto.FinalityProviderStatus_name[0], fpInfo.Status)
		require.Equal(t, false, fpInfo.IsRunning)
		fpListInfo, err := app.ListAllFinalityProvidersInfo()
		require.NoError(t, err)
		require.Equal(t, fpInfo.BtcPkHex, fpListInfo[0].BtcPkHex)

		txHash := testutil.GenRandomHexStr(r, 32)
		mockClientController.EXPECT().
			RegisterFinalityProvider(
				fp.BtcPk,
				popBytes,
				testutil.ZeroCommissionRate(),
				gomock.Any(),
			).Return(&types.TxResponse{TxHash: txHash}, nil).AnyTimes()

		res, err := app.RegisterFinalityProvider(fp.GetBIP340BTCPK().MarshalHex())
		require.NoError(t, err)
		require.Equal(t, txHash, res.TxHash)

		mockClientController.EXPECT().QueryLastCommittedPublicRand(gomock.Any(), uint64(1)).Return(nil, nil).AnyTimes()
		err = app.StartFinalityProvider(fp.GetBIP340BTCPK(), passphrase)
		require.NoError(t, err)

		fpAfterReg, err := app.GetFinalityProviderInstance()
		require.NoError(t, err)
		require.Equal(t, proto.FinalityProviderStatus_REGISTERED, fpAfterReg.GetStoreFinalityProvider().Status)

		fpInfo, err = app.GetFinalityProviderInfo(fp.GetBIP340BTCPK())
		require.NoError(t, err)
		require.Equal(t, proto.FinalityProviderStatus_name[1], fpInfo.Status)
		require.Equal(t, true, fpInfo.IsRunning)
	})
}

func FuzzSyncFinalityProviderStatus(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 14)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		logger := zap.NewNop()

		pathSuffix := datagen.GenRandomHexStr(r, 10)
		// create an EOTS manager
		eotsHomeDir := filepath.Join(t.TempDir(), "eots-home", pathSuffix)
		eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
		dbBackend, err := eotsCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)
		em, err := eotsmanager.NewLocalEOTSManager(eotsHomeDir, eotsCfg.KeyringBackend, dbBackend, logger)
		require.NoError(t, err)

		// Create randomized config
		fpHomeDir := filepath.Join(t.TempDir(), "fp-home", pathSuffix)
		fpCfg := config.DefaultConfigWithHome(fpHomeDir)
		fpCfg.SyncFpStatusInterval = time.Millisecond * 100
		// no need for other intervals to run
		fpCfg.StatusUpdateInterval = time.Minute * 10
		fpCfg.SubmissionRetryInterval = time.Minute * 10
		fpdb, err := fpCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)

		randomStartingHeight := uint64(r.Int63n(100) + 1)
		currentHeight := randomStartingHeight + uint64(r.Int63n(10)+2)
		mockClientController := testutil.PrepareMockedClientController(t, r, randomStartingHeight, currentHeight, 0)

		blkInfo := &types.BlockInfo{Height: currentHeight}

		mockClientController.EXPECT().QueryLastCommittedPublicRand(gomock.Any(), uint64(1)).Return(nil, nil).AnyTimes()
		mockClientController.EXPECT().QueryLatestFinalizedBlocks(gomock.Any()).Return(nil, nil).AnyTimes()
		mockClientController.EXPECT().QueryBestBlock().Return(blkInfo, nil).Return(blkInfo, nil).AnyTimes()
		mockClientController.EXPECT().QueryBlock(gomock.Any()).Return(nil, errors.New("chain not online")).AnyTimes()

		noVotingPowerTable := r.Int31n(10) > 5
		if noVotingPowerTable {
			allowedErr := fmt.Sprintf("failed to query Finality Voting Power at Height %d: rpc error: code = Unknown desc = %s: unknown request",
				currentHeight, finalitytypes.ErrVotingPowerTableNotUpdated.Wrapf("height: %d", currentHeight).Error())
			mockClientController.EXPECT().QueryFinalityProviderVotingPower(gomock.Any(), gomock.Any()).Return(uint64(0), nil).AnyTimes()
			mockClientController.EXPECT().QueryActivatedHeight().Return(uint64(0), errors.New(allowedErr)).AnyTimes()
		} else {
			mockClientController.EXPECT().QueryActivatedHeight().Return(currentHeight, nil).AnyTimes()
			mockClientController.EXPECT().QueryFinalityProviderVotingPower(gomock.Any(), gomock.Any()).Return(uint64(2), nil).AnyTimes()
		}
		mockClientController.EXPECT().QueryFinalityProviderHighestVotedHeight(gomock.Any()).Return(uint64(0), nil).AnyTimes()

		app, err := service.NewFinalityProviderApp(&fpCfg, mockClientController, em, fpdb, logger)
		require.NoError(t, err)

		err = app.Start()
		require.NoError(t, err)

		eotsKeyName := testutil.GenRandomHexStr(r, 4)
		require.NoError(t, err)
		eotsPkBz, err := em.CreateKey(eotsKeyName, passphrase, hdPath)
		require.NoError(t, err)
		eotsPk, err := bbntypes.NewBIP340PubKey(eotsPkBz)
		require.NoError(t, err)
		fp := testutil.GenStoredFinalityProvider(r, t, app, "", hdPath, eotsPk)

		require.Eventually(t, func() bool {
			fpPk := fp.GetBIP340BTCPK()
			fpInfo, err := app.GetFinalityProviderInfo(fpPk)
			if err != nil {
				return false
			}

			expectedStatus := proto.FinalityProviderStatus_ACTIVE
			if noVotingPowerTable {
				expectedStatus = proto.FinalityProviderStatus_REGISTERED
			}
			fpInstance, err := app.GetFinalityProviderInstance()
			if err != nil {
				return false
			}

			// TODO: verify why mocks are failing
			btcPkEqual := fpInstance.GetBtcPk().IsEqual(fp.BtcPk)
			statusEqual := strings.EqualFold(fpInfo.Status, expectedStatus.String())
			return statusEqual && btcPkEqual
		}, time.Second*5, time.Millisecond*200, "should eventually be registered or active")
	})
}

func FuzzUnjailFinalityProvider(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		logger := zap.NewNop()

		pathSuffix := datagen.GenRandomHexStr(r, 10)
		// create an EOTS manager
		eotsHomeDir := filepath.Join(t.TempDir(), "eots-home", pathSuffix)
		eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
		dbBackend, err := eotsCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)
		em, err := eotsmanager.NewLocalEOTSManager(eotsHomeDir, eotsCfg.KeyringBackend, dbBackend, logger)
		require.NoError(t, err)

		// Create randomized config
		fpHomeDir := filepath.Join(t.TempDir(), "fp-home", pathSuffix)
		fpCfg := config.DefaultConfigWithHome(fpHomeDir)
		// use shorter interval for the test to end faster
		fpCfg.SyncFpStatusInterval = time.Millisecond * 10
		fpCfg.StatusUpdateInterval = time.Millisecond * 10
		fpCfg.SubmissionRetryInterval = time.Millisecond * 10
		fpdb, err := fpCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)

		randomStartingHeight := uint64(r.Int63n(100) + 1)
		currentHeight := randomStartingHeight + uint64(r.Int63n(10)+2)
		mockClientController := testutil.PrepareMockedClientController(t, r, randomStartingHeight, currentHeight, 0)

		blkInfo := &types.BlockInfo{Height: currentHeight}

		mockClientController.EXPECT().QueryLastCommittedPublicRand(gomock.Any(), uint64(1)).Return(nil, nil).AnyTimes()
		mockClientController.EXPECT().QueryLatestFinalizedBlocks(gomock.Any()).Return(nil, nil).AnyTimes()
		mockClientController.EXPECT().QueryBestBlock().Return(blkInfo, nil).Return(blkInfo, nil).AnyTimes()
		mockClientController.EXPECT().QueryBlock(gomock.Any()).Return(nil, errors.New("chain not online")).AnyTimes()

		// set voting power to be positive so that the fp should eventually become ACTIVE
		mockClientController.EXPECT().QueryFinalityProviderVotingPower(gomock.Any(), gomock.Any()).Return(uint64(0), nil).AnyTimes()
		mockClientController.EXPECT().QueryActivatedHeight().Return(uint64(1), nil).AnyTimes()
		mockClientController.EXPECT().QueryFinalityProviderSlashedOrJailed(gomock.Any()).Return(false, false, nil).AnyTimes()
		mockClientController.EXPECT().QueryFinalityProviderHighestVotedHeight(gomock.Any()).Return(uint64(0), nil).AnyTimes()

		app, err := service.NewFinalityProviderApp(&fpCfg, mockClientController, em, fpdb, logger)
		require.NoError(t, err)

		err = app.Start()
		defer func() {
			err := app.Stop()
			require.NoError(t, err)
		}()
		require.NoError(t, err)

		eotsKeyName := testutil.GenRandomHexStr(r, 4)
		require.NoError(t, err)
		eotsPkBz, err := em.CreateKey(eotsKeyName, passphrase, hdPath)
		require.NoError(t, err)
		eotsPk, err := bbntypes.NewBIP340PubKey(eotsPkBz)
		require.NoError(t, err)
		fp := testutil.GenStoredFinalityProvider(r, t, app, "", hdPath, eotsPk)
		err = app.GetFinalityProviderStore().SetFpStatus(fp.BtcPk, proto.FinalityProviderStatus_JAILED)
		require.NoError(t, err)

		expectedTxHash := datagen.GenRandomHexStr(r, 32)
		mockClientController.EXPECT().UnjailFinalityProvider(fp.BtcPk).Return(&types.TxResponse{TxHash: expectedTxHash}, nil)
		txHash, err := app.UnjailFinalityProvider(fp.GetBIP340BTCPK())
		require.NoError(t, err)
		require.Equal(t, expectedTxHash, txHash)
		fpInfo, err := app.GetFinalityProviderInfo(fp.GetBIP340BTCPK())
		require.NoError(t, err)
		require.Equal(t, proto.FinalityProviderStatus_INACTIVE.String(), fpInfo.GetStatus())
	})
}

func FuzzStatusUpdate(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		ctl := gomock.NewController(t)
		mockClientController := mocks.NewMockClientController(ctl)
		fpApp, fpPk, cleanUp := newFinalityProviderManagerWithRegisteredFp(t, r, mockClientController)
		defer cleanUp()

		// setup mocks
		currentHeight := uint64(r.Int63n(100) + 1)
		currentBlockRes := &types.BlockInfo{
			Height: currentHeight,
			Hash:   datagen.GenRandomByteArray(r, 32),
		}
		mockClientController.EXPECT().QueryBestBlock().Return(currentBlockRes, nil).AnyTimes()
		mockClientController.EXPECT().Close().Return(nil).AnyTimes()
		mockClientController.EXPECT().QueryLatestFinalizedBlocks(gomock.Any()).Return(nil, nil).AnyTimes()
		mockClientController.EXPECT().QueryBestBlock().Return(currentBlockRes, nil).AnyTimes()
		mockClientController.EXPECT().QueryActivatedHeight().Return(uint64(1), nil).AnyTimes()
		mockClientController.EXPECT().QueryFinalityActivationBlockHeight().Return(uint64(0), nil).AnyTimes()
		mockClientController.EXPECT().QueryFinalityProviderHighestVotedHeight(gomock.Any()).Return(uint64(0), nil).AnyTimes()
		mockClientController.EXPECT().QueryBlock(gomock.Any()).Return(currentBlockRes, nil).AnyTimes()
		mockClientController.EXPECT().QueryLastCommittedPublicRand(gomock.Any(), uint64(1)).Return(nil, nil).AnyTimes()

		votingPower := uint64(r.Intn(2))
		mockClientController.EXPECT().QueryFinalityProviderVotingPower(gomock.Any(), currentHeight).Return(votingPower, nil).AnyTimes()
		mockClientController.EXPECT().SubmitFinalitySig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&types.TxResponse{TxHash: ""}, nil).AnyTimes()
		var isSlashedOrJailed int
		if votingPower == 0 {
			// 0 means is slashed, 1 means is jailed, 2 means neither slashed nor jailed
			isSlashedOrJailed = r.Intn(3)
			switch isSlashedOrJailed {
			case 0:
				mockClientController.EXPECT().QueryFinalityProviderSlashedOrJailed(gomock.Any()).Return(true, false, nil).AnyTimes()
			case 1:
				mockClientController.EXPECT().QueryFinalityProviderSlashedOrJailed(gomock.Any()).Return(false, true, nil).AnyTimes()
			case 2:
				mockClientController.EXPECT().QueryFinalityProviderSlashedOrJailed(gomock.Any()).Return(false, false, nil).AnyTimes()
			}
		}

		err := fpApp.StartFinalityProvider(fpPk, passphrase)
		require.NoError(t, err)
		fpIns, err := fpApp.GetFinalityProviderInstance()
		require.NoError(t, err)
		// stop the finality-provider as we are testing static functionalities
		err = fpIns.Stop()
		require.NoError(t, err)

		if votingPower > 0 {
			waitForStatus(t, fpIns, proto.FinalityProviderStatus_ACTIVE)
		} else {
			switch {
			case isSlashedOrJailed == 2 && fpIns.GetStatus() == proto.FinalityProviderStatus_ACTIVE:
				waitForStatus(t, fpIns, proto.FinalityProviderStatus_INACTIVE)
			case isSlashedOrJailed == 1:
				waitForStatus(t, fpIns, proto.FinalityProviderStatus_JAILED)
			case isSlashedOrJailed == 0:
				waitForStatus(t, fpIns, proto.FinalityProviderStatus_SLASHED)
			}
		}
	})
}

func waitForStatus(t *testing.T, fpIns *service.FinalityProviderInstance, s proto.FinalityProviderStatus) {
	require.Eventually(t,
		func() bool {
			return fpIns.GetStatus() == s
		}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func newFinalityProviderManagerWithRegisteredFp(t *testing.T, r *rand.Rand, cc clientcontroller.ClientController) (*service.FinalityProviderApp, *bbntypes.BIP340PubKey, func()) {
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
	fpCfg.StatusUpdateInterval = 10 * time.Millisecond
	input := strings.NewReader("")
	kr, err := keyring.CreateKeyring(
		fpCfg.BabylonConfig.KeyDirectory,
		fpCfg.BabylonConfig.ChainID,
		fpCfg.BabylonConfig.KeyringBackend,
		input,
	)
	require.NoError(t, err)
	err = util.MakeDirectory(config.DataDir(fpHomeDir))
	require.NoError(t, err)
	db, err := fpCfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)
	fpStore, err := fpstore.NewFinalityProviderStore(db)
	require.NoError(t, err)
	app, err := service.NewFinalityProviderApp(&fpCfg, cc, em, db, logger)
	require.NoError(t, err)
	err = app.Start()
	require.NoError(t, err)

	// create registered finality-provider
	keyName := datagen.GenRandomHexStr(r, 10)
	chainID := datagen.GenRandomHexStr(r, 10)
	kc, err := keyring.NewChainKeyringControllerWithKeyring(kr, keyName, input)
	require.NoError(t, err)
	btcPkBytes, err := em.CreateKey(keyName, passphrase, hdPath)
	require.NoError(t, err)
	btcPk, err := bbntypes.NewBIP340PubKey(btcPkBytes)
	require.NoError(t, err)
	keyInfo, err := kc.CreateChainKey(passphrase, hdPath, "")
	require.NoError(t, err)
	fpAddr := keyInfo.AccAddress
	fpRecord, err := em.KeyRecord(btcPk.MustMarshal(), passphrase)
	require.NoError(t, err)
	pop, err := kc.CreatePop(fpAddr, fpRecord.PrivKey)
	require.NoError(t, err)

	err = fpStore.CreateFinalityProvider(
		fpAddr,
		btcPk.MustToBTCPK(),
		testutil.RandomDescription(r),
		testutil.ZeroCommissionRate(),
		keyName,
		chainID,
		pop.BtcSig,
	)
	require.NoError(t, err)
	err = fpStore.SetFpStatus(btcPk.MustToBTCPK(), proto.FinalityProviderStatus_REGISTERED)
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

	return app, btcPk, cleanUp
}

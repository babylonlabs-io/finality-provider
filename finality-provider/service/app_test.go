package service_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	finalitytypes "github.com/babylonlabs-io/babylon/x/finality/types"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	fpstore "github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/keyring"
	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/babylonlabs-io/finality-provider/util"
)

const (
	passphrase            = "testpass"
	hdPath                = ""
	eventuallyWaitTimeOut = 5 * time.Second
	eventuallyPollTime    = 10 * time.Millisecond
)

func FuzzCreateFinalityProvider(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		logger := testutil.GetTestLogger(t)
		// create an EOTS manager
		eotsHomeDir := filepath.Join(t.TempDir(), "eots-home")
		eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
		dbBackend, err := eotsCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)
		useFileKeyring := rand.Intn(2) == 1
		var passphrase string

		if useFileKeyring {
			eotsCfg.KeyringBackend = sdkkeyring.BackendFile
			passphrase = testutil.GenRandomHexStr(r, 8)
		}

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
		mockConsumerController := testutil.PrepareMockedConsumerController(t, r, randomStartingHeight, currentHeight)
		mockConsumerController.EXPECT().QueryLatestFinalizedBlock(gomock.Any()).Return(nil, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryFinalityProviderHasPower(gomock.Any(),
			gomock.Any()).Return(false, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryBlocks(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryLastPublicRandCommit(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockBabylonController := testutil.PrepareMockedBabylonController(t)

		// Create randomized config
		fpHomeDir := filepath.Join(t.TempDir(), "fp-home")
		fpCfg := config.DefaultConfigWithHome(fpHomeDir)
		fpCfg.NumPubRand = testutil.TestPubRandNum
		fpCfg.PollerConfig.AutoChainScanningMode = false
		fpCfg.PollerConfig.StaticChainScanningStartHeight = randomStartingHeight
		fpdb, err := fpCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)
		fpMetrics := metrics.NewFpMetrics()
		poller := service.NewChainPoller(logger, fpCfg.PollerConfig, mockConsumerController, fpMetrics)
		app, err := service.NewFinalityProviderApp(&fpCfg, mockBabylonController, mockConsumerController, em, poller, fpMetrics, fpdb, logger)
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
		eotsPkBz, err := em.CreateKey(eotsKeyName, passphrase)
		require.NoError(t, err)
		if useFileKeyring {
			err = em.Unlock(eotsPkBz, passphrase)
			require.NoError(t, err)
		}
		eotsPk, err = bbntypes.NewBIP340PubKey(eotsPkBz)
		require.NoError(t, err)

		// generate keyring
		keyName := testutil.GenRandomHexStr(r, 4)
		chainID := testutil.GenRandomHexStr(r, 4)

		cfg := app.GetConfig()
		_, err = testutil.CreateChainKey(cfg.BabylonConfig.KeyDirectory, cfg.BabylonConfig.ChainID, keyName, sdkkeyring.BackendTest, passphrase, hdPath, "")
		require.NoError(t, err)

		txHash := testutil.GenRandomHexStr(r, 32)
		mockBabylonController.EXPECT().
			RegisterFinalityProvider(
				chainID,
				eotsPk.MustToBTCPK(),
				gomock.Any(),
				testutil.ZeroCommissionRate(),
				gomock.Any(),
			).Return(&types.TxResponse{TxHash: txHash}, nil).AnyTimes()
		mockBabylonController.EXPECT().QueryFinalityProvider(gomock.Any()).Return(nil, nil).AnyTimes()
		res, err := app.CreateFinalityProvider(context.Background(), keyName, chainID, eotsPk, testutil.RandomDescription(r), testutil.ZeroCommissionRate())
		require.NoError(t, err)
		require.Equal(t, txHash, res.TxHash)

		fpInfo, err := app.GetFinalityProviderInfo(eotsPk)
		require.NoError(t, err)
		require.Equal(t, eotsPk.MarshalHex(), fpInfo.BtcPkHex)
	})
}

func FuzzSyncFinalityProviderStatus(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		mockBabylonController := testutil.PrepareMockedBabylonController(t)
		randomStartingHeight := uint64(r.Int63n(100) + 1)
		currentHeight := randomStartingHeight + uint64(r.Int63n(10)+2)
		mockConsumerController := testutil.PrepareMockedConsumerController(t, r, randomStartingHeight, currentHeight)

		mockConsumerController.EXPECT().QueryLastPublicRandCommit(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryLatestFinalizedBlock(gomock.Any()).Return(nil, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryLatestBlockHeight(gomock.Any()).Return(currentHeight, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryBlock(gomock.Any(), gomock.Any()).Return(nil, errors.New("chain not online")).AnyTimes()

		noVotingPowerTable := r.Int31n(10) > 5
		if noVotingPowerTable {
			allowedErr := fmt.Sprintf("failed to query Finality Voting Power at Height %d: rpc error: code = Unknown desc = %s: unknown request",
				currentHeight, finalitytypes.ErrVotingPowerTableNotUpdated.Wrapf("height: %d", currentHeight).Error())
			mockConsumerController.EXPECT().QueryFinalityProviderHasPower(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
			mockConsumerController.EXPECT().QueryActivatedHeight(gomock.Any()).Return(uint64(0), errors.New(allowedErr)).AnyTimes()
		} else {
			mockConsumerController.EXPECT().QueryFinalityProviderHasPower(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
			mockConsumerController.EXPECT().QueryActivatedHeight(gomock.Any()).Return(currentHeight, nil).AnyTimes()
		}
		mockConsumerController.EXPECT().QueryFinalityProviderHighestVotedHeight(gomock.Any(), gomock.Any()).Return(uint64(0), nil).AnyTimes()
		var isSlashedOrJailed int
		if noVotingPowerTable {
			// 0 means is slashed, 1 means is jailed, 2 means neither slashed nor jailed
			isSlashedOrJailed = r.Intn(3)
			switch isSlashedOrJailed {
			case 0:
				mockConsumerController.EXPECT().QueryFinalityProviderStatus(gomock.Any(), gomock.Any()).Return(&api.FinalityProviderStatusResponse{
					Slashed: true,
					Jailed:  false,
				}, nil).AnyTimes()
			case 1:
				mockConsumerController.EXPECT().QueryFinalityProviderStatus(gomock.Any(), gomock.Any()).Return(&api.FinalityProviderStatusResponse{
					Slashed: false,
					Jailed:  true,
				}, nil).AnyTimes()
			case 2:
				mockConsumerController.EXPECT().QueryFinalityProviderStatus(gomock.Any(), gomock.Any()).Return(&api.FinalityProviderStatusResponse{
					Slashed: false,
					Jailed:  false,
				}, nil).AnyTimes()
			}
		}

		// Create randomized config
		pathSuffix := datagen.GenRandomHexStr(r, 10)
		fpHomeDir := filepath.Join(t.TempDir(), "fp-home", pathSuffix)
		fpCfg := config.DefaultConfigWithHome(fpHomeDir)
		// no need for other intervals to run
		fpCfg.SubmissionRetryInterval = time.Minute * 10

		// Create fp app
		app, fpPk, cleanup := startFPAppWithRegisteredFp(t, r, fpHomeDir, &fpCfg, mockBabylonController, mockConsumerController)
		defer cleanup()

		fpInfo, err := app.GetFinalityProviderInfo(fpPk)
		require.NoError(t, err)

		expectedStatus := proto.FinalityProviderStatus_ACTIVE
		if noVotingPowerTable {
			switch isSlashedOrJailed {
			case 0:
				expectedStatus = proto.FinalityProviderStatus_SLASHED
			case 1:
				expectedStatus = proto.FinalityProviderStatus_JAILED
			case 2:
				expectedStatus = proto.FinalityProviderStatus_INACTIVE
			}
		}

		require.Equal(t, fpInfo.Status, expectedStatus.String())
	})
}

func FuzzUnjailFinalityProvider(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		mockBabylonController := testutil.PrepareMockedBabylonController(t)
		randomStartingHeight := uint64(r.Int63n(100) + 1)
		currentHeight := randomStartingHeight + uint64(r.Int63n(10)+2)
		mockConsumerController := testutil.PrepareMockedConsumerController(t, r, randomStartingHeight, currentHeight)

		// Create randomized config
		pathSuffix := datagen.GenRandomHexStr(r, 10)
		fpHomeDir := filepath.Join(t.TempDir(), "fp-home", pathSuffix)
		fpCfg := config.DefaultConfigWithHome(fpHomeDir)
		// use shorter interval for the test to end faster
		fpCfg.SubmissionRetryInterval = time.Millisecond * 10
		fpCfg.SignatureSubmissionInterval = time.Millisecond * 10

		mockConsumerController.EXPECT().QueryLastPublicRandCommit(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryLatestFinalizedBlock(gomock.Any()).Return(nil, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryLatestBlockHeight(gomock.Any()).Return(currentHeight, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryBlocks(gomock.Any(), gomock.Any()).Return(nil, errors.New("chain not online")).AnyTimes()

		// set voting power to be positive so that the fp should eventually become ACTIVE
		mockConsumerController.EXPECT().QueryFinalityProviderHasPower(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryFinalityActivationBlockHeight(gomock.Any()).Return(uint64(0), nil).AnyTimes()
		mockConsumerController.EXPECT().QueryActivatedHeight(gomock.Any()).Return(uint64(1), nil).AnyTimes()
		mockConsumerController.EXPECT().QueryFinalityProviderStatus(gomock.Any(), gomock.Any()).Return(&api.FinalityProviderStatusResponse{
			Slashed: false,
			Jailed:  true,
		}, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryFinalityProviderHighestVotedHeight(gomock.Any(), gomock.Any()).Return(uint64(0), nil).AnyTimes()

		// Create fp app
		app, fpPk, cleanup := startFPAppWithRegisteredFp(t, r, fpHomeDir, &fpCfg, mockBabylonController, mockConsumerController)
		defer cleanup()

		expectedTxHash := datagen.GenRandomHexStr(r, 32)
		mockConsumerController.EXPECT().UnjailFinalityProvider(gomock.Any(), fpPk.MustToBTCPK()).Return(&types.TxResponse{TxHash: expectedTxHash}, nil).AnyTimes()
		err := app.StartFinalityProvider(fpPk)
		require.NoError(t, err)
		fpIns, err := app.GetFinalityProviderInstance()
		require.NoError(t, err)
		require.True(t, fpIns.IsJailed())
		res, err := app.UnjailFinalityProvider(fpPk)
		require.NoError(t, err)
		require.Equal(t, expectedTxHash, res.TxHash)
		require.Eventually(t, func() bool {
			return !fpIns.IsJailed()
		}, eventuallyWaitTimeOut, eventuallyPollTime)
	})
}

func FuzzSaveAlreadyRegisteredFinalityProvider(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		logger := testutil.GetTestLogger(t)
		// create an EOTS manager
		eotsHomeDir := filepath.Join(t.TempDir(), "eots-home")
		eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
		dbBackend, err := eotsCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)
		useFileKeyring := rand.Intn(2) == 1
		var passphraseEots string

		if useFileKeyring {
			eotsCfg.KeyringBackend = sdkkeyring.BackendFile
			passphraseEots = testutil.GenRandomHexStr(r, 8)
		}

		em, err := eotsmanager.NewLocalEOTSManager(eotsHomeDir, eotsCfg.KeyringBackend, dbBackend, logger)
		require.NoError(t, err)
		defer func() {
			dbBackend.Close()
			err = os.RemoveAll(eotsHomeDir)
			require.NoError(t, err)
		}()

		randomStartingHeight := uint64(r.Int63n(100) + 1)
		currentHeight := randomStartingHeight + uint64(r.Int63n(10)+2)
		mockBabylonController := testutil.PrepareMockedBabylonController(t)
		mockConsumerController := testutil.PrepareMockedConsumerController(t, r, randomStartingHeight, currentHeight)
		rndFp, err := datagen.GenRandomFinalityProvider(r)
		require.NoError(t, err)

		// Create randomized config
		fpHomeDir := filepath.Join(t.TempDir(), "fp-home")
		fpCfg := config.DefaultConfigWithHome(fpHomeDir)
		fpCfg.PollerConfig.AutoChainScanningMode = false
		fpCfg.PollerConfig.StaticChainScanningStartHeight = randomStartingHeight
		fpdb, err := fpCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)
		fpMetrics := metrics.NewFpMetrics()
		poller := service.NewChainPoller(logger, fpCfg.PollerConfig, mockConsumerController, fpMetrics)
		app, err := service.NewFinalityProviderApp(&fpCfg, mockBabylonController, mockConsumerController, em, poller, fpMetrics, fpdb, logger)
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
		eotsPkBz, err := em.CreateKey(eotsKeyName, passphraseEots)
		require.NoError(t, err)
		eotsPk, err = bbntypes.NewBIP340PubKey(eotsPkBz)
		require.NoError(t, err)

		if useFileKeyring {
			err = em.Unlock(eotsPkBz, passphraseEots)
			require.NoError(t, err)
		}

		// generate keyring
		keyName := testutil.GenRandomHexStr(r, 4)
		chainID := testutil.GenRandomHexStr(r, 4)

		cfg := app.GetConfig()
		_, err = testutil.CreateChainKey(cfg.BabylonConfig.KeyDirectory, cfg.BabylonConfig.ChainID, keyName, sdkkeyring.BackendTest, passphrase, hdPath, "")
		require.NoError(t, err)

		fpRes := &btcstakingtypes.QueryFinalityProviderResponse{FinalityProvider: &btcstakingtypes.FinalityProviderResponse{
			Description:          rndFp.Description,
			Commission:           rndFp.Commission,
			Addr:                 rndFp.Addr,
			BtcPk:                eotsPk,
			Pop:                  rndFp.Pop,
			SlashedBabylonHeight: rndFp.SlashedBabylonHeight,
			SlashedBtcHeight:     rndFp.SlashedBtcHeight,
			Jailed:               rndFp.Jailed,
			HighestVotedHeight:   rndFp.HighestVotedHeight,
			CommissionInfo:       rndFp.CommissionInfo,
		}}

		mockBabylonController.EXPECT().QueryFinalityProvider(gomock.Any()).Return(fpRes, nil).AnyTimes()

		res, err := app.CreateFinalityProvider(context.Background(), keyName, chainID, eotsPk, testutil.RandomDescription(r), testutil.ZeroCommissionRate())
		require.NoError(t, err)
		require.Equal(t, res.FpInfo.BtcPkHex, eotsPk.MarshalHex())

		fpInfo, err := app.GetFinalityProviderInfo(eotsPk)
		require.NoError(t, err)
		require.Equal(t, eotsPk.MarshalHex(), fpInfo.BtcPkHex)
	})
}

func startFPAppWithRegisteredFp(t *testing.T, r *rand.Rand, homePath string, cfg *config.Config, cc api.ClientController, consumerCon api.ConsumerController) (*service.FinalityProviderApp, *bbntypes.BIP340PubKey, func()) {
	logger := testutil.GetTestLogger(t)
	// create an EOTS manager
	eotsHomeDir := filepath.Join(t.TempDir(), "eots-home")
	eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
	eotsdb, err := eotsCfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)
	useFileKeyring := rand.Intn(2) == 1
	var passphraseEots string

	if useFileKeyring {
		eotsCfg.KeyringBackend = sdkkeyring.BackendFile
		passphraseEots = testutil.GenRandomHexStr(r, 8)
	}

	em, err := eotsmanager.NewLocalEOTSManager(eotsHomeDir, eotsCfg.KeyringBackend, eotsdb, logger)
	require.NoError(t, err)

	// create finality-provider app with randomized config
	require.NoError(t, err)
	err = util.MakeDirectory(config.DataDir(homePath))
	require.NoError(t, err)
	db, err := cfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)
	fpStore, err := fpstore.NewFinalityProviderStore(db)
	require.NoError(t, err)
	fpMetrics := metrics.NewFpMetrics()
	poller := service.NewChainPoller(logger, cfg.PollerConfig, consumerCon, fpMetrics)
	app, err := service.NewFinalityProviderApp(cfg, cc, consumerCon, em, poller, fpMetrics, db, logger)
	require.NoError(t, err)

	// create registered finality-provider
	keyName := datagen.GenRandomHexStr(r, 10)
	chainID := datagen.GenRandomHexStr(r, 10)
	kr, err := keyring.CreateKeyring(
		cfg.BabylonConfig.KeyDirectory,
		cfg.BabylonConfig.ChainID,
		cfg.BabylonConfig.KeyringBackend,
	)
	require.NoError(t, err)
	kc, err := keyring.NewChainKeyringControllerWithKeyring(kr, keyName)
	require.NoError(t, err)
	btcPkBytes, err := em.CreateKey(keyName, passphraseEots)
	require.NoError(t, err)

	if useFileKeyring {
		err = em.Unlock(btcPkBytes, passphraseEots)
		require.NoError(t, err)
	}

	btcPk, err := bbntypes.NewBIP340PubKey(btcPkBytes)
	require.NoError(t, err)
	keyInfo, err := kc.CreateChainKey(passphrase, hdPath, "")
	require.NoError(t, err)
	fpAddr := keyInfo.AccAddress

	err = fpStore.CreateFinalityProvider(
		fpAddr,
		btcPk.MustToBTCPK(),
		testutil.RandomDescription(r),
		testutil.ZeroCommissionRate(),
		chainID,
	)
	require.NoError(t, err)
	err = app.Start()
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
		err = os.RemoveAll(homePath)
		require.NoError(t, err)
	}

	return app, btcPk, cleanUp
}

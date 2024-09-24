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
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/babylonlabs-io/finality-provider/types"
)

var (
	passphrase = "testpass"
	hdPath     = ""
)

func FuzzRegisterFinalityProvider(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		logger := zap.NewNop()
		// create an EOTS manager
		eotsHomeDir := filepath.Join(t.TempDir(), "eots-home")
		eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
		dbBackend, err := eotsCfg.DatabaseConfig.GetDbBackend()
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
		mockConsumerController := testutil.PrepareMockedConsumerController(t, r, randomStartingHeight, currentHeight)
		mockConsumerController.EXPECT().QueryLatestFinalizedBlock().Return(nil, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryFinalityProviderHasPower(gomock.Any(),
			gomock.Any()).Return(false, nil).AnyTimes()
		mockBabylonController := testutil.PrepareMockedBabylonController(t)

		// Create randomized config
		fpHomeDir := filepath.Join(t.TempDir(), "fp-home")
		fpCfg := config.DefaultConfigWithHome(fpHomeDir)
		fpCfg.PollerConfig.AutoChainScanningMode = false
		fpCfg.PollerConfig.StaticChainScanningStartHeight = randomStartingHeight
		fpdb, err := fpCfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)
		app, err := service.NewFinalityProviderApp(&fpCfg, mockBabylonController, mockConsumerController, em, fpdb, logger)
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
		eotsPk = nil
		generateEotsKeyBefore := r.Int31n(10) > 5
		if generateEotsKeyBefore {
			// sometimes uses the previously generated EOTS pk
			eotsKeyName := testutil.GenRandomHexStr(r, 4)
			eotsPkBz, err := em.CreateKey(eotsKeyName, passphrase, hdPath)
			require.NoError(t, err)
			eotsPk, err = bbntypes.NewBIP340PubKey(eotsPkBz)
			require.NoError(t, err)
		}

		// create a finality-provider object and save it to db
		fp := testutil.GenStoredFinalityProvider(r, t, app, passphrase, hdPath, eotsPk)
		if generateEotsKeyBefore {
			require.Equal(t, eotsPk, bbntypes.NewBIP340PubKeyFromBTCPK(fp.BtcPk))
		}

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
		mockBabylonController.EXPECT().
			RegisterFinalityProvider(
				fp.ChainID,
				fp.BtcPk,
				popBytes,
				testutil.ZeroCommissionRate(),
				gomock.Any(),
			).Return(&types.TxResponse{TxHash: txHash}, nil).AnyTimes()

		res, err := app.RegisterFinalityProvider(fp.GetBIP340BTCPK().MarshalHex())
		require.NoError(t, err)
		require.Equal(t, txHash, res.TxHash)

		mockConsumerController.EXPECT().QueryLastPublicRandCommit(gomock.Any()).Return(nil, nil).AnyTimes()
		err = app.StartHandlingFinalityProvider(fp.GetBIP340BTCPK(), passphrase)
		require.NoError(t, err)

		fpAfterReg, err := app.GetFinalityProviderInstance(fp.GetBIP340BTCPK())
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
		dbBackend, err := eotsCfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)
		em, err := eotsmanager.NewLocalEOTSManager(eotsHomeDir, eotsCfg.KeyringBackend, dbBackend, logger)
		require.NoError(t, err)

		// clean up after the test
		defer func() {
			dbBackend.Close()
			err = os.RemoveAll(eotsHomeDir)
			require.NoError(t, err)
		}()

		// Create randomized config
		fpHomeDir := filepath.Join(t.TempDir(), "fp-home", pathSuffix)
		fpCfg := config.DefaultConfigWithHome(fpHomeDir)
		fpCfg.SyncFpStatusInterval = time.Millisecond * 100
		// no need for other intervals to run
		fpCfg.StatusUpdateInterval = time.Minute * 10
		fpCfg.SubmissionRetryInterval = time.Minute * 10
		fpdb, err := fpCfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)

		randomStartingHeight := uint64(r.Int63n(100) + 1)
		currentHeight := randomStartingHeight + uint64(r.Int63n(10)+2)
		mockConsumerController := testutil.PrepareMockedConsumerController(t, r, randomStartingHeight, currentHeight)
		mockBabylonController := testutil.PrepareMockedBabylonController(t)

		mockConsumerController.EXPECT().QueryLatestFinalizedBlock().Return(nil, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryLatestBlockHeight().Return(currentHeight, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryBlock(gomock.Any()).Return(nil, errors.New("chain not online")).AnyTimes()
		mockConsumerController.EXPECT().QueryLastPublicRandCommit(gomock.Any()).Return(nil, nil).AnyTimes()

		noVotingPowerTable := r.Int31n(10) > 5
		if noVotingPowerTable {
			allowedErr := fmt.Sprintf("failed to query Finality Voting Power at Height %d: rpc error: code = Unknown desc = %s: unknown request", currentHeight, bstypes.ErrVotingPowerTableNotUpdated.Wrapf("height: %d", currentHeight).Error())
			mockConsumerController.EXPECT().QueryFinalityProviderHasPower(gomock.Any(), gomock.Any()).Return(false, errors.New(allowedErr)).AnyTimes()
			mockConsumerController.EXPECT().QueryActivatedHeight().Return(uint64(0), errors.New(allowedErr)).AnyTimes()
		} else {
			mockConsumerController.EXPECT().QueryFinalityProviderHasPower(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
			mockConsumerController.EXPECT().QueryActivatedHeight().Return(currentHeight, nil).AnyTimes()
		}

		app, err := service.NewFinalityProviderApp(&fpCfg, mockBabylonController, mockConsumerController, em, fpdb, logger)
		require.NoError(t, err)

		err = app.Start()
		require.NoError(t, err)

		fp := testutil.GenStoredFinalityProvider(r, t, app, "", hdPath, nil)

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
			fpInstance, err := app.GetFinalityProviderInstance(fpPk)
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

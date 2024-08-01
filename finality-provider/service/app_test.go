package service_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	bbntypes "github.com/babylonlabs-io/babylon/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonchain/finality-provider/eotsmanager"
	eotscfg "github.com/babylonchain/finality-provider/eotsmanager/config"
	"github.com/babylonchain/finality-provider/finality-provider/config"
	"github.com/babylonchain/finality-provider/finality-provider/proto"
	"github.com/babylonchain/finality-provider/finality-provider/service"
	"github.com/babylonchain/finality-provider/testutil"
	"github.com/babylonchain/finality-provider/types"
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

		// create a finality-provider object and save it to db
		fp := testutil.GenStoredFinalityProvider(r, t, app, passphrase, hdPath)
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

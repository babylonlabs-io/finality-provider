package service_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	ccapi "github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	fpstore "github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/keyring"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/babylonlabs-io/finality-provider/util"
)

var (
	eventuallyWaitTimeOut = 5 * time.Second
	eventuallyPollTime    = 10 * time.Millisecond
)

func FuzzStatusUpdate(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		mockBabylonController := testutil.PrepareMockedBabylonController(t)
		randomStartingHeight := uint64(r.Int63n(100) + 100)
		currentHeight := randomStartingHeight + uint64(r.Int63n(10)+1)
		mockConsumerController := testutil.PrepareMockedConsumerController(t, r, randomStartingHeight, currentHeight)
		vm, fpPk, cleanUp := newFinalityProviderManagerWithRegisteredFp(t, r, mockBabylonController, mockConsumerController)
		defer cleanUp()

		// setup mocks
		currentBlockRes := &types.BlockInfo{
			Height: currentHeight,
			Hash:   datagen.GenRandomByteArray(r, 32),
		}
		mockConsumerController.EXPECT().QueryLatestBlockHeight().Return(currentHeight, nil).AnyTimes()
		mockConsumerController.EXPECT().Close().Return(nil).AnyTimes()
		mockConsumerController.EXPECT().QueryLatestFinalizedBlock().Return(nil, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryActivatedHeight().Return(uint64(1), nil).AnyTimes()
		mockConsumerController.EXPECT().QueryBlock(gomock.Any()).Return(currentBlockRes, nil).AnyTimes()
		mockConsumerController.EXPECT().QueryLastPublicRandCommit(gomock.Any()).Return(nil, nil).AnyTimes()

		hasPower := r.Intn(2) != 0
		mockConsumerController.EXPECT().QueryFinalityProviderHasPower(gomock.Any(), currentHeight).Return(hasPower, nil).AnyTimes()
		mockConsumerController.EXPECT().SubmitFinalitySig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&types.TxResponse{TxHash: ""}, nil).AnyTimes()
		var isSlashedOrJailed int
		if !hasPower {
			// 0 means is slashed, 1 means is jailed, 2 means neither slashed nor jailed
			isSlashedOrJailed = r.Intn(3)
			if isSlashedOrJailed == 0 {
				mockBabylonController.EXPECT().QueryFinalityProviderSlashedOrJailed(gomock.Any()).Return(true, false, nil).AnyTimes()
			} else if isSlashedOrJailed == 1 {
				mockBabylonController.EXPECT().QueryFinalityProviderSlashedOrJailed(gomock.Any()).Return(false, true, nil).AnyTimes()
			} else {
				mockBabylonController.EXPECT().QueryFinalityProviderSlashedOrJailed(gomock.Any()).Return(false, false, nil).AnyTimes()
			}
		}

		err := vm.StartFinalityProvider(fpPk, passphrase)
		require.NoError(t, err)
		fpIns := vm.ListFinalityProviderInstances()[0]
		// stop the finality-provider as we are testing static functionalities
		err = fpIns.Stop()
		require.NoError(t, err)

		if hasPower {
			waitForStatus(t, fpIns, proto.FinalityProviderStatus_ACTIVE)
		} else {
			if isSlashedOrJailed == 2 && fpIns.GetStatus() == proto.FinalityProviderStatus_ACTIVE {
				waitForStatus(t, fpIns, proto.FinalityProviderStatus_INACTIVE)
			} else if isSlashedOrJailed == 1 {
				waitForStatus(t, fpIns, proto.FinalityProviderStatus_JAILED)
			} else if isSlashedOrJailed == 0 {
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

func newFinalityProviderManagerWithRegisteredFp(t *testing.T, r *rand.Rand, cc ccapi.ClientController, consumerCon ccapi.ConsumerController) (*service.FinalityProviderManager, *bbntypes.BIP340PubKey, func()) {
	logger := zap.NewNop()
	// create an EOTS manager
	eotsHomeDir := filepath.Join(t.TempDir(), "eots-home")
	eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
	eotsdb, err := eotsCfg.DatabaseConfig.GetDbBackend()
	require.NoError(t, err)
	em, err := eotsmanager.NewLocalEOTSManager(eotsHomeDir, eotsCfg.KeyringBackend, eotsdb, logger)
	require.NoError(t, err)

	// create finality-provider app with randomized config
	fpHomeDir := filepath.Join(t.TempDir(), "fp-home")
	fpCfg := fpcfg.DefaultConfigWithHome(fpHomeDir)
	fpCfg.StatusUpdateInterval = 10 * time.Millisecond
	input := strings.NewReader("")
	kr, err := keyring.CreateKeyring(
		fpCfg.BabylonConfig.KeyDirectory,
		fpCfg.BabylonConfig.ChainID,
		fpCfg.BabylonConfig.KeyringBackend,
		input,
	)
	require.NoError(t, err)
	err = util.MakeDirectory(fpcfg.DataDir(fpHomeDir))
	require.NoError(t, err)
	db, err := fpCfg.DatabaseConfig.GetDbBackend()
	require.NoError(t, err)
	fpStore, err := fpstore.NewFinalityProviderStore(db)
	require.NoError(t, err)
	pubRandStore, err := fpstore.NewPubRandProofStore(db)
	require.NoError(t, err)

	metricsCollectors := metrics.NewFpMetrics()
	vm, err := service.NewFinalityProviderManager(fpStore, pubRandStore, &fpCfg, cc, consumerCon, em, metricsCollectors, logger)
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
		err = vm.Stop()
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

	return vm, btcPk, cleanUp
}

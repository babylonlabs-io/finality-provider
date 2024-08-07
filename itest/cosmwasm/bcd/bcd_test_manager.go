//go:build e2e_bcd
// +build e2e_bcd

package e2etest_bcd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	sdklogs "cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	wasmapp "github.com/CosmWasm/wasmd/app"
	wasmparams "github.com/CosmWasm/wasmd/app/params"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	_ "github.com/babylonlabs-io/babylon-sdk/demo/app"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	bbncc "github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	cwcc "github.com/babylonlabs-io/finality-provider/clientcontroller/cosmwasm"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	eotsconfig "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	dbm "github.com/cosmos/cosmos-db"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"go.uber.org/zap"
)

type BcdTestManager struct {
	BabylonHandler    *e2eutils.BabylonNodeHandler
	FpConfig          *fpcfg.Config
	BBNClient         *bbncc.BabylonController
	BcdHandler        *BcdNodeHandler
	BcdConsumerClient *cwcc.CosmwasmConsumerController
	StakingParams     *types.StakingParams
	EOTSServerHandler *e2eutils.EOTSServerHandler
	EOTSConfig        *eotsconfig.Config
	Fpa               *service.FinalityProviderApp
	EOTSClient        *client.EOTSManagerGRpcClient
	CovenantPrivKeys  []*btcec.PrivateKey
	baseDir           string
}

func createLogger(t *testing.T, level zapcore.Level) *zap.Logger {
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(level)
	logger, err := config.Build()
	require.NoError(t, err)
	return logger
}

func StartBcdTestManager(t *testing.T) *BcdTestManager {
	// Setup consumer test manager
	testDir, err := e2eutils.BaseDir("fpe2etest")
	require.NoError(t, err)

	logger := createLogger(t, zapcore.ErrorLevel)

	// 1. generate covenant committee
	covenantQuorum := 2
	numCovenants := 3
	covenantPrivKeys, covenantPubKeys := e2eutils.GenerateCovenantCommittee(numCovenants, t)

	// 2. prepare Babylon node
	bh := e2eutils.NewBabylonNodeHandler(t, covenantQuorum, covenantPubKeys)
	err = bh.Start()
	require.NoError(t, err)
	fpHomeDir := filepath.Join(testDir, "fp-home")
	cfg := e2eutils.DefaultFpConfig(bh.GetNodeDataDir(), fpHomeDir)
	bc, err := bbncc.NewBabylonController(cfg.BabylonConfig, &cfg.BTCNetParams, logger)
	require.NoError(t, err)

	// 3. setup bcd node
	wh := NewBcdNodeHandler(t)
	err = wh.Start()
	require.NoError(t, err)
	cfg.CosmwasmConfig = config.DefaultCosmwasmConfig()
	cfg.CosmwasmConfig.KeyDirectory = wh.dataDir
	// make random contract address for now to avoid validation errors, later we will update it with the correct address in the test
	cfg.CosmwasmConfig.BtcStakingContractAddress = datagen.GenRandomAccount().GetAddress().String()
	cfg.ChainName = fpcc.WasmConsumerChainName
	cfg.CosmwasmConfig.AccountPrefix = "bbnc"
	cfg.CosmwasmConfig.ChainID = bcdChainID
	cfg.CosmwasmConfig.RPCAddr = fmt.Sprintf("http://localhost:%d", bcdRpcPort)
	// tempApp := bcdapp.NewTmpApp() // TODO: investigate why wasmapp works and bcdapp doesn't
	tempApp := wasmapp.NewWasmApp(sdklogs.NewNopLogger(), dbm.NewMemDB(), nil, false, simtestutil.NewAppOptionsWithFlagHome(t.TempDir()), []wasmkeeper.Option{})
	encodingCfg := wasmparams.EncodingConfig{
		InterfaceRegistry: tempApp.InterfaceRegistry(),
		Codec:             tempApp.AppCodec(),
		TxConfig:          tempApp.TxConfig(),
		Amino:             tempApp.LegacyAmino(),
	}
	wcc, err := cwcc.NewCosmwasmConsumerController(cfg.CosmwasmConfig, encodingCfg, logger)
	require.NoError(t, err)

	// 4. prepare EOTS manager
	eotsHomeDir := filepath.Join(testDir, "eots-home")
	eotsCfg := eotsconfig.DefaultConfigWithHomePath(eotsHomeDir)
	eh := e2eutils.NewEOTSServerHandler(t, eotsCfg, eotsHomeDir)
	eh.Start()
	// wait for EOTS servers to start
	// see https://github.com/babylonchain/finality-provider/pull/517
	var eotsCli *client.EOTSManagerGRpcClient
	require.Eventually(t, func() bool {
		eotsCli, err = client.NewEOTSManagerGRpcClient(cfg.EOTSManagerAddress)
		return err == nil
	}, 5*time.Second, time.Second, "Failed to create EOTS clients")

	// 5. prepare finality-provider
	fpdb, err := cfg.DatabaseConfig.GetDbBackend()
	require.NoError(t, err)
	fpApp, err := service.NewFinalityProviderApp(cfg, bc, wcc, eotsCli, fpdb, logger)
	require.NoError(t, err)
	err = fpApp.Start()
	require.NoError(t, err)

	// TODO: setup fp app after contract supports relevant queries

	ctm := &BcdTestManager{
		BabylonHandler:    bh,
		FpConfig:          cfg,
		BBNClient:         bc,
		BcdHandler:        wh,
		BcdConsumerClient: wcc,
		EOTSServerHandler: eh,
		EOTSConfig:        eotsCfg,
		Fpa:               fpApp,
		EOTSClient:        eotsCli,
		CovenantPrivKeys:  covenantPrivKeys,
		baseDir:           testDir,
	}

	ctm.WaitForServicesStart(t)
	return ctm
}

func (ctm *BcdTestManager) WaitForServicesStart(t *testing.T) {
	require.Eventually(t, func() bool {
		params, err := ctm.BBNClient.QueryStakingParams()
		if err != nil {
			return false
		}
		ctm.StakingParams = params
		return true
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
	t.Logf("Babylon node is started")

	// wait for wasmd to start
	require.Eventually(t, func() bool {
		bcdNodeStatus, err := ctm.BcdConsumerClient.GetCometNodeStatus()
		if err != nil {
			t.Logf("Error getting bcd node status: %v", err)
			return false
		}
		return bcdNodeStatus.SyncInfo.LatestBlockHeight > 2
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
	t.Logf("Bcd node is started")
}

func (ctm *BcdTestManager) Stop(t *testing.T) {
	err := ctm.Fpa.Stop()
	require.NoError(t, err)
	err = ctm.BabylonHandler.Stop()
	require.NoError(t, err)
	ctm.EOTSServerHandler.Stop()
	ctm.BcdHandler.Stop(t)
	err = os.RemoveAll(ctm.baseDir)
	require.NoError(t, err)
}

func (ctm *BcdTestManager) CreateConsumerFinalityProviders(t *testing.T, consumerId string, n int) []*service.FinalityProviderInstance {
	app := ctm.Fpa
	cfg := app.GetConfig()
	keyName := cfg.BabylonConfig.Key

	// register all finality providers
	fpPKs := make([]*bbntypes.BIP340PubKey, 0, n)
	for i := 0; i < n; i++ {
		moniker := e2eutils.MonikerPrefix + consumerId + "-" + strconv.Itoa(i)
		commission := sdkmath.LegacyZeroDec()
		desc := e2eutils.NewDescription(moniker)

		res, err := app.CreateFinalityProvider(keyName, consumerId, e2eutils.Passphrase, e2eutils.HdPath, desc, &commission)
		require.NoError(t, err)
		fpPk, err := bbntypes.NewBIP340PubKeyFromHex(res.FpInfo.BtcPkHex)
		require.NoError(t, err)
		fpPKs = append(fpPKs, fpPk)
		_, err = app.RegisterFinalityProvider(fpPk.MarshalHex())
		require.NoError(t, err)
	}

	for i := 0; i < n; i++ {
		// start
		err := app.StartHandlingFinalityProvider(fpPKs[i], e2eutils.Passphrase)
		require.NoError(t, err)
		fpIns, err := app.GetFinalityProviderInstance(fpPKs[i])
		require.NoError(t, err)
		require.True(t, fpIns.IsRunning())
		require.NoError(t, err)
	}

	// check finality providers on Babylon side
	require.Eventually(t, func() bool {
		fps, err := ctm.BBNClient.QueryConsumerFinalityProviders(consumerId)
		if err != nil {
			t.Logf("failed to query finality providers from Babylon %s", err.Error())
			return false
		}

		if len(fps) != n {
			return false
		}

		for _, fp := range fps {
			if !strings.Contains(fp.Description.Moniker, e2eutils.MonikerPrefix) {
				return false
			}
			if !fp.Commission.Equal(sdkmath.LegacyZeroDec()) {
				return false
			}
		}

		return true
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

	fpInsList := app.ListFinalityProviderInstancesForChain(consumerId)
	require.Equal(t, n, len(fpInsList))

	t.Logf("the consumer test manager is running with %v finality-provider(s)", len(fpInsList))

	return fpInsList
}

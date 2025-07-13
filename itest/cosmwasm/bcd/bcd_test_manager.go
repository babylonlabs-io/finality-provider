//go:build e2e_bcd
// +build e2e_bcd

package e2etest_bcd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/babylonlabs-io/finality-provider/metrics"

	sdklogs "cosmossdk.io/log"
	wasmapp "github.com/CosmWasm/wasmd/app"
	wasmparams "github.com/CosmWasm/wasmd/app/params"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	_ "github.com/babylonlabs-io/babylon-sdk/demo/app"
	bbnsdktypes "github.com/babylonlabs-io/babylon-sdk/x/babylon/types"
	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"
	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	dbm "github.com/cosmos/cosmos-db"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	ccapi "github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	bbncc "github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	cwcc "github.com/babylonlabs-io/finality-provider/clientcontroller/cosmwasm"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	eotsconfig "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	"github.com/babylonlabs-io/finality-provider/itest/container"
	base_test_manager "github.com/babylonlabs-io/finality-provider/itest/test-manager"
	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/babylonlabs-io/finality-provider/types"
)

type BcdTestManager struct {
	*base_test_manager.BaseTestManager
	manager           *container.Manager
	FpConfig          *fpcfg.Config
	BcdHandler        *BcdNodeHandler
	BcdConsumerClient *cwcc.CosmwasmConsumerController
	StakingParams     *types.StakingParams
	EOTSServerHandler *e2eutils.EOTSServerHandler
	EOTSConfig        *eotsconfig.Config
	Fpa               *service.FinalityProviderApp
	EOTSClient        *client.EOTSManagerGRpcClient
	baseDir           string
	logger            *zap.Logger
}

func createLogger(t *testing.T, level zapcore.Level) *zap.Logger {
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(level)
	logger, err := config.Build()
	require.NoError(t, err)
	return logger
}

func StartBcdTestManager(t *testing.T, ctx context.Context) *BcdTestManager {
	testDir, err := base_test_manager.TempDir(t, "fp-e2e-test-*")
	require.NoError(t, err)

	loggerConfig := zap.NewDevelopmentConfig()
	loggerConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	logger, err := loggerConfig.Build()
	require.NoError(t, err)

	// 1. generate covenant committee
	covenantQuorum := 2
	numCovenants := 3
	covenantPrivKeys, covenantPubKeys := e2eutils.GenerateCovenantCommittee(numCovenants, t)

	// 2. prepare Babylon node
	manager, err := container.NewManager(t)
	require.NoError(t, err)

	// Create temp dir for babylon node
	babylonDir, err := base_test_manager.TempDir(t, "babylon-test-*")
	require.NoError(t, err)

	// Start babylon node in docker
	babylond, err := manager.RunBabylondResource(t, babylonDir, covenantQuorum, covenantPubKeys)
	require.NoError(t, err)
	require.NotNil(t, babylond)

	keyDir := filepath.Join(babylonDir, "node0", "babylond")
	fpHomeDir := filepath.Join(testDir, "fp-home")
	cfg := e2eutils.DefaultFpConfig(keyDir, fpHomeDir)

	// Update ports with dynamically allocated ones from docker
	cfg.BabylonConfig.RPCAddr = fmt.Sprintf("http://localhost:%s", babylond.GetPort("26657/tcp"))
	cfg.BabylonConfig.GRPCAddr = fmt.Sprintf("localhost:%s", babylond.GetPort("9090/tcp"))

	cfg.ContextSigningHeight = ^uint64(0) // enable context signing height, max uint64 value

	var bc ccapi.ClientController
	require.Eventually(t, func() bool {
		bbnCfg := fpcfg.BBNConfigToBabylonConfig(cfg.BabylonConfig)
		bbnCl, err := bbnclient.New(&bbnCfg, logger)
		if err != nil {
			t.Logf("failed to create Babylon client: %v", err)
			return false
		}
		bc, err = bbncc.NewBabylonController(bbnCl, cfg.BabylonConfig, logger)
		if err != nil {
			t.Logf("failed to create Babylon controller: %v", err)
			return false
		}
		err = bc.Start()
		if err != nil {
			t.Logf("failed to start Babylon controller: %v", err)
			return false
		}
		return true
	}, 5*time.Second, e2eutils.EventuallyPollTime)

	// 3. setup bcd node
	wh := NewBcdNodeHandler(t)
	err = wh.Start()
	require.NoError(t, err)
	cfg.CosmwasmConfig = fpcfg.DefaultCosmwasmConfig()
	cfg.CosmwasmConfig.KeyDirectory = wh.dataDir
	// make random contract address for now to avoid validation errors, later we will update it with the correct address in the test
	cfg.CosmwasmConfig.BtcStakingContractAddress = datagen.GenRandomAccount().GetAddress().String()
	cfg.CosmwasmConfig.BtcFinalityContractAddress = datagen.GenRandomAccount().GetAddress().String()
	cfg.ChainType = fpcc.WasmConsumerChainType
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
	bbnsdktypes.RegisterInterfaces(encodingCfg.InterfaceRegistry)

	var wcc *cwcc.CosmwasmConsumerController
	require.Eventually(t, func() bool {
		wcc, err = cwcc.NewCosmwasmConsumerController(cfg.CosmwasmConfig, encodingCfg, logger)
		if err != nil {
			t.Logf("failed to create Cosmwasm consumer controller: %v", err)
			return false
		}
		return true
	}, 5*time.Second, e2eutils.EventuallyPollTime)

	// 4. prepare EOTS manager
	eotsHomeDir := filepath.Join(testDir, "eots-home")
	eotsCfg := eotsconfig.DefaultConfigWithHomePath(eotsHomeDir)
	eotsCfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eotsCfg.Metrics.Port = testutil.AllocateUniquePort(t)
	eh := e2eutils.NewEOTSServerHandler(t, eotsCfg, eotsHomeDir)
	eh.Start(ctx)
	cfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eotsCli, err := client.NewEOTSManagerGRpcClient(eotsCfg.RPCListener, "")
	require.NoError(t, err)

	fpMetrics := metrics.NewFpMetrics()
	poller := service.NewChainPoller(logger, cfg.PollerConfig, wcc, fpMetrics)

	// 5. prepare finality-provider
	fpdb, err := cfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)
	pubRandStore, err := store.NewPubRandProofStore(fpdb)
	require.NoError(t, err)
	rndCommitter := service.NewDefaultRandomnessCommitter(
		service.NewRandomnessCommitterConfig(cfg.NumPubRand, int64(cfg.TimestampingDelayBlocks), cfg.ContextSigningHeight),
		service.NewPubRandState(pubRandStore), wcc, eotsCli, logger, fpMetrics)
	heightDeterminer := service.NewStartHeightDeterminer(wcc, cfg.PollerConfig, logger)

	fpApp, err := service.NewFinalityProviderApp(cfg, bc, wcc, eotsCli, poller, rndCommitter, heightDeterminer, fpMetrics, fpdb, logger)
	require.NoError(t, err)
	err = fpApp.Start()
	require.NoError(t, err)

	ctm := &BcdTestManager{
		BaseTestManager: &base_test_manager.BaseTestManager{
			BBNClient:        bc.(*bbncc.BabylonController),
			CovenantPrivKeys: covenantPrivKeys,
		},
		manager:           manager,
		FpConfig:          cfg,
		BcdHandler:        wh,
		BcdConsumerClient: wcc,
		EOTSServerHandler: eh,
		EOTSConfig:        eotsCfg,
		Fpa:               fpApp,
		EOTSClient:        eotsCli,
		baseDir:           testDir,
		logger:            logger,
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
	err = ctm.manager.ClearResources()
	require.NoError(t, err)
	err = os.RemoveAll(ctm.baseDir)
	require.NoError(t, err)
}

// CreateConsumerFinalityProviders creates finality providers for a consumer chain
// and registers them in Babylon and consumer smart contract
func (ctm *BcdTestManager) CreateConsumerFinalityProviders(t *testing.T, consumerId string) *service.FinalityProviderInstance {
	app := ctm.Fpa
	cfg := app.GetConfig()
	keyName := cfg.BabylonConfig.Key

	// register all finality providers
	moniker := e2eutils.MonikerPrefix + consumerId + "-" + strconv.Itoa(0)
	commission := testutil.ZeroCommissionRate()
	desc := e2eutils.NewDescription(moniker)

	eotsPk, err := ctm.EOTSServerHandler.CreateKey(keyName, "")
	require.NoError(t, err)
	eotsPubKey, err := bbntypes.NewBIP340PubKey(eotsPk)
	require.NoError(t, err)

	// inject fp in smart contract using admin
	fpMsg := e2eutils.GenBtcStakingFpExecMsg(eotsPubKey.MarshalHex())
	fpMsgBytes, err := json.Marshal(fpMsg)
	require.NoError(t, err)
	_, err = ctm.BcdConsumerClient.ExecuteBTCStakingContract(context.Background(), fpMsgBytes)
	require.NoError(t, err)

	// register fp in Babylon
	_, err = app.CreateFinalityProvider(context.Background(), keyName, consumerId, eotsPubKey, desc, commission)
	require.NoError(t, err)

	cfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	cfg.Metrics.Port = testutil.AllocateUniquePort(t)

	err = app.StartFinalityProvider(eotsPubKey)
	require.NoError(t, err)

	fpIns, err := app.GetFinalityProviderInstance()
	require.NoError(t, err)
	require.True(t, fpIns.IsRunning())

	// ensure finality providers are registered in smart contract
	require.Eventually(t, func() bool {
		consumerFpsResp, err := ctm.BcdConsumerClient.QueryFinalityProviders(context.Background())
		if err != nil {
			t.Logf("failed to query finality providers from consumer contract: %s", err.Error())
			return false
		}
		if consumerFpsResp == nil {
			return false
		}
		if len(consumerFpsResp.Fps) != 1 {
			return false
		}
		// verify each FP matches the expected public key
		if consumerFpsResp.Fps[0].BtcPkHex != eotsPubKey.MarshalHex() {
			return false
		}
		return true
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

	t.Logf("the consumer test manager is running with %v finality-provider(s)", 1)

	return fpIns
}

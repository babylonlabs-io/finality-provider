//go:build e2e_op
// +build e2e_op

package e2etest_op

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	bbncfg "github.com/babylonlabs-io/babylon/client/config"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	fgcfg "github.com/babylonlabs-io/finality-gadget/config"
	fgdb "github.com/babylonlabs-io/finality-gadget/db"
	"github.com/babylonlabs-io/finality-gadget/finalitygadget"
	fgsrv "github.com/babylonlabs-io/finality-gadget/server"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	bbncc "github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	opcc "github.com/babylonlabs-io/finality-provider/clientcontroller/opstackl2"
	cwclient "github.com/babylonlabs-io/finality-provider/cosmwasmclient/client"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	eotsclient "github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	eotsconfig "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	base_test_manager "github.com/babylonlabs-io/finality-provider/itest/test-manager"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/testutil/log"
	"github.com/babylonlabs-io/finality-provider/types"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	ope2e "github.com/ethereum-optimism/optimism/op-e2e"
	optestlog "github.com/ethereum-optimism/optimism/op-service/testlog"
	gethlog "github.com/ethereum/go-ethereum/log"
	"github.com/lightningnetwork/lnd/signal"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	opFinalityGadgetContractPath = "../bytecode/op_finality_gadget_16f6154.wasm"
	consumerChainIdPrefix        = "op-stack-l2-"
	bbnAddrTopUpAmount           = 100000000
	finalityGadgetRpc            = "localhost:50051"
	finalityGadgetHttp           = "localhost:8080"
	fgDbFilePath                 = "data.db"
)

type BaseTestManager = base_test_manager.BaseTestManager

type OpL2ConsumerTestManager struct {
	BaseTestManager
	BaseDir           string
	OpSystem          *ope2e.System
	BabylonHandler    *e2eutils.BabylonNodeHandler
	EOTSServerHandler *e2eutils.EOTSServerHandler
	BabylonFpApp      *service.FinalityProviderApp
	ConsumerFpApp     *service.FinalityProviderApp
	FinalityGadget    *finalitygadget.FinalityGadget
}

// - setup OP consumer chain
// - setup Babylon finality system
//   - start Babylon node and wait for it starts
//   - start Babylon and consumer EOTS manager
//   - create and start Babylon FP app
//   - register consumer chain to Babylon
//   - deploy finality gadget cw contract
//   - create and start consumer FP app
//   - start finality gadget server
//
// - update the finality gadget gRPC and then restart op-node
// - create btc delegation & wait for activation
// - set enabled to true in CW contract
func StartOpL2ConsumerManager(t *testing.T) *OpL2ConsumerTestManager {
	// Setup base dir and logger
	testDir, err := e2eutils.BaseDir("fpe2etest")
	require.NoError(t, err)

	// setup logger
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.Level(zap.DebugLevel))
	logger, err := config.Build()
	require.NoError(t, err)

	// start OP consumer chain
	opSys := startOpConsumerChain(t, logger)

	opConsumerId := getConsumerChainId(&opSys.Cfg)

	// start Babylon node
	babylonHandler, covenantPrivKeys := startBabylonNode(t)

	// create Babylon FP config
	babylonFpCfg := createBabylonFpConfig(t, testDir, babylonHandler)

	// create Babylon controller
	babylonController, err := bbncc.NewBabylonController(babylonFpCfg.BabylonConfig, &babylonFpCfg.BTCNetParams, logger)
	require.NoError(t, err)

	// wait for Babylon node starts b/c we will fund the FP address with babylon node
	stakingParams := WaitForBabylonNodeStart(t, babylonController)

	// deploy cw contract with babylon node keyring
	opL2ConsumerConfig := mockOpL2ConsumerCtrlConfig(babylonHandler.GetNodeDataDir())

	// create consumer FP config
	// it would be updated with updated opL2ConsumerConfig later
	consumerFpCfg := createConsumerFpConfig(t, testDir, babylonHandler, opL2ConsumerConfig)

	// create shutdown interceptor
	shutdownInterceptor, err := signal.Intercept()
	require.NoError(t, err)

	// create EOTS handler and EOTS gRPC clients for Babylon and consumer
	eotsHandler, EOTSClients := startEotsManagers(t, logger, testDir, babylonFpCfg, consumerFpCfg, &shutdownInterceptor)

	// create Babylon consumer controller
	babylonConsumerController, err := bbncc.NewBabylonConsumerController(babylonFpCfg.BabylonConfig, &babylonFpCfg.BTCNetParams, logger)
	require.NoError(t, err)

	// create and start Babylon FP app
	babylonFpApp := createAndStartFpApp(t, logger, babylonFpCfg, babylonConsumerController, EOTSClients[0])
	t.Log(log.Prefix("Started Babylon FP App"))

	// register consumer chain to Babylon
	_, err = babylonController.RegisterConsumerChain(
		opConsumerId,
		"OP consumer chain (test)",
		"some description about the chain",
	)
	require.NoError(t, err)
	t.Logf(log.Prefix("Register consumer %s to Babylon"), opConsumerId)

	cwConfig := opL2ConsumerConfig.ToCosmwasmConfig()
	cwClient, err := opcc.NewCwClient(&cwConfig, logger)
	require.NoError(t, err)

	// deploy finality gadget cw contract
	opFinalityGadgetAddress := deployCwContract(t, opConsumerId, cwClient)
	t.Logf(log.Prefix("op-finality-gadget contract address: %s"), opFinalityGadgetAddress)

	// update opL2ConsumerConfig
	opL2ConsumerConfig.OPStackL2RPCAddress = opSys.NodeEndpoint("sequencer").RPC()
	opL2ConsumerConfig.OPFinalityGadgetAddress = opFinalityGadgetAddress
	opL2ConsumerConfig.BabylonFinalityGadgetRpc = finalityGadgetRpc

	// create op consumer controller
	opConsumerController, err := opcc.NewOPStackL2ConsumerController(opL2ConsumerConfig, logger)
	require.NoError(t, err)

	// update consumer FP config
	consumerFpCfg.OPStackL2Config = opL2ConsumerConfig

	// create and start consumer FP app
	consumerFpApp := createAndStartFpApp(t, logger, consumerFpCfg, opConsumerController, EOTSClients[1])
	t.Log(log.Prefix("Started Consumer FP App"))

	// create and register consumer FP
	consumerFpPk := createAndRegisterFinalityProvider(t, consumerFpApp, opConsumerId)
	t.Logf(log.Prefix("Registered Finality Provider %s for %s"), consumerFpPk.MarshalHex(), opConsumerId)

	// start finality gadget server
	fg := startFinalityGadgetServer(t, logger, opL2ConsumerConfig, shutdownInterceptor)

	ctm := &OpL2ConsumerTestManager{
		BaseTestManager: BaseTestManager{
			BBNClient:        babylonController,
			CovenantPrivKeys: covenantPrivKeys,
			StakingParams:    stakingParams,
		},
		BaseDir:           testDir,
		OpSystem:          opSys,
		BabylonHandler:    babylonHandler,
		EOTSServerHandler: eotsHandler,
		BabylonFpApp:      babylonFpApp,
		ConsumerFpApp:     consumerFpApp,
		FinalityGadget:    fg,
	}

	return ctm
}

func WaitForBabylonNodeStart(t *testing.T, babylonController *bbncc.BabylonController) *types.StakingParams {
	var stakingParams *types.StakingParams
	// wait for Babylon node starts
	require.Eventually(t, func() bool {
		params, err := babylonController.QueryStakingParams()
		if err != nil {
			return false
		}
		stakingParams = params
		return true
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

	t.Logf("Babylon node is started")
	return stakingParams
}

// - set babylonFinalityGadgetRpc with empty string
// - start op stack system
// - return op stack system
func startOpConsumerChain(t *testing.T, logger *zap.Logger) *ope2e.System {
	// DefaultSystemConfig load the op deploy config from devnet-data folder
	opSysCfg := ope2e.DefaultSystemConfig(t)

	// supress OP system logs
	opSysCfg.Loggers["verifier"] = optestlog.Logger(t, gethlog.LevelError).New("role", "verifier")
	opSysCfg.Loggers["sequencer"] = optestlog.Logger(t, gethlog.LevelError).New("role", "sequencer")
	opSysCfg.Loggers["batcher"] = optestlog.Logger(t, gethlog.LevelError).New("role", "watcher")
	opSysCfg.Loggers["proposer"] = optestlog.Logger(t, gethlog.LevelError).New("role", "proposer")

	// set babylonFinalityGadgetRpc to empty string
	opSysCfg.DeployConfig.BabylonFinalityGadgetRpc = ""

	// start op stack system
	opSys, err := opSysCfg.Start(t)
	require.NoError(t, err, "Error starting up op stack system")

	return opSys
}

// wait for chain has at least one finalized block
func waitForOneFinalizedBlock(t *testing.T, opSys *ope2e.System) {
	rollupClient := opSys.RollupClient("sequencer")
	require.Eventually(t, func() bool {
		stat, err := rollupClient.SyncStatus(context.Background())
		require.NoError(t, err)
		finalizedHeight := stat.FinalizedL2.Number
		t.Logf("finalized block height: %d", finalizedHeight)
		return finalizedHeight > 1
	}, 30*time.Second, 2*time.Second, "expect at least one finalized block")
}

func startBabylonNode(t *testing.T) (*e2eutils.BabylonNodeHandler, []*secp256k1.PrivateKey) {
	// generate covenant committee
	covenantQuorum := 2
	numCovenants := 3
	covenantPrivKeys, covenantPubKeys := e2eutils.GenerateCovenantCommittee(numCovenants, t)

	bh := e2eutils.NewBabylonNodeHandler(t, covenantQuorum, covenantPubKeys)
	err := bh.Start()
	require.NoError(t, err)
	return bh, covenantPrivKeys
}

func getConsumerChainId(opSysCfg *ope2e.SystemConfig) string {
	l2ChainId := opSysCfg.DeployConfig.L2ChainID
	return fmt.Sprintf("%s%d", consumerChainIdPrefix, l2ChainId)
}

func startEotsManagers(
	t *testing.T,
	logger *zap.Logger,
	testDir string,
	babylonFpCfg *fpcfg.Config,
	consumerFpCfg *fpcfg.Config,
	shutdownInterceptor *signal.Interceptor,
) (*e2eutils.EOTSServerHandler, []*eotsclient.EOTSManagerGRpcClient) {
	fpCfgs := []*fpcfg.Config{babylonFpCfg, consumerFpCfg}
	eotsClients := make([]*eotsclient.EOTSManagerGRpcClient, len(fpCfgs))
	eotsHomeDirs := []string{filepath.Join(testDir, "babylon-eots-home"), filepath.Join(testDir, "consumer-eots-home")}
	eotsConfigs := make([]*eotsconfig.Config, len(fpCfgs))
	for i := 0; i < len(fpCfgs); i++ {
		eotsCfg := eotsconfig.DefaultConfigWithHomePathAndPorts(
			eotsHomeDirs[i],
			eotsconfig.DefaultRPCPort+i,
			metrics.DefaultEotsConfig().Port+i,
		)
		eotsConfigs[i] = eotsCfg
	}

	eh := e2eutils.NewEOTSServerHandlerMultiFP(t, logger, eotsConfigs, eotsHomeDirs, shutdownInterceptor)
	eh.Start()

	// create EOTS clients
	for i := 0; i < len(fpCfgs); i++ {
		// wait for EOTS servers to start
		// see https://github.com/babylonchain/finality-provider/pull/517
		var eotsCli *eotsclient.EOTSManagerGRpcClient
		var err error
		require.Eventually(t, func() bool {
			eotsCli, err = eotsclient.NewEOTSManagerGRpcClient(fpCfgs[i].EOTSManagerAddress)
			if err != nil {
				t.Logf("Error creating EOTS client: %v", err)
				return false
			}
			eotsClients[i] = eotsCli
			return true
		}, 5*time.Second, time.Second, "Failed to create EOTS client")
	}
	return eh, eotsClients
}

func createAndStartFpApp(
	t *testing.T,
	logger *zap.Logger,
	cfg *fpcfg.Config,
	cc api.ConsumerController,
	eotsCli *client.EOTSManagerGRpcClient,
) *service.FinalityProviderApp {
	bc, err := bbncc.NewBabylonController(cfg.BabylonConfig, &cfg.BTCNetParams, logger)
	require.NoError(t, err)

	fpdb, err := cfg.DatabaseConfig.GetDbBackend()
	require.NoError(t, err)

	fpApp, err := service.NewFinalityProviderApp(cfg, bc, cc, eotsCli, fpdb, logger)
	require.NoError(t, err)

	err = fpApp.Start()
	require.NoError(t, err)

	return fpApp
}

func createBabylonFpConfig(
	t *testing.T,
	testDir string,
	bh *e2eutils.BabylonNodeHandler,
) *fpcfg.Config {
	fpHomeDir := filepath.Join(testDir, "babylon-fp-home")
	t.Logf(log.Prefix("Babylon FP home dir: %s"), fpHomeDir)
	cfg := e2eutils.DefaultFpConfigWithPorts(
		bh.GetNodeDataDir(),
		fpHomeDir,
		fpcfg.DefaultRPCPort,
		metrics.DefaultFpConfig().Port,
		eotsconfig.DefaultRPCPort,
	)

	return cfg
}

func createConsumerFpConfig(
	t *testing.T,
	testDir string,
	bh *e2eutils.BabylonNodeHandler,
	opL2ConsumerConfig *fpcfg.OPStackL2Config,
) *fpcfg.Config {
	fpHomeDir := filepath.Join(testDir, "consumer-fp-home")
	t.Logf(log.Prefix("Consumer FP home dir: %s"), fpHomeDir)

	cfg := e2eutils.DefaultFpConfigWithPorts(
		fpHomeDir,
		fpHomeDir,
		fpcfg.DefaultRPCPort+1,
		metrics.DefaultFpConfig().Port+1,
		eotsconfig.DefaultRPCPort+1,
	)

	fpBbnKeyInfo := createChainKey(cfg.BabylonConfig, t)
	fundBBNAddr(t, bh, fpBbnKeyInfo)

	opL2ConsumerConfig.KeyDirectory = cfg.BabylonConfig.KeyDirectory
	opL2ConsumerConfig.Key = cfg.BabylonConfig.Key
	cfg.OPStackL2Config = opL2ConsumerConfig

	return cfg
}

func createChainKey(bbnConfig *fpcfg.BBNConfig, t *testing.T) *types.ChainKeyInfo {
	fpBbnKeyInfo, err := service.CreateChainKey(
		bbnConfig.KeyDirectory,
		bbnConfig.ChainID,
		bbnConfig.Key,
		bbnConfig.KeyringBackend,
		e2eutils.Passphrase,
		e2eutils.HdPath,
		"",
	)
	require.NoError(t, err)
	return fpBbnKeyInfo
}

func fundBBNAddr(t *testing.T, bh *e2eutils.BabylonNodeHandler, fpBbnKeyInfo *types.ChainKeyInfo) {
	t.Logf(log.Prefix("Funding %dubbn to %s"), bbnAddrTopUpAmount, fpBbnKeyInfo.AccAddress.String())
	err := bh.BabylonNode.TxBankSend(
		fpBbnKeyInfo.AccAddress.String(),
		fmt.Sprintf("%dubbn", bbnAddrTopUpAmount),
	)
	require.NoError(t, err)

	// check balance
	require.Eventually(t, func() bool {
		balance, err := bh.BabylonNode.CheckAddrBalance(fpBbnKeyInfo.AccAddress.String())
		if err != nil {
			t.Logf("Error checking balance: %v", err)
			return false
		}
		return balance == bbnAddrTopUpAmount
	}, 30*time.Second, 2*time.Second, fmt.Sprintf("failed to top up %s", fpBbnKeyInfo.AccAddress.String()))
	t.Logf(log.Prefix("Sent %dubbn to %s"), bbnAddrTopUpAmount, fpBbnKeyInfo.AccAddress.String())
}

func mockOpL2ConsumerCtrlConfig(bbnNodeDir string) *fpcfg.OPStackL2Config {
	dc := bbncfg.DefaultBabylonConfig()

	// fill up the config from dc config
	return &fpcfg.OPStackL2Config{
		Key:            dc.Key,
		ChainID:        dc.ChainID,
		RPCAddr:        dc.RPCAddr,
		GRPCAddr:       dc.GRPCAddr,
		AccountPrefix:  dc.AccountPrefix,
		KeyringBackend: dc.KeyringBackend,
		KeyDirectory:   bbnNodeDir,
		GasAdjustment:  1.5,
		GasPrices:      "0.002ubbn",
		Debug:          dc.Debug,
		Timeout:        dc.Timeout,
		// Setting this to relatively low value, out currnet babylon client (lens) will
		// block for this amout of time to wait for transaction inclusion in block
		BlockTimeout: 1 * time.Minute,
		OutputFormat: dc.OutputFormat,
		SignModeStr:  dc.SignModeStr,
	}
}

func deployCwContract(
	t *testing.T,
	opConsumerId string,
	cwClient *cwclient.Client,
) string {
	// store op-finality-gadget contract
	err := cwClient.StoreWasmCode(opFinalityGadgetContractPath)
	require.NoError(t, err)

	var codeId uint64
	require.Eventually(t, func() bool {
		codeId, _ = cwClient.GetLatestCodeId()
		return codeId > 0
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
	require.Equal(t, uint64(1), codeId, "first deployed contract code_id should be 1")

	// instantiate op contract with FG disabled
	opFinalityGadgetInitMsg := map[string]interface{}{
		"admin":       cwClient.MustGetAddr(),
		"consumer_id": opConsumerId,
		"is_enabled":  false,
	}
	opFinalityGadgetInitMsgBytes, err := json.Marshal(opFinalityGadgetInitMsg)
	require.NoError(t, err)
	err = cwClient.InstantiateContract(codeId, opFinalityGadgetInitMsgBytes)
	require.NoError(t, err)

	var listContractsResponse *wasmtypes.QueryContractsByCodeResponse
	require.Eventually(t, func() bool {
		listContractsResponse, err = cwClient.ListContractsByCode(
			codeId,
			&sdkquerytypes.PageRequest{},
		)
		return err == nil
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
	require.Len(t, listContractsResponse.Contracts, 1)
	return listContractsResponse.Contracts[0]
}

func startFinalityGadgetServer(
	t *testing.T,
	logger *zap.Logger,
	opL2ConsumerConfig *fpcfg.OPStackL2Config,
	shutdownInterceptor signal.Interceptor,
) *finalitygadget.FinalityGadget {
	// define finality gadget config
	fgCfg := fgcfg.Config{
		L2RPCHost:         opL2ConsumerConfig.OPStackL2RPCAddress,
		BitcoinRPCHost:    "mock-btc-client",
		FGContractAddress: opL2ConsumerConfig.OPFinalityGadgetAddress,
		BBNChainID:        e2eutils.ChainID,
		BBNRPCAddress:     opL2ConsumerConfig.RPCAddr,
		DBFilePath:        fgDbFilePath,
		GRPCListener:      finalityGadgetRpc,
		HTTPListener:      finalityGadgetHttp,
		PollInterval:      time.Second * time.Duration(10),
	}

	// Init local DB for storing and querying blocks
	fgDb, err := fgdb.NewBBoltHandler(fgCfg.DBFilePath, logger)
	require.NoError(t, err)
	err = fgDb.CreateInitialSchema()
	require.NoError(t, err)

	// create finality gadget
	fg, err := finalitygadget.NewFinalityGadget(&fgCfg, fgDb, logger)
	require.NoError(t, err)

	// start finality gadget server
	srv := fgsrv.NewFinalityGadgetServer(&fgCfg, fgDb, fg, shutdownInterceptor, logger)
	go func() {
		err = srv.RunUntilShutdown()
		require.NoError(t, err)
	}()

	// start finality gadget
	// err = fg.Startup(context.Background())
	// require.NoError(t, err)

	return fg
}

func createAndRegisterFinalityProvider(t *testing.T, fpApp *service.FinalityProviderApp, consumerID string) *bbntypes.BIP340PubKey {
	fpCfg := fpApp.GetConfig()
	keyName := fpCfg.BabylonConfig.Key
	moniker := fmt.Sprintf("%s-%s", consumerID, e2eutils.MonikerPrefix)
	commission := sdkmath.LegacyZeroDec()
	desc := e2eutils.NewDescription(moniker)

	res, err := fpApp.CreateFinalityProvider(
		keyName,
		consumerID,
		e2eutils.Passphrase,
		e2eutils.HdPath,
		nil,
		desc,
		&commission,
	)
	require.NoError(t, err)

	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(res.FpInfo.BtcPkHex)
	require.NoError(t, err)

	_, err = fpApp.RegisterFinalityProvider(fpPk.MarshalHex())
	require.NoError(t, err)
	return fpPk
}

// A BTC delegation has to stake to at least one Babylon finality provider
// https://github.com/babylonlabs-io/babylon-private/blob/74a24c962ce2cf64e5216edba9383fe0b460070c/x/btcstaking/keeper/msg_server.go#L220
func (ctm *OpL2ConsumerTestManager) setupFinalityProviders(t *testing.T) {
	// create and register Babylon FP
	babylonFpPk := createAndRegisterFinalityProvider(t, ctm.BabylonFpApp, e2eutils.ChainID)
	t.Logf(log.Prefix("Registered Finality Provider %s for %s"), babylonFpPk.MarshalHex(), e2eutils.ChainID)

	// wait for Babylon FP registration
	ctm.waitForBabylonFPRegistration(t)

	// create and register consumer FP
	consumerFpPk := createAndRegisterFinalityProvider(t, ctm.ConsumerFpApp, getConsumerChainId(&ctm.OpSystem.Cfg))
	t.Logf(log.Prefix("Registered Finality Provider %s for %s"), consumerFpPk.MarshalHex(), getConsumerChainId(&ctm.OpSystem.Cfg))

	// wait for consumer FP registration
	ctm.waitForConsumerFPRegistration(t)
}

func (ctm *OpL2ConsumerTestManager) waitForBabylonFPRegistration(t *testing.T) {
	require.Eventually(t, func() bool {
		_, err := ctm.BBNClient.QueryFinalityProviders()
		return err == nil
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime, "Failed to wait for Babylon FP registration")
}

func (ctm *OpL2ConsumerTestManager) waitForConsumerFPRegistration(t *testing.T) {
	require.Eventually(t, func() bool {
		_, err := ctm.BBNClient.QueryConsumerFinalityProviders(getConsumerChainId(&ctm.OpSystem.Cfg))
		return err == nil
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime, "Failed to wait for consumer FP registration")
}

func (ctm *OpL2ConsumerTestManager) Stop(t *testing.T) {
	t.Log("Stopping test manager")
	var err error
	// FpApp has to stop first or you will get "rpc error: desc = account xxx not found: key not found" error
	// b/c when Babylon daemon is stopped, FP won't be able to find the keyring backend
	err = ctm.BabylonFpApp.Stop()
	require.NoError(t, err)
	t.Log(log.Prefix("Stopped Babylon FP App"))

	err = ctm.ConsumerFpApp.Stop()
	require.NoError(t, err)
	t.Log(log.Prefix("Stopped Consumer FP App"))

	ctm.FinalityGadget.Close()
	os.Remove(fgDbFilePath)

	err = ctm.BabylonHandler.Stop()
	require.NoError(t, err)

	ctm.EOTSServerHandler.Stop()

	err = os.RemoveAll(ctm.BaseDir)
	require.NoError(t, err)
}

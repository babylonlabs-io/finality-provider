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

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	bbncfg "github.com/babylonlabs-io/babylon/client/config"
	opcc "github.com/babylonlabs-io/finality-provider/clientcontroller/opstackl2"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	ope2e "github.com/ethereum-optimism/optimism/op-e2e"
	optestlog "github.com/ethereum-optimism/optimism/op-service/testlog"
	gethlog "github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	bbncc "github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	eotsconfig "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	base_test_manager "github.com/babylonlabs-io/finality-provider/itest/test-manager"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/testutil/log"
)

const (
	opFinalityGadgetContractPath = "../bytecode/op_finality_gadget_16f6154.wasm"
	consumerChainIdPrefix        = "op-stack-l2-"
	bbnAddrTopUpAmount           = 100000000
)

type BaseTestManager = base_test_manager.BaseTestManager

type OpL2ConsumerTestManager struct {
	BaseTestManager
	BaseDir           string
	OpSystem          *ope2e.System
	BabylonHandler    *e2eutils.BabylonNodeHandler
	EOTSServerHandler *e2eutils.EOTSServerHandler
	BabylonFpApp      *service.FinalityProviderApp
}

// - setup OP consumer chain
// - setup Babylon finality system
//   - start Babylon node
//   - start Babylon FP
//   - register consumer chain to Babylon
//   - deploy finality gadget cw contract
//
// ......
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

	// create EOTS server handler and EOTS client
	eotsHandler, babylonEOTSClient := startBabylonEotsManager(t, logger, testDir, babylonFpCfg)

	// create and start Babylon FP app
	babylonFpApp := createAndStartBabylonFpApp(t, logger, babylonFpCfg, babylonEOTSClient)

	// register consumer chain to Babylon
	_, err = babylonController.RegisterConsumerChain(
		opConsumerId,
		"OP consumer chain (test)",
		"some description about the chain",
	)
	require.NoError(t, err)
	t.Logf(log.Prefix("Register consumer %s to Babylon"), opConsumerId)

	// deploy finality gadget cw contract
	opFinalityGadgetAddress := deployCwContract(t, logger, opConsumerId, babylonHandler)
	t.Logf(log.Prefix("op-finality-gadget contract address: %s"), opFinalityGadgetAddress)

	ctm := &OpL2ConsumerTestManager{
		BaseTestManager:   BaseTestManager{BBNClient: babylonController, CovenantPrivKeys: covenantPrivKeys},
		BaseDir:           testDir,
		OpSystem:          opSys,
		BabylonHandler:    babylonHandler,
		EOTSServerHandler: eotsHandler,
		BabylonFpApp:      babylonFpApp,
	}

	return ctm
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

func createAndStartBabylonFpApp(
	t *testing.T,
	logger *zap.Logger,
	cfg *fpcfg.Config,
	eotsCli *client.EOTSManagerGRpcClient,
) *service.FinalityProviderApp {
	cc, err := bbncc.NewBabylonConsumerController(cfg.BabylonConfig, &cfg.BTCNetParams, logger)
	require.NoError(t, err)

	fpApp := createAndStartFpApp(t, logger, cfg, cc, eotsCli)
	t.Log(log.Prefix("Started Babylon FP App"))
	return fpApp
}

func startBabylonEotsManager(
	t *testing.T,
	logger *zap.Logger,
	testDir string,
	fpCfg *fpcfg.Config,
) (*e2eutils.EOTSServerHandler, *client.EOTSManagerGRpcClient) {
	eotsHomeDir := filepath.Join(testDir, "babylon-eots-home")
	eotsCfg := eotsconfig.DefaultConfigWithHomePathAndPorts(
		eotsHomeDir,
		eotsconfig.DefaultRPCPort,
		metrics.DefaultEotsConfig().Port,
	)

	eh := e2eutils.NewEOTSServerHandler(t, logger, eotsCfg, eotsHomeDir)
	eh.Start()

	// wait for EOTS servers to start
	// see https://github.com/babylonchain/finality-provider/pull/517
	var eotsCli *client.EOTSManagerGRpcClient
	var err error
	require.Eventually(t, func() bool {
		eotsCli, err = client.NewEOTSManagerGRpcClient(fpCfg.EOTSManagerAddress)
		return err == nil
	}, 5*time.Second, time.Second, "Failed to create EOTS client")

	return eh, eotsCli
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
	logger *zap.Logger,
	opConsumerId string,
	bh *e2eutils.BabylonNodeHandler,
) string {
	// deploy cw contract with babylon node keyring
	opL2ConsumerConfig := mockOpL2ConsumerCtrlConfig(bh.GetNodeDataDir())
	cwConfig := opL2ConsumerConfig.ToCosmwasmConfig()
	cwClient, err := opcc.NewCwClient(&cwConfig, logger)
	require.NoError(t, err)

	// store op-finality-gadget contract
	err = cwClient.StoreWasmCode(opFinalityGadgetContractPath)
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

func (ctm *OpL2ConsumerTestManager) Stop(t *testing.T) {
	t.Log("Stopping test manager")
	var err error
	// FpApp has to stop first or you will get "rpc error: desc = account xxx not found: key not found" error
	// b/c when Babylon daemon is stopped, FP won't be able to find the keyring backend
	err = ctm.BabylonFpApp.Stop()
	require.NoError(t, err)
	t.Log(log.Prefix("Stopped Babylon FP App"))

	ctm.OpSystem.Close()
	err = ctm.BabylonHandler.Stop()
	require.NoError(t, err)

	err = os.RemoveAll(ctm.BaseDir)
	require.NoError(t, err)
}

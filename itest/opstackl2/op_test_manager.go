//go:build e2e_op
// +build e2e_op

package e2etest_op

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	bbncc "github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	opcc "github.com/babylonlabs-io/finality-provider/clientcontroller/opstackl2"
	cwclient "github.com/babylonlabs-io/finality-provider/cosmwasmclient/client"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	eotsconfig "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	"github.com/babylonlabs-io/finality-provider/itest/container"
	base_test_manager "github.com/babylonlabs-io/finality-provider/itest/test-manager"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/babylonlabs-io/finality-provider/testutil/log"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/lightningnetwork/lnd/signal"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	opConsumerChainId            = "op-stack-l2-706114"
	bbnAddrTopUpAmount           = 100000000
	eventuallyWaitTimeOut        = 5 * time.Minute
	eventuallyPollTime           = 500 * time.Millisecond
	passphrase                   = "testpass"
	hdPath                       = ""
	opFinalityGadgetContractPath = "../bytecode/op_finality_gadget_16f6154.wasm"
)

type BaseTestManager = base_test_manager.BaseTestManager

type OpL2ConsumerTestManager struct {
	BaseTestManager
	BaseDir              string
	manager              *container.Manager
	OpConsumerController *opcc.OPStackL2ConsumerController
	EOTSServerHandler    *e2eutils.EOTSServerHandler
	BabylonFpApp         *service.FinalityProviderApp
	ConsumerFpApp        *service.FinalityProviderApp
	ConsumerEOTSClient   *client.EOTSManagerGRpcClient
	logger               *zap.Logger
}

// StartOpL2ConsumerManager
// - starts Babylon node and wait for it starts
// - deploys finality gadget cw contract
// - creates and starts Babylon and consumer FPs without any FP instances
func StartOpL2ConsumerManager(t *testing.T) *OpL2ConsumerTestManager {
	// Setup base dir and logger
	testDir, err := base_test_manager.TempDir(t, "op-fp-e2e-test-*")
	require.NoError(t, err)

	// setup logger
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.Level(zap.DebugLevel))
	logger, err := config.Build()
	require.NoError(t, err)

	// start Babylon node
	manager, covenantPrivKeys := startBabylonNode(t)

	// wait for Babylon node starts b/c we will fund the FP address with babylon node
	babylonController, _ := waitForBabylonNodeStart(t, testDir, logger, manager)

	// register consumer chain to Babylon
	_, err = babylonController.RegisterConsumerChain(
		opConsumerChainId,
		"OP consumer chain",
		"Some description about the chain",
	)
	require.NoError(t, err)
	t.Logf(log.Prefix("Register consumer %s to Babylon"), opConsumerChainId)

	// create cosmwasm client
	consumerFpCfg, opConsumerCfg := createConsumerFpConfig(t, testDir, manager)
	cwConfig := opConsumerCfg.ToCosmwasmConfig()
	cwClient, err := opcc.NewCwClient(&cwConfig, logger)
	require.NoError(t, err)

	// deploy finality gadget cw contract
	opFinalityGadgetAddress := deployCwContract(t, cwClient)
	t.Logf(log.Prefix("op-finality-gadget contract address: %s"), opFinalityGadgetAddress)

	// update opConsumerCfg with opFinalityGadgetAddress
	opConsumerCfg.OPFinalityGadgetAddress = opFinalityGadgetAddress

	// update consumer FP config with opConsumerCfg
	consumerFpCfg.OPStackL2Config = opConsumerCfg

	// create Babylon FP config
	babylonFpCfg := createBabylonFpConfig(t, testDir, manager)

	// create shutdown interceptor
	shutdownInterceptor, err := signal.Intercept()
	require.NoError(t, err)

	// create EOTS handler and EOTS gRPC clients for Babylon and consumer
	eotsHandler, EOTSClients := base_test_manager.StartEotsManagers(t, logger, testDir, babylonFpCfg, consumerFpCfg, &shutdownInterceptor)

	// create Babylon consumer controller
	babylonConsumerController, err := bbncc.NewBabylonConsumerController(babylonFpCfg.BabylonConfig, &babylonFpCfg.BTCNetParams, logger)
	require.NoError(t, err)

	// create and start Babylon FP app
	babylonFpApp := base_test_manager.CreateAndStartFpApp(t, logger, babylonFpCfg, babylonConsumerController, EOTSClients[0])
	t.Log(log.Prefix("Started Babylon FP App"))

	// create op consumer controller
	opConsumerController, err := opcc.NewOPStackL2ConsumerController(opConsumerCfg, logger)
	require.NoError(t, err)

	// create and start consumer FP app
	consumerFpApp := base_test_manager.CreateAndStartFpApp(t, logger, consumerFpCfg, opConsumerController, EOTSClients[1])
	t.Log(log.Prefix("Started Consumer FP App"))

	ctm := &OpL2ConsumerTestManager{
		BaseTestManager: BaseTestManager{
			BBNClient:        babylonController,
			CovenantPrivKeys: covenantPrivKeys,
		},
		BaseDir:              testDir,
		manager:              manager,
		OpConsumerController: opConsumerController,
		EOTSServerHandler:    eotsHandler,
		BabylonFpApp:         babylonFpApp,
		ConsumerFpApp:        consumerFpApp,
		ConsumerEOTSClient:   EOTSClients[1],
		logger:               logger,
	}

	return ctm
}

func startBabylonNode(t *testing.T) (*container.Manager, []*secp256k1.PrivateKey) {
	// generate covenant committee
	covenantQuorum := 2
	numCovenants := 3
	covenantPrivKeys, covenantPubKeys := e2eutils.GenerateCovenantCommittee(numCovenants, t)

	// Create container manager
	manager, err := container.NewManager(t)
	require.NoError(t, err)

	// Create temp dir for babylon node
	babylonDir, err := base_test_manager.TempDir(t, "babylon-test-*")
	require.NoError(t, err)

	// Start babylon node in docker
	_, err = manager.RunBabylondResource(t, babylonDir, covenantQuorum, covenantPubKeys)
	require.NoError(t, err)

	return manager, covenantPrivKeys
}

func waitForBabylonNodeStart(
	t *testing.T,
	testDir string,
	logger *zap.Logger,
	manager *container.Manager,
) (*bbncc.BabylonController, *types.StakingParams) {
	// create Babylon FP config
	babylonFpCfg := createBabylonFpConfig(t, testDir, manager)

	// create Babylon controller
	babylonController, err := bbncc.NewBabylonController(babylonFpCfg.BabylonConfig, &babylonFpCfg.BTCNetParams, logger)
	require.NoError(t, err)

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

	t.Logf("Babylon node is started, chain_id: %s", babylonController.GetBBNClient().GetConfig().ChainID)
	return babylonController, stakingParams
}

func createBabylonFpConfig(
	t *testing.T,
	testDir string,
	manager *container.Manager,
) *fpcfg.Config {
	fpHomeDir := filepath.Join(testDir, "babylon-fp-home")
	t.Logf(log.Prefix("Babylon FP home dir: %s"), fpHomeDir)

	// Get dynamically allocated ports from docker
	babylonDir, err := base_test_manager.TempDir(t, "babylon-test-*")
	require.NoError(t, err)

	// Start babylond if not already started
	babylond, err := manager.RunBabylondResource(t, babylonDir, 2, nil)
	require.NoError(t, err)
	require.NotNil(t, babylond)

	cfg := e2eutils.DefaultFpConfigWithPorts(
		"/home/node0/babylond", // This is the path inside docker container
		fpHomeDir,
		fpcfg.DefaultRPCPort,
		metrics.DefaultFpConfig().Port,
		eotsconfig.DefaultRPCPort,
	)

	// Update ports with dynamically allocated ones from docker
	cfg.BabylonConfig.RPCAddr = fmt.Sprintf("http://localhost:%s", babylond.GetPort("26657/tcp"))
	cfg.BabylonConfig.GRPCAddr = fmt.Sprintf("localhost:%s", babylond.GetPort("9090/tcp"))

	return cfg
}

func createConsumerFpConfig(
	t *testing.T,
	testDir string,
	manager *container.Manager,
) (*fpcfg.Config, *fpcfg.OPStackL2Config) {
	fpHomeDir := filepath.Join(testDir, "consumer-fp-home")
	t.Logf(log.Prefix("Consumer FP home dir: %s"), fpHomeDir)

	// Get dynamically allocated ports from docker
	babylonDir, err := base_test_manager.TempDir(t, "babylon-test-*")
	require.NoError(t, err)

	// Start babylond if not already started
	babylond, err := manager.RunBabylondResource(t, babylonDir, 2, nil)
	require.NoError(t, err)
	require.NotNil(t, babylond)

	cfg := e2eutils.DefaultFpConfigWithPorts(
		"/home/node0/babylond", // This is the path inside docker container
		fpHomeDir,
		fpcfg.DefaultRPCPort,
		metrics.DefaultFpConfig().Port,
		eotsconfig.DefaultRPCPort,
	)

	// Update ports with dynamically allocated ones from docker
	cfg.BabylonConfig.RPCAddr = fmt.Sprintf("http://localhost:%s", babylond.GetPort("26657/tcp"))
	cfg.BabylonConfig.GRPCAddr = fmt.Sprintf("localhost:%s", babylond.GetPort("9090/tcp"))

	// create consumer FP key/address
	fpBbnKeyInfo, err := testutil.CreateChainKey(
		cfg.BabylonConfig.KeyDirectory,
		cfg.BabylonConfig.ChainID,
		cfg.BabylonConfig.Key,
		cfg.BabylonConfig.KeyringBackend,
		passphrase,
		hdPath,
		"",
	)
	require.NoError(t, err)

	// fund the consumer FP address
	t.Logf(log.Prefix("Funding %dubbn to %s"), bbnAddrTopUpAmount, fpBbnKeyInfo.AccAddress.String())
	_, _, err = manager.BabylondTxBankSend(t, fpBbnKeyInfo.AccAddress.String(), fmt.Sprintf("%dubbn", bbnAddrTopUpAmount), "node0")
	require.NoError(t, err)

	// check consumer FP address balance
	require.Eventually(t, func() bool {
		out, _, err := manager.ExecBabylondCmd(t, []string{"query", "bank", "balances", fpBbnKeyInfo.AccAddress.String(), "--output=json"})
		if err != nil {
			return false
		}
		var balances struct {
			Balances []struct {
				Denom  string `json:"denom"`
				Amount string `json:"amount"`
			} `json:"balances"`
		}
		if err := json.Unmarshal(out.Bytes(), &balances); err != nil {
			return false
		}
		for _, bal := range balances.Balances {
			if bal.Denom == "ubbn" && bal.Amount == fmt.Sprintf("%d", bbnAddrTopUpAmount) {
				return true
			}
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// create consumer FP config
	opCfg := &fpcfg.OPStackL2Config{
		ChainID:                  opConsumerChainId,
		OPStackL2RPCAddress:      "http://localhost:8545",
		BabylonFinalityGadgetRpc: "http://localhost:8547",
	}

	return cfg, opCfg
}

func deployCwContract(t *testing.T, cwClient *cwclient.Client) string {
	// store op-finality-gadget contract
	err := cwClient.StoreWasmCode(opFinalityGadgetContractPath)
	require.NoError(t, err)

	var codeId uint64
	require.Eventually(t, func() bool {
		codeId, _ = cwClient.GetLatestCodeID()
		return codeId > 0
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
	require.Equal(t, uint64(1), codeId, "first deployed contract code_id should be 1")

	// instantiate op contract with FG disabled
	opFinalityGadgetInitMsg := map[string]interface{}{
		"admin":       cwClient.MustGetAddr(),
		"consumer_id": opConsumerChainId,
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

func (ctm *OpL2ConsumerTestManager) setupBabylonAndConsumerFp(t *testing.T) []*bbntypes.BIP340PubKey {
	// create and register Babylon FP
	babylonFpPk := base_test_manager.CreateAndRegisterFinalityProvider(t, ctm.BabylonFpApp, e2eutils.ChainID)
	t.Logf(log.Prefix("Registered Finality Provider %s for %s"), babylonFpPk.MarshalHex(), e2eutils.ChainID)

	// wait for Babylon FP registration
	require.Eventually(t, func() bool {
		fps, err := ctm.BBNClient.QueryFinalityProviders()
		return err == nil && len(fps) > 0
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime, "Failed to wait for Babylon FP registration")

	// create and register consumer FP
	consumerFpPk := base_test_manager.CreateAndRegisterFinalityProvider(t, ctm.ConsumerFpApp, opConsumerChainId)
	t.Logf(log.Prefix("Registered Finality Provider %s for %s"), consumerFpPk.MarshalHex(), opConsumerChainId)

	// wait for consumer FP registration
	require.Eventually(t, func() bool {
		fps, err := ctm.BBNClient.QueryFinalityProviders()
		return err == nil && len(fps) > 1
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime, "Failed to wait for consumer FP registration")

	return []*bbntypes.BIP340PubKey{babylonFpPk, consumerFpPk}
}

func (ctm *OpL2ConsumerTestManager) getConsumerFpInstance(
	t *testing.T,
	consumerFpPk *bbntypes.BIP340PubKey,
) *service.FinalityProviderInstance {
	fpCfg := ctm.ConsumerFpApp.GetConfig()
	fpStore := ctm.ConsumerFpApp.GetFinalityProviderStore()
	pubRandStore := ctm.ConsumerFpApp.GetPubRandProofStore()
	bc := ctm.BabylonFpApp.GetBabylonController()

	fpInstance, err := service.NewFinalityProviderInstance(
		consumerFpPk, fpCfg, fpStore, pubRandStore, bc, ctm.OpConsumerController, ctm.ConsumerEOTSClient,
		metrics.NewFpMetrics(), "", make(chan<- *service.CriticalError), ctm.logger)
	require.NoError(t, err)
	return fpInstance
}

func (ctm *OpL2ConsumerTestManager) delegateBTCAndWaitForActivation(t *testing.T, babylonFpPk *bbntypes.BIP340PubKey, consumerFpPk *bbntypes.BIP340PubKey) {
	// send a BTC delegation
	ctm.InsertBTCDelegation(t, []*btcec.PublicKey{babylonFpPk.MustToBTCPK(), consumerFpPk.MustToBTCPK()},
		e2eutils.StakingTime, e2eutils.StakingAmount)

	// check the BTC delegation is pending
	delsResp := ctm.WaitForNPendingDels(t, 1)
	del, err := e2eutils.ParseRespBTCDelToBTCDel(delsResp[0])
	require.NoError(t, err)

	// send covenant sigs
	ctm.InsertCovenantSigForDelegation(t, del)

	// check the BTC delegation is active
	ctm.WaitForNActiveDels(t, 1)
}

func (ctm *OpL2ConsumerTestManager) queryCwContract(
	t *testing.T,
	queryMsg map[string]interface{},
) *wasmtypes.QuerySmartContractStateResponse {
	// create cosmwasm client
	cwConfig := ctm.OpConsumerController.Cfg.ToCosmwasmConfig()
	cwClient, err := opcc.NewCwClient(&cwConfig, ctm.logger)
	require.NoError(t, err)

	// marshal query message
	queryMsgBytes, err := json.Marshal(queryMsg)
	require.NoError(t, err)

	var queryResponse *wasmtypes.QuerySmartContractStateResponse
	require.Eventually(t, func() bool {
		queryResponse, err = cwClient.QuerySmartContractState(
			ctm.OpConsumerController.Cfg.OPFinalityGadgetAddress,
			string(queryMsgBytes),
		)
		return err == nil
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

	return queryResponse
}

func (ctm *OpL2ConsumerTestManager) Stop(t *testing.T) {
	t.Log("Stopping test manager")
	var err error
	err = ctm.BabylonFpApp.Stop()
	require.NoError(t, err)
	err = ctm.ConsumerFpApp.Stop()
	require.NoError(t, err)
	err = ctm.manager.ClearResources()
	require.NoError(t, err)
	err = os.RemoveAll(ctm.BaseDir)
	require.NoError(t, err)
}

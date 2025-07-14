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
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	"github.com/btcsuite/btcd/btcec/v2"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	rollupfpcontroller "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/clientcontroller"
	rollupfpconfig "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/config"
	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	bbncc "github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
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
)

const (
	rollupBSNID                = "op-stack-l2-706114"
	bbnAddrTopUpAmount         = 100000000
	eventuallyWaitTimeOut      = 5 * time.Minute
	eventuallyPollTime         = 500 * time.Millisecond
	passphrase                 = "testpass"
	hdPath                     = ""
	rollupFinalityContractPath = "../bytecode/rollup/finality.wasm"
)

type BaseTestManager = base_test_manager.BaseTestManager

type OpL2ConsumerTestManager struct {
	BaseTestManager
	BaseDir              string
	manager              *container.Manager
	OpConsumerController *rollupfpcontroller.OPStackL2ConsumerController
	EOTSServerHandler    *e2eutils.EOTSServerHandler
	BabylonFpApp         *service.FinalityProviderApp
	ConsumerFpApp        *service.FinalityProviderApp
	BabylonEOTSClient    *client.EOTSManagerGRpcClient
	ConsumerEOTSClient   *client.EOTSManagerGRpcClient
	logger               *zap.Logger
}

// StartOpL2ConsumerManager
// - starts Babylon node and wait for it starts
// - deploys finality gadget cw contract
// - creates and starts Babylon and consumer FPs without any FP instances
func StartOpL2ConsumerManager(t *testing.T, ctx context.Context) *OpL2ConsumerTestManager {
	// Setup base dir and logger
	testDir, err := base_test_manager.TempDir(t, "op-fp-e2e-test-*")
	require.NoError(t, err)

	// setup logger
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.Level(zap.DebugLevel))
	logger, err := config.Build()
	require.NoError(t, err)

	// start Babylon node
	manager, babylond, covenantPrivKeys, keyDir := startBabylonNode(t)

	// wait for Babylon node starts b/c we will fund the FP address with babylon node
	babylonController, _ := waitForBabylonNodeStart(t, keyDir, testDir, logger, manager, babylond)

	// create cosmwasm client
	consumerFpCfg, opConsumerCfg := createConsumerFpConfig(t, testDir, manager, babylond)
	rollupController, err := rollupfpcontroller.NewOPStackL2ConsumerController(opConsumerCfg, logger)
	require.NoError(t, err)

	// deploy finality gadget cw contract
	opFinalityGadgetAddress := deployCwContract(t, rollupController, ctx)
	t.Logf(log.Prefix("rollup BSN finality contract address: %s"), opFinalityGadgetAddress)

	// register consumer chain to Babylon
	_, err = babylonController.RegisterConsumerChain(
		rollupBSNID,
		"OP stack rollup BSN",
		"Some description about the chain",
		opFinalityGadgetAddress,
	)
	require.NoError(t, err)
	t.Logf(log.Prefix("Register consumer %s to Babylon"), rollupBSNID)

	// update opConsumerCfg with opFinalityGadgetAddress
	opConsumerCfg.OPFinalityGadgetAddress = opFinalityGadgetAddress

	// update consumer FP config with opConsumerCfg
	consumerFpCfg.ContextSigningHeight = ^uint64(0) // enable context signing height, max uint64 value

	// create Babylon FP config
	babylonFpCfg := createBabylonFpConfig(t, keyDir, testDir, manager, babylond)
	babylonFpCfg.ContextSigningHeight = ^uint64(0) // enable context signing height, max uint64 value

	// create EOTS handler and EOTS gRPC clients for Babylon and consumer
	eotsHandler, EOTSClients := base_test_manager.StartEotsManagers(t, ctx, logger, testDir, babylonFpCfg, consumerFpCfg)

	// create Babylon consumer controller
	babylonConsumerController, err := bbncc.NewBabylonConsumerController(babylonFpCfg.BabylonConfig, logger)
	require.NoError(t, err)

	// create and start Babylon FP app
	babylonFpApp := base_test_manager.CreateAndStartFpApp(t, logger, babylonFpCfg, babylonConsumerController, EOTSClients[0])
	t.Log(log.Prefix("Started Babylon FP App"))

	// create op consumer controller
	opConsumerController, err := rollupfpcontroller.NewOPStackL2ConsumerController(opConsumerCfg, logger)
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
		BabylonEOTSClient:    EOTSClients[0],
		ConsumerEOTSClient:   EOTSClients[1],
		logger:               logger,
	}

	return ctm
}

func startBabylonNode(t *testing.T) (*container.Manager, *dockertest.Resource, []*secp256k1.PrivateKey, string) {
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
	babylond, err := manager.RunBabylondResource(t, babylonDir, covenantQuorum, covenantPubKeys)
	require.NoError(t, err)

	keyDir := filepath.Join(babylonDir, "node0", "babylond")

	return manager, babylond, covenantPrivKeys, keyDir
}

func waitForBabylonNodeStart(
	t *testing.T,
	keyDir string,
	testDir string,
	logger *zap.Logger,
	manager *container.Manager,
	babylond *dockertest.Resource,
) (*bbncc.BabylonController, *types.StakingParams) {
	// create Babylon FP config
	babylonFpCfg := createBabylonFpConfig(t, keyDir, testDir, manager, babylond)

	// create Babylon controller
	var babylonController *bbncc.BabylonController
	require.Eventually(t, func() bool {
		var err error
		bc, err := fpcc.NewBabylonController(babylonFpCfg, logger)
		if err != nil {
			t.Logf("Failed to create Babylon controller: %v", err)
			return false
		}
		babylonController = bc.(*bbncc.BabylonController)
		return true
	}, 30*time.Second, 1*time.Second)

	var stakingParams *types.StakingParams
	// wait for Babylon node starts
	require.Eventually(t, func() bool {
		params, err := babylonController.QueryStakingParams()
		if err != nil {
			t.Logf("Failed to query staking params: %v", err)
			return false
		}
		stakingParams = params
		return true
	}, 30*time.Second, 1*time.Second)

	t.Logf("Babylon node is started, chain_id: %s", babylonController.GetBBNClient().GetConfig().ChainID)
	return babylonController, stakingParams
}

func createBabylonFpConfig(
	t *testing.T,
	keyDir string,
	testDir string,
	manager *container.Manager,
	babylond *dockertest.Resource,
) *fpcfg.Config {
	fpHomeDir := filepath.Join(testDir, "babylon-fp-home")
	t.Logf(log.Prefix("Babylon FP home dir: %s"), fpHomeDir)

	cfg := e2eutils.DefaultFpConfigWithPorts(
		keyDir, // This is the path inside docker container
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
	babylond *dockertest.Resource,
) (*fpcfg.Config, *rollupfpconfig.OPStackL2Config) {
	fpHomeDir := filepath.Join(testDir, "consumer-fp-home")
	t.Logf(log.Prefix("Consumer FP home dir: %s"), fpHomeDir)

	cfg := e2eutils.DefaultFpConfigWithPorts(
		fpHomeDir, // This is the path inside docker container
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

	// set consumer FP config
	opConsumerCfg := &rollupfpconfig.OPStackL2Config{
		// it will be updated later
		OPFinalityGadgetAddress: "",
		// it must be a dialable RPC address checked by NewOPStackL2ConsumerController
		OPStackL2RPCAddress: "https://optimism-sepolia.drpc.org",
		// the value does not matter for the test
		BabylonFinalityGadgetRpc: "127.0.0.1:50051",
		Babylon:                  cfg.BabylonConfig,
	}

	return cfg, opConsumerCfg
}

func deployCwContract(t *testing.T, cc *rollupfpcontroller.OPStackL2ConsumerController, ctx context.Context) string {
	// store op-finality-gadget contract
	err := cc.StoreWasmCode(ctx, rollupFinalityContractPath)
	require.NoError(t, err)

	var codeId uint64
	require.Eventually(t, func() bool {
		codeId, _ = cc.GetLatestCodeID(ctx)
		return codeId > 0
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
	require.Equal(t, uint64(1), codeId, "first deployed contract code_id should be 1")

	// instantiate op contract with FG disabled
	opFinalityGadgetInitMsg := map[string]interface{}{
		"admin":        cc.MustGetBabylonAddr(),
		"bsn_id":       rollupBSNID,
		"min_pub_rand": 100,
	}
	opFinalityGadgetInitMsgBytes, err := json.Marshal(opFinalityGadgetInitMsg)
	require.NoError(t, err)
	err = cc.InstantiateContract(ctx, codeId, opFinalityGadgetInitMsgBytes)
	require.NoError(t, err)

	var listContractsResponse *wasmtypes.QueryContractsByCodeResponse
	require.Eventually(t, func() bool {
		listContractsResponse, err = cc.ListContractsByCode(
			ctx,
			codeId,
			&sdkquerytypes.PageRequest{},
		)
		return err == nil
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
	require.Len(t, listContractsResponse.Contracts, 1)
	return listContractsResponse.Contracts[0]
}

func (ctm *OpL2ConsumerTestManager) setupBabylonAndConsumerFp(t *testing.T) []*bbntypes.BIP340PubKey {
	babylonCfg := ctm.BabylonFpApp.GetConfig()
	babylonKeyName := babylonCfg.BabylonConfig.Key

	// create and register Babylon FP
	babylonChainID := babylonCfg.BabylonConfig.ChainID
	eotsPk, err := ctm.EOTSServerHandler.CreateKey(babylonKeyName, "")
	require.NoError(t, err)
	babylonFpPk, err := bbntypes.NewBIP340PubKey(eotsPk)
	require.NoError(t, err)
	base_test_manager.CreateAndRegisterFinalityProvider(t, ctm.BabylonFpApp, babylonChainID, babylonFpPk)
	t.Logf(log.Prefix("Registered Finality Provider %s for %s"), babylonFpPk.MarshalHex(), babylonChainID)

	// wait for Babylon FP registration
	require.Eventually(t, func() bool {
		fps, err := ctm.BBNClient.QueryFinalityProviders()
		return err == nil && len(fps) > 0
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime, "Failed to wait for Babylon FP registration")

	consumerCfg := ctm.ConsumerFpApp.GetConfig()
	consumerKeyName := consumerCfg.BabylonConfig.Key + "2"

	// create and register consumer FP
	consumerEotsPk, err := ctm.EOTSServerHandler.CreateKey(consumerKeyName, "")
	require.NoError(t, err)
	consumerFpPk, err := bbntypes.NewBIP340PubKey(consumerEotsPk)
	require.NoError(t, err)
	base_test_manager.CreateAndRegisterFinalityProvider(t, ctm.ConsumerFpApp, rollupBSNID, consumerFpPk)
	t.Logf(log.Prefix("Registered Finality Provider %s for %s"), consumerFpPk.MarshalHex(), rollupBSNID)

	// wait for Babylon FP registration
	require.Eventually(t, func() bool {
		fps, err := ctm.BBNClient.QueryFinalityProviders()
		if err != nil {
			t.Logf("Failed to query finality providers: %v", err)
			return false
		}
		if len(fps) < 1 {
			t.Logf("Expected at least 1 Babylon finality provider, got %d", len(fps))
			return false
		}
		return true
	}, 30*time.Second, 1*time.Second, "Failed to wait for Babylon FP registration")

	// wait for consumer FP registration
	require.Eventually(t, func() bool {
		fps, err := ctm.BBNClient.QueryConsumerFinalityProviders(rollupBSNID)
		if err != nil {
			t.Logf("Failed to query finality providers: %v", err)
			return false
		}
		if len(fps) < 1 {
			t.Logf("Expected at least 1 consumer finality provider, got %d", len(fps))
			return false
		}
		return true
	}, 30*time.Second, 1*time.Second, "Failed to wait for consumer FP registration")

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

	fpMetrics := metrics.NewFpMetrics()
	poller := service.NewChainPoller(ctm.logger, fpCfg.PollerConfig, ctm.OpConsumerController, fpMetrics)

	fpInstance, err := service.NewFinalityProviderInstance(
		consumerFpPk, fpCfg, fpStore, pubRandStore, bc, ctm.OpConsumerController, ctm.ConsumerEOTSClient, poller,
		fpMetrics, make(chan<- *service.CriticalError), ctm.logger)
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
	ctx context.Context,
) *wasmtypes.QuerySmartContractStateResponse {
	// create rollup controller
	rollupController, err := rollupfpcontroller.NewOPStackL2ConsumerController(ctm.OpConsumerController.Cfg, ctm.logger)
	require.NoError(t, err)

	// marshal query message
	queryMsgBytes, err := json.Marshal(queryMsg)
	require.NoError(t, err)

	var queryResponse *wasmtypes.QuerySmartContractStateResponse
	require.Eventually(t, func() bool {
		queryResponse, err = rollupController.QuerySmartContractState(
			ctx,
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

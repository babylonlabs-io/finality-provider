//go:build e2e_rollup

package e2etest_rollup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	"github.com/btcsuite/btcd/btcec/v2"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	ckpttypes "github.com/babylonlabs-io/babylon/v3/x/checkpointing/types"
	rollupfpcontroller "github.com/babylonlabs-io/finality-provider/bsn/rollup/clientcontroller"
	rollupfpconfig "github.com/babylonlabs-io/finality-provider/bsn/rollup/config"
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
	rollupBSNID                = "rollup-bsn"
	bbnAddrTopUpAmount         = 100000000
	eventuallyWaitTimeOut      = 5 * time.Minute
	eventuallyPollTime         = 500 * time.Millisecond
	passphrase                 = "testpass"
	hdPath                     = ""
	rollupFinalityContractPath = "./bytecode/finality.wasm"
	// finalitySignatureInterval is the interval at which finality signatures are allowed
	finalitySignatureInterval = uint64(5)
)

type BaseTestManager = base_test_manager.BaseTestManager

type OpL2ConsumerTestManager struct {
	BaseTestManager
	BaseDir             string
	manager             *container.Manager
	RollupBSNController *rollupfpcontroller.RollupBSNController
	EOTSServerHandler   *e2eutils.EOTSServerHandler
	BabylonFpApp        *service.FinalityProviderApp
	ConsumerFpApp       *service.FinalityProviderApp
	BabylonEOTSClient   *client.EOTSManagerGRpcClient
	ConsumerEOTSClient  *client.EOTSManagerGRpcClient
	AnvilResource       *dockertest.Resource
	logger              *zap.Logger
}

// StartRollupTestManager
// - starts Babylon node and wait for it starts
// - deploys finality gadget cw contract
// - creates and starts Babylon and consumer FPs without any FP instances
func StartRollupTestManager(t *testing.T, ctx context.Context) *OpL2ConsumerTestManager {
	// Setup base dir and logger
	testDir, err := base_test_manager.TempDir(t, "op-fp-e2e-test-*")
	require.NoError(t, err)

	// setup logger
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.Level(zap.WarnLevel))
	logger, err := config.Build()
	require.NoError(t, err)

	// start Babylon node
	manager, babylond, covenantPrivKeys, keyDir := startBabylonNode(t)

	// start Anvil node (simulated rollup chain)
	anvil := startAnvilNode(t, manager)

	// wait for Babylon node starts b/c we will fund the FP address with babylon node
	babylonController, _ := waitForBabylonNodeStart(t, keyDir, testDir, logger, manager, babylond)

	// deploy rollup BSN finality contract
	contractAddress := deployCwContract(t, babylonController.GetBBNClient(), ctx)
	t.Logf(log.Prefix("rollup BSN finality contract address: %s"), contractAddress)

	// create cosmwasm client
	rollupFpCfg := createRollupFpConfig(t, testDir, manager, babylond, anvil)
	rollupFpCfg.FinalityContractAddress = contractAddress

	// register consumer chain to Babylon
	_, err = babylonController.RegisterConsumerChain(
		rollupBSNID,
		"rollup BSN",
		"Some description about the chain",
		contractAddress,
	)
	require.NoError(t, err)
	t.Logf(log.Prefix("Register consumer %s to Babylon"), rollupBSNID)

	rollupFpCfg.Common.ContextSigningHeight = ^uint64(0) // enable context signing height, max uint64 value

	// create Babylon FP config
	babylonFpCfg := createBabylonFpConfig(t, keyDir, testDir, manager, babylond)

	// create EOTS handler and EOTS gRPC clients for Babylon and consumer
	eotsHandler, EOTSClients := base_test_manager.StartEotsManagers(t, ctx, logger, testDir, babylonFpCfg, rollupFpCfg.Common)

	// create Babylon consumer controller
	babylonConsumerController, err := bbncc.NewBabylonConsumerController(babylonFpCfg.BabylonConfig, logger)
	require.NoError(t, err)

	// create and start Babylon FP app
	babylonFpApp := base_test_manager.CreateAndStartFpApp(t, ctx, logger, babylonFpCfg, babylonConsumerController, EOTSClients[0])
	t.Log(log.Prefix("Started Babylon FP App"))

	// create rollup BSN controller
	rollupBSNController, err := rollupfpcontroller.NewRollupBSNController(rollupFpCfg, logger)
	require.NoError(t, err)

	// create and start BSN FP app
	consumerFpApp := base_test_manager.CreateAndStartFpApp(t, ctx, logger, rollupFpCfg.Common, rollupBSNController, EOTSClients[1])
	t.Log(log.Prefix("Started BSN FP App"))

	ctm := &OpL2ConsumerTestManager{
		BaseTestManager: BaseTestManager{
			BabylonController: babylonController,
			CovenantPrivKeys:  covenantPrivKeys,
		},
		BaseDir:             testDir,
		manager:             manager,
		RollupBSNController: rollupBSNController,
		EOTSServerHandler:   eotsHandler,
		BabylonFpApp:        babylonFpApp,
		ConsumerFpApp:       consumerFpApp,
		BabylonEOTSClient:   EOTSClients[0],
		ConsumerEOTSClient:  EOTSClients[1],
		AnvilResource:       anvil,
		logger:              logger,
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

func startAnvilNode(t *testing.T, manager *container.Manager) *dockertest.Resource {
	// Start anvil node in docker
	anvil, err := manager.RunAnvilResource(t)
	require.NoError(t, err)
	t.Logf("Started Anvil node on port %s", anvil.GetPort("8545/tcp"))

	return anvil
}

func waitForBabylonNodeStart(
	t *testing.T,
	keyDir string,
	testDir string,
	logger *zap.Logger,
	manager *container.Manager,
	babylond *dockertest.Resource,
) (*bbncc.ClientWrapper, *types.StakingParams) {
	// create Babylon FP config
	babylonFpCfg := createBabylonFpConfig(t, keyDir, testDir, manager, babylond)

	// create Babylon controller
	var babylonController *bbncc.ClientWrapper
	require.Eventually(t, func() bool {
		var err error
		bc, err := fpcc.NewBabylonController(babylonFpCfg.BabylonConfig, logger)
		if err != nil {
			t.Logf("Failed to create Babylon controller: %v", err)
			return false
		}
		babylonController = bc.(*bbncc.ClientWrapper)
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
		testutil.AllocateUniquePort(t),
		eotsconfig.DefaultRPCPort,
	)

	// Update ports with dynamically allocated ones from docker
	cfg.BabylonConfig.RPCAddr = fmt.Sprintf("http://localhost:%s", babylond.GetPort("26657/tcp"))
	cfg.BabylonConfig.GRPCAddr = fmt.Sprintf("localhost:%s", babylond.GetPort("9090/tcp"))

	return cfg
}

func createRollupFpConfig(
	t *testing.T,
	testDir string,
	manager *container.Manager,
	babylond *dockertest.Resource,
	anvil *dockertest.Resource,
) *rollupfpconfig.RollupFPConfig {
	fpHomeDir := filepath.Join(testDir, "consumer-fp-home")
	t.Logf(log.Prefix("Consumer FP home dir: %s"), fpHomeDir)

	cfg := e2eutils.DefaultFpConfigWithPorts(
		fpHomeDir, // This is the path inside docker container
		fpHomeDir,
		fpcfg.DefaultRPCPort,
		testutil.AllocateUniquePort(t),
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
	opConsumerCfg := &rollupfpconfig.RollupFPConfig{
		// it will be updated later
		FinalityContractAddress: "",
		// use local anvil node with dynamically allocated port
		RollupNodeRPCAddress: fmt.Sprintf("http://localhost:%s", anvil.GetPort("8545/tcp")),
		Common:               cfg,
	}

	t.Logf(log.Prefix("Using Anvil RPC address: %s"), opConsumerCfg.RollupNodeRPCAddress)

	return opConsumerCfg
}

func deployCwContract(t *testing.T, bbnClient *bbnclient.Client, ctx context.Context) string {
	// store rollup BSN finality contract
	err := StoreWasmCode(ctx, bbnClient, rollupFinalityContractPath)
	require.NoError(t, err)

	var codeId uint64
	require.Eventually(t, func() bool {
		codeId, _ = GetLatestCodeID(ctx, bbnClient)
		return codeId > 0
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
	require.Equal(t, uint64(1), codeId, "first deployed contract code_id should be 1")

	// instantiate contract with all required fields for the new InstantiateMsg
	opFinalityGadgetInitMsg := map[string]interface{}{
		"admin":                       bbnClient.MustGetAddr(),
		"bsn_id":                      rollupBSNID,
		"min_pub_rand":                100,
		"rate_limiting_interval":      10000,                     // test value
		"max_msgs_per_interval":       100,                       // test value
		"bsn_activation_height":       0,                         // immediate activation for tests
		"finality_signature_interval": finalitySignatureInterval, // allow signatures every block for tests
		"allowed_finality_providers":  nil,                       // allow none by default
	}
	opFinalityGadgetInitMsgBytes, err := json.Marshal(opFinalityGadgetInitMsg)
	require.NoError(t, err)
	err = InstantiateContract(bbnClient, ctx, codeId, opFinalityGadgetInitMsgBytes)
	require.NoError(t, err)

	var listContractsResponse *wasmtypes.QueryContractsByCodeResponse
	require.Eventually(t, func() bool {
		listContractsResponse, err = ListContractsByCode(
			ctx,
			bbnClient,
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
		fps, err := ctm.BabylonController.QueryFinalityProviders()
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
		fps, err := ctm.BabylonController.QueryFinalityProviders()
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
		fps, err := ctm.BabylonController.QueryConsumerFinalityProviders(rollupBSNID)
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

func (ctm *OpL2ConsumerTestManager) addFpToContractAllowlist(t *testing.T, ctx context.Context, fpPk *bbntypes.BIP340PubKey) {
	// Create the AddToAllowlist message following the same pattern as local deployment
	addToAllowlistMsg := map[string]interface{}{
		"add_to_allowlist": map[string]interface{}{
			"fp_pubkey_hex_list": []string{fpPk.MarshalHex()},
		},
	}

	addToAllowlistMsgBytes, err := json.Marshal(addToAllowlistMsg)
	require.NoError(t, err)

	// Execute the contract to add FP to allowlist
	bbnClient := ctm.BabylonController.GetBBNClient()
	executeMsg := &wasmtypes.MsgExecuteContract{
		Sender:   bbnClient.MustGetAddr(),
		Contract: ctm.RollupBSNController.Cfg.FinalityContractAddress,
		Msg:      addToAllowlistMsgBytes,
		Funds:    nil,
	}

	_, err = bbnClient.ReliablySendMsg(ctx, executeMsg, nil, nil)
	require.NoError(t, err)

	t.Logf(log.Prefix("Added FP %s to contract allowlist"), fpPk.MarshalHex())

	// Verify FP is now in allowlist
	require.Eventually(t, func() bool {
		queryMsg := map[string]interface{}{
			"allowed_finality_providers": map[string]interface{}{},
		}
		queryMsgBytes, err := json.Marshal(queryMsg)
		if err != nil {
			return false
		}

		queryResponse, err := ctm.RollupBSNController.QuerySmartContractState(
			ctx,
			ctm.RollupBSNController.Cfg.FinalityContractAddress,
			string(queryMsgBytes),
		)
		if err != nil {
			return false
		}

		var allowedFPs []string
		err = json.Unmarshal(queryResponse.Data, &allowedFPs)
		if err != nil {
			return false
		}

		// Check if our FP is in the list
		for _, allowedFP := range allowedFPs {
			if allowedFP == fpPk.MarshalHex() {
				return true
			}
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime, "FP should be added to contract allowlist")
}

func (ctm *OpL2ConsumerTestManager) getConsumerFpInstance(
	t *testing.T,
	consumerFpPk *bbntypes.BIP340PubKey,
) *service.FinalityProviderInstance {
	fpCfg := ctm.ConsumerFpApp.GetConfig()
	fpStore := ctm.ConsumerFpApp.GetFinalityProviderStore()
	pubRandStore := ctm.ConsumerFpApp.GetPubRandProofStore()
	// Use the Consumer FP App's own Babylon controller, not the Babylon App's controller
	// This ensures proper connection lifecycle management
	bc := ctm.ConsumerFpApp.GetBabylonController()

	fpMetrics := metrics.NewFpMetrics()
	poller := service.NewChainPoller(ctm.logger, fpCfg.PollerConfig, ctm.RollupBSNController, fpMetrics)

	rndCommitter := service.NewDefaultRandomnessCommitter(
		service.NewRandomnessCommitterConfig(fpCfg.NumPubRand, int64(fpCfg.TimestampingDelayBlocks), fpCfg.ContextSigningHeight),
		service.NewPubRandState(pubRandStore), ctm.RollupBSNController, ctm.ConsumerEOTSClient, ctm.logger, fpMetrics)

	heightDeterminer := service.NewStartHeightDeterminer(ctm.RollupBSNController, fpCfg.PollerConfig, ctm.logger)
	finalitySubmitter := service.NewDefaultFinalitySubmitter(ctm.RollupBSNController, ctm.ConsumerEOTSClient, rndCommitter.GetPubRandProofList,
		service.NewDefaultFinalitySubmitterConfig(fpCfg.MaxSubmissionRetries,
			fpCfg.ContextSigningHeight,
			fpCfg.SubmissionRetryInterval),
		ctm.logger, fpMetrics)

	fpInstance, err := service.NewFinalityProviderInstance(
		consumerFpPk,
		fpCfg,
		fpStore,
		pubRandStore,
		bc,
		ctm.RollupBSNController,
		ctm.ConsumerEOTSClient,
		poller,
		rndCommitter,
		heightDeterminer,
		finalitySubmitter,
		fpMetrics,
		make(chan<- *service.CriticalError, 100),
		ctm.logger,
	)
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
	rollupController, err := rollupfpcontroller.NewRollupBSNController(ctm.RollupBSNController.Cfg, ctm.logger)
	require.NoError(t, err)

	// marshal query message
	queryMsgBytes, err := json.Marshal(queryMsg)
	require.NoError(t, err)

	var queryResponse *wasmtypes.QuerySmartContractStateResponse
	require.Eventually(t, func() bool {
		queryResponse, err = rollupController.QuerySmartContractState(
			ctx,
			ctm.RollupBSNController.Cfg.FinalityContractAddress,
			string(queryMsgBytes),
		)
		return err == nil
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

	return queryResponse
}

func (ctm *OpL2ConsumerTestManager) Stop(t *testing.T) {
	// Stop all FP apps with error handling
	if ctm.BabylonFpApp != nil {
		err := ctm.BabylonFpApp.Stop()
		if err != nil {
			t.Logf("Warning: Error stopping finality provider: %v", err)
		}
	}
	if ctm.ConsumerFpApp != nil {
		err := ctm.ConsumerFpApp.Stop()
		if err != nil {
			t.Logf("Warning: Error stopping finality provider: %v", err)
		}
	}

	// Stop EOTS server handler
	if ctm.EOTSServerHandler != nil {
		ctm.EOTSServerHandler.Stop()
	}

	// Clear Docker resources with error handling
	err := ctm.manager.ClearResources()
	if err != nil {
		t.Logf("Warning: Error clearing Docker resources: %v", err)
	}

	// Clean up temp directory
	err = os.RemoveAll(ctm.BaseDir)
	if err != nil {
		t.Logf("Warning: Error removing temporary directory: %v", err)
	}
}

// WaitForFpPubRandTimestamped waits for the FP to commit public randomness and get it timestamped
// This is a rollup-specific version that works with the rollup controller
func (ctm *OpL2ConsumerTestManager) WaitForFpPubRandTimestamped(t *testing.T, ctx context.Context, fpIns *service.FinalityProviderInstance) {
	var lastCommittedHeight uint64
	var err error

	require.Eventually(t, func() bool {
		lastCommittedHeight, err = fpIns.GetLastCommittedHeight(ctx)
		if err != nil {
			return false
		}
		return lastCommittedHeight > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("public randomness is successfully committed, last committed height: %d", lastCommittedHeight)

	// wait until the last registered epoch is finalised
	currentEpoch, err := ctm.BabylonController.QueryCurrentEpoch()
	require.NoError(t, err)

	ctm.FinalizeUntilEpoch(t, currentEpoch)

	res, err := ctm.BabylonController.GetBBNClient().LatestEpochFromStatus(ckpttypes.Finalized)
	require.NoError(t, err)
	t.Logf("last finalized epoch: %d", res.RawCheckpoint.EpochNum)

	t.Logf("public randomness is successfully timestamped, last finalized epoch: %d", currentEpoch)
}

// WaitForNRollupBlocks waits for the rollup chain to produce N more blocks
// This is the BSN equivalent of the Babylon test's WaitForNBlocks function
func (ctm *OpL2ConsumerTestManager) WaitForNRollupBlocks(t *testing.T, ctx context.Context, n int) uint64 {
	beforeHeight, err := ctm.RollupBSNController.QueryLatestBlock(ctx)
	require.NoError(t, err)

	var afterHeight uint64
	require.Eventually(t, func() bool {
		height, err := ctm.RollupBSNController.QueryLatestBlock(ctx)
		if err != nil {
			t.Logf("Failed to query latest rollup block height: %v", err)
			return false
		}

		if height.GetHeight() >= uint64(n)+beforeHeight.GetHeight() {
			afterHeight = height.GetHeight()
			return true
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime,
		fmt.Sprintf("rollup chain should produce %d more blocks", n))

	t.Logf("Rollup chain produced %d blocks: %d -> %d", n, beforeHeight, afterHeight)
	return afterHeight
}

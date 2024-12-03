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
	bbncfg "github.com/babylonlabs-io/babylon/client/config"
	bbncc "github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	opcc "github.com/babylonlabs-io/finality-provider/clientcontroller/opstackl2"
	cwclient "github.com/babylonlabs-io/finality-provider/cosmwasmclient/client"
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
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	opFinalityGadgetContractPath = "../bytecode/op_finality_gadget_16f6154.wasm"
	opConsumerChainId            = "op-stack-l2-706114"
	bbnAddrTopUpAmount           = 100000000
)

type BaseTestManager = base_test_manager.BaseTestManager

type OpL2ConsumerTestManager struct {
	BaseTestManager
	BaseDir              string
	BabylonHandler       *e2eutils.BabylonNodeHandler
	OpConsumerController *opcc.OPStackL2ConsumerController
}

// Config is the config of the OP finality gadget cw contract
// It will be removed by the final PR
type Config struct {
	ConsumerId string `json:"consumer_id"`
}

// - start Babylon node and wait for it starts
func StartOpL2ConsumerManager(t *testing.T) *OpL2ConsumerTestManager {
	// Setup base dir and logger
	testDir, err := e2eutils.BaseDir("op-fp-e2e-test")
	require.NoError(t, err)

	// setup logger
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.Level(zap.DebugLevel))
	logger, err := config.Build()
	require.NoError(t, err)

	// start Babylon node
	babylonHandler, covenantPrivKeys := startBabylonNode(t)

	// wait for Babylon node starts b/c we will fund the FP address with babylon node
	babylonController, stakingParams := waitForBabylonNodeStart(t, testDir, logger, babylonHandler)

	// register consumer chain to Babylon
	_, err = babylonController.RegisterConsumerChain(
		opConsumerChainId,
		"OP consumer chain",
		"Some description about the chain",
	)
	require.NoError(t, err)
	t.Logf(log.Prefix("Register consumer %s to Babylon"), opConsumerChainId)

	// create cosmwasm client
	// consumer FP config will be defined and updated later
	_, opConsumerCfg := createConsumerFpConfig(t, testDir, babylonHandler)
	cwConfig := opConsumerCfg.ToCosmwasmConfig()
	cwClient, err := opcc.NewCwClient(&cwConfig, logger)
	require.NoError(t, err)

	// deploy finality gadget cw contract
	opFinalityGadgetAddress := deployCwContract(t, cwClient)
	t.Logf(log.Prefix("op-finality-gadget contract address: %s"), opFinalityGadgetAddress)

	// update opConsumerCfg with opFinalityGadgetAddress
	opConsumerCfg.OPFinalityGadgetAddress = opFinalityGadgetAddress

	// create op consumer controller
	opConsumerController, err := opcc.NewOPStackL2ConsumerController(opConsumerCfg, logger)
	require.NoError(t, err)

	ctm := &OpL2ConsumerTestManager{
		BaseTestManager: BaseTestManager{
			BBNClient:        babylonController,
			CovenantPrivKeys: covenantPrivKeys,
			StakingParams:    stakingParams,
		},
		BaseDir:              testDir,
		BabylonHandler:       babylonHandler,
		OpConsumerController: opConsumerController,
	}

	return ctm
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

func waitForBabylonNodeStart(
	t *testing.T,
	testDir string,
	logger *zap.Logger,
	babylonHandler *e2eutils.BabylonNodeHandler,
) (*bbncc.BabylonController, *types.StakingParams) {
	// create Babylon FP config
	babylonFpCfg := createBabylonFpConfig(t, testDir, babylonHandler)

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
) (*fpcfg.Config, *fpcfg.OPStackL2Config) {
	fpHomeDir := filepath.Join(testDir, "consumer-fp-home")
	t.Logf(log.Prefix("Consumer FP home dir: %s"), fpHomeDir)

	cfg := e2eutils.DefaultFpConfigWithPorts(
		fpHomeDir,
		fpHomeDir,
		fpcfg.DefaultRPCPort+1,
		metrics.DefaultFpConfig().Port+1,
		eotsconfig.DefaultRPCPort+1,
	)

	// create consumer FP key/address
	fpBbnKeyInfo, err := service.CreateChainKey(
		cfg.BabylonConfig.KeyDirectory,
		cfg.BabylonConfig.ChainID,
		cfg.BabylonConfig.Key,
		cfg.BabylonConfig.KeyringBackend,
		e2eutils.Passphrase,
		e2eutils.HdPath,
		"",
	)
	require.NoError(t, err)

	// fund the consumer FP address
	t.Logf(log.Prefix("Funding %dubbn to %s"), bbnAddrTopUpAmount, fpBbnKeyInfo.AccAddress.String())
	err = bh.BabylonNode.TxBankSend(
		fpBbnKeyInfo.AccAddress.String(),
		fmt.Sprintf("%dubbn", bbnAddrTopUpAmount),
	)
	require.NoError(t, err)

	// check consumer FP address balance
	require.Eventually(t, func() bool {
		balance, err := bh.BabylonNode.CheckAddrBalance(fpBbnKeyInfo.AccAddress.String())
		if err != nil {
			t.Logf("Error checking balance: %v", err)
			return false
		}
		return balance == bbnAddrTopUpAmount
	}, 30*time.Second, 2*time.Second, fmt.Sprintf("failed to top up %s", fpBbnKeyInfo.AccAddress.String()))
	t.Logf(log.Prefix("Sent %dubbn to %s"), bbnAddrTopUpAmount, fpBbnKeyInfo.AccAddress.String())

	// set consumer FP config
	dc := bbncfg.DefaultBabylonConfig()
	opConsumerCfg := &fpcfg.OPStackL2Config{
		// it will be updated later
		OPFinalityGadgetAddress: "",
		// it must be a dialable RPC address checked by NewOPStackL2ConsumerController
		OPStackL2RPCAddress: "https://optimism-sepolia.drpc.org",
		// the value does not matter for the test
		BabylonFinalityGadgetRpc: "127.0.0.1:50051",
		Key:                      cfg.BabylonConfig.Key,
		ChainID:                  dc.ChainID,
		RPCAddr:                  dc.RPCAddr,
		GRPCAddr:                 dc.GRPCAddr,
		AccountPrefix:            dc.AccountPrefix,
		KeyringBackend:           dc.KeyringBackend,
		KeyDirectory:             cfg.BabylonConfig.KeyDirectory,
		GasAdjustment:            1.5,
		GasPrices:                "0.002ubbn",
		Debug:                    dc.Debug,
		Timeout:                  dc.Timeout,
		BlockTimeout:             1 * time.Minute,
		OutputFormat:             dc.OutputFormat,
		SignModeStr:              dc.SignModeStr,
	}
	cfg.OPStackL2Config = opConsumerCfg
	return cfg, opConsumerCfg
}

func deployCwContract(t *testing.T, cwClient *cwclient.Client) string {
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

func (ctm *OpL2ConsumerTestManager) Stop(t *testing.T) {
	t.Log("Stopping test manager")
	var err error
	err = ctm.BabylonHandler.Stop()
	require.NoError(t, err)

	err = os.RemoveAll(ctm.BaseDir)
	require.NoError(t, err)
}

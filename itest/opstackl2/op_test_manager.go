//go:build e2e_op
// +build e2e_op

package e2etest_op

import (
	"os"
	"path/filepath"
	"testing"

	bbncc "github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	eotsconfig "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	base_test_manager "github.com/babylonlabs-io/finality-provider/itest/test-manager"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/testutil/log"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type BaseTestManager = base_test_manager.BaseTestManager

type OpL2ConsumerTestManager struct {
	BaseTestManager
	BaseDir        string
	BabylonHandler *e2eutils.BabylonNodeHandler
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

	ctm := &OpL2ConsumerTestManager{
		BaseTestManager: BaseTestManager{
			BBNClient:        babylonController,
			CovenantPrivKeys: covenantPrivKeys,
			StakingParams:    stakingParams,
		},
		BaseDir:        testDir,
		BabylonHandler: babylonHandler,
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

func (ctm *OpL2ConsumerTestManager) Stop(t *testing.T) {
	t.Log("Stopping test manager")
	var err error
	err = ctm.BabylonHandler.Stop()
	require.NoError(t, err)

	err = os.RemoveAll(ctm.BaseDir)
	require.NoError(t, err)
}

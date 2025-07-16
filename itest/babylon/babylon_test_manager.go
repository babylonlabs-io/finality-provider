package e2etest_babylon

import (
	"context"
	"fmt"
	"github.com/avast/retry-go/v4"
	fpstore "github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"

	ccapi "github.com/babylonlabs-io/finality-provider/clientcontroller/api"

	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	bbncc "github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
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

const (
	eventuallyWaitTimeOut = 5 * time.Minute
	eventuallyPollTime    = 1 * time.Second

	testMoniker = "test-moniker"
	testChainID = "chain-test"
	passphrase  = "testpass"
	hdPath      = ""
)

type TestManager struct {
	*base_test_manager.BaseTestManager
	EOTSServerHandler *e2eutils.EOTSServerHandler
	EOTSHomeDir       string
	FpConfig          *fpcfg.Config
	Fps               []*service.FinalityProviderApp
	EOTSClient        *client.EOTSManagerGRpcClient
	BBNConsumerClient *bbncc.BabylonConsumerController
	baseDir           string
	manager           *container.Manager
	logger            *zap.Logger
	babylond          *dockertest.Resource
}

func StartManager(t *testing.T, ctx context.Context, eotsHmacKey string, fpHmacKey string) *TestManager {
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

	// update ports with the dynamically allocated ones from docker
	cfg.BabylonConfig.RPCAddr = fmt.Sprintf("http://localhost:%s", babylond.GetPort("26657/tcp"))
	cfg.BabylonConfig.GRPCAddr = fmt.Sprintf("https://localhost:%s", babylond.GetPort("9090/tcp"))

	var bc ccapi.ClientController
	var bcc ccapi.ConsumerController

	// Increase timeout and polling interval for CI environments
	startTimeout := 30 * time.Second
	startPollInterval := 1 * time.Second

	require.Eventually(t, func() bool {
		bbnCfg := cfg.BabylonConfig.ToBabylonConfig()
		bbnCl, err := bbnclient.New(&bbnCfg, logger)
		if err != nil {
			t.Logf("failed to create Babylon client: %v", err)
			// Add small delay to avoid overwhelming the system
			time.Sleep(100 * time.Millisecond)
			return false
		}
		bc, err = bbncc.NewBabylonController(bbnCl, cfg.BabylonConfig, logger)
		if err != nil {
			t.Logf("failed to create Babylon controller: %v", err)
			time.Sleep(100 * time.Millisecond)
			return false
		}

		err = bc.Start()
		if err != nil {
			t.Logf("failed to start Babylon controller: %v", err)
			time.Sleep(200 * time.Millisecond)
			return false
		}
		bcc, err = bbncc.NewBabylonConsumerController(cfg.BabylonConfig, logger)
		if err != nil {
			t.Logf("failed to create Babylon consumer controller: %v", err)
			time.Sleep(100 * time.Millisecond)
			return false
		}
		return true
	}, startTimeout, startPollInterval)

	// Prepare EOTS manager
	eotsHomeDir := filepath.Join(testDir, "eots-home")
	eotsCfg := eotsconfig.DefaultConfigWithHomePath(eotsHomeDir)
	eotsCfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eotsCfg.Metrics.Port = testutil.AllocateUniquePort(t)

	// Set HMAC key for EOTS server if provided
	if eotsHmacKey != "" {
		eotsCfg.HMACKey = eotsHmacKey
		t.Logf("Using EOTS server HMAC key: %s", eotsHmacKey)
	}

	// Set HMAC key for finality provider client if provided
	if fpHmacKey != "" {
		cfg.HMACKey = fpHmacKey
		t.Logf("Using FP client HMAC key: %s", fpHmacKey)
	}

	eh := e2eutils.NewEOTSServerHandler(t, eotsCfg, eotsHomeDir)
	eh.Start(ctx)

	cfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eotsCli := NewEOTSManagerGrpcClientWithRetry(t, eotsCfg)

	tm := &TestManager{
		BaseTestManager: &base_test_manager.BaseTestManager{
			BabylonController: bc.(*bbncc.BabylonController),
			CovenantPrivKeys:  covenantPrivKeys,
		},
		EOTSServerHandler: eh,
		EOTSHomeDir:       eotsHomeDir,
		FpConfig:          cfg,
		EOTSClient:        eotsCli,
		BBNConsumerClient: bcc.(*bbncc.BabylonConsumerController),
		baseDir:           testDir,
		manager:           manager,
		logger:            logger,
		babylond:          babylond,
	}

	tm.WaitForServicesStart(t)

	return tm
}

func (tm *TestManager) AddFinalityProvider(t *testing.T, ctx context.Context, hmacKey ...string) *service.FinalityProviderInstance {
	r := rand.New(rand.NewSource(time.Now().Unix()))

	eotsKeyName := fmt.Sprintf("eots-key-%s", datagen.GenRandomHexStr(r, 4))
	eotsPkBz, err := tm.EOTSServerHandler.CreateKey(eotsKeyName, "")
	require.NoError(t, err)

	eotsPk, err := bbntypes.NewBIP340PubKey(eotsPkBz)
	require.NoError(t, err)

	t.Logf("the EOTS key is created: %s", eotsPk.MarshalHex())

	// Create FP babylon key
	fpKeyName := fmt.Sprintf("fp-key-%s", datagen.GenRandomHexStr(r, 4))
	fpHomeDir := filepath.Join(tm.baseDir, fmt.Sprintf("fp-%s", datagen.GenRandomHexStr(r, 4)))
	cfg := e2eutils.DefaultFpConfig(tm.baseDir, fpHomeDir)
	cfg.BabylonConfig.Key = fpKeyName
	cfg.BabylonConfig.RPCAddr = fmt.Sprintf("http://localhost:%s", tm.babylond.GetPort("26657/tcp"))
	cfg.BabylonConfig.GRPCAddr = fmt.Sprintf("https://localhost:%s", tm.babylond.GetPort("9090/tcp"))
	cfg.ContextSigningHeight = ^uint64(0) // max uint64 to enable context signing

	// Set HMAC key if provided
	if len(hmacKey) > 0 && hmacKey[0] != "" {
		cfg.HMACKey = hmacKey[0]
	}

	fpBbnKeyInfo, err := testutil.CreateChainKey(cfg.BabylonConfig.KeyDirectory, cfg.BabylonConfig.ChainID, cfg.BabylonConfig.Key, cfg.BabylonConfig.KeyringBackend, passphrase, hdPath, "")
	require.NoError(t, err)

	t.Logf("the Babylon key is created: %s", fpBbnKeyInfo.AccAddress.String())

	// Add funds for new FP
	_, _, err = tm.manager.BabylondTxBankSend(t, fpBbnKeyInfo.AccAddress.String(), "1000000ubbn", "node0")
	require.NoError(t, err)

	// create new clients
	bc, err := fpcc.NewBabylonController(cfg.BabylonConfig, tm.logger)
	require.NoError(t, err)
	err = bc.Start()
	require.NoError(t, err)
	bcc, err := bbncc.NewBabylonConsumerController(cfg.BabylonConfig, tm.logger)
	require.NoError(t, err)

	// Create and start finality provider app
	eotsCli, err := client.NewEOTSManagerGRpcClient(tm.EOTSServerHandler.Config().RPCListener, tm.EOTSServerHandler.Config().HMACKey)
	require.NoError(t, err)
	fpdb, err := cfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)

	fpMetrics := metrics.NewFpMetrics()
	poller := service.NewChainPoller(tm.logger, cfg.PollerConfig, bcc, fpMetrics)
	pubRandStore, err := fpstore.NewPubRandProofStore(fpdb)
	require.NoError(t, err)
	rndCommitter := service.NewDefaultRandomnessCommitter(
		service.NewRandomnessCommitterConfig(cfg.NumPubRand, int64(cfg.TimestampingDelayBlocks), cfg.ContextSigningHeight),
		service.NewPubRandState(pubRandStore), bcc, eotsCli, tm.logger, fpMetrics)
	heightDeterminer := service.NewStartHeightDeterminer(bcc, cfg.PollerConfig, tm.logger)

	fpApp, err := service.NewFinalityProviderApp(cfg, bc, bcc, eotsCli, poller, rndCommitter, heightDeterminer, fpMetrics, fpdb, tm.logger)
	require.NoError(t, err)
	err = fpApp.Start()
	require.NoError(t, err)

	// Create and register the finality provider
	// Add retry logic for creating the finality provider
	commission := testutil.ZeroCommissionRate()
	desc := newDescription(testMoniker)

	_, err = fpApp.CreateFinalityProvider(context.Background(), cfg.BabylonConfig.Key, testChainID, eotsPk, desc, commission)
	require.NoError(t, err)

	cfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	cfg.Metrics.Port = testutil.AllocateUniquePort(t)

	err = fpApp.StartFinalityProvider(eotsPk)
	require.NoError(t, err)

	fpServer := service.NewFinalityProviderServer(cfg, tm.logger, fpApp, fpdb)
	go func() {
		err = fpServer.RunUntilShutdown(ctx)
		require.NoError(t, err)
	}()

	tm.Fps = append(tm.Fps, fpApp)

	fpIns, err := fpApp.GetFinalityProviderInstance()
	require.NoError(t, err)

	return fpIns
}

func (tm *TestManager) WaitForServicesStart(t *testing.T) {
	require.Eventually(t, func() bool {
		_, err := tm.BabylonController.QueryBtcLightClientTip()

		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("Babylon node is started")
}

func StartManagerWithFinalityProvider(t *testing.T, n int, ctx context.Context, hmacKey ...string) (*TestManager, []*service.FinalityProviderInstance) {
	// If HMAC key is provided, use it for both server and client
	var tm *TestManager
	if len(hmacKey) > 0 && hmacKey[0] != "" {
		// Use the same key for both EOTS server and FP client for simplicity
		tm = StartManager(t, ctx, hmacKey[0], hmacKey[0])
	} else {
		tm = StartManager(t, ctx, "", "")
	}

	var runningFps []*service.FinalityProviderInstance
	for i := 0; i < n; i++ {
		// Pass the HMAC key if provided, otherwise don't use HMAC
		var fpIns *service.FinalityProviderInstance
		if len(hmacKey) > 0 && hmacKey[0] != "" {
			fpIns = tm.AddFinalityProvider(t, ctx, hmacKey[0])
		} else {
			fpIns = tm.AddFinalityProvider(t, ctx)
		}
		runningFps = append(runningFps, fpIns)
	}

	// Check finality providers on Babylon side
	require.Eventually(t, func() bool {
		fps, err := tm.BabylonController.QueryFinalityProviders()
		if err != nil {
			t.Logf("failed to query finality providers from Babylon %s", err.Error())
			return false
		}

		return len(fps) == n
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("the test manager is running with a finality provider")

	return tm, runningFps
}

func (tm *TestManager) Stop(t *testing.T) {
	for _, fpApp := range tm.Fps {
		err := fpApp.Stop()
		if err != nil {
			t.Logf("Warning: Error stopping finality provider: %v", err)
		}
	}
	err := tm.manager.ClearResources()
	if err != nil {
		t.Logf("Warning: Error clearing Docker resources: %v", err)
	}

	err = os.RemoveAll(tm.baseDir)
	if err != nil {
		t.Logf("Warning: Error removing temporary directory: %v", err)
	}
}

func (tm *TestManager) CheckBlockFinalization(t *testing.T, height uint64, num int) {
	// We need to ensure votes are collected at the given height
	require.Eventually(t, func() bool {
		votes, err := tm.BabylonController.QueryVotesAtHeight(height)
		if err != nil {
			t.Logf("failed to get the votes at height %v: %s", height, err.Error())
			return false
		}
		return len(votes) >= num // votes could come in faster than we poll
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// As the votes have been collected, the block should be finalized
	require.Eventually(t, func() bool {
		finalized, err := tm.BBNConsumerClient.QueryIsBlockFinalized(context.Background(), height)
		if err != nil {
			t.Logf("failed to query block at height %v: %s", height, err.Error())
			return false
		}
		return finalized
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func (tm *TestManager) WaitForFpVoteCast(t *testing.T, fpIns *service.FinalityProviderInstance) uint64 {
	var lastVotedHeight uint64
	require.Eventually(t, func() bool {
		if fpIns.GetLastVotedHeight() > 0 {
			lastVotedHeight = fpIns.GetLastVotedHeight()
			return true
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	return lastVotedHeight
}

func (tm *TestManager) WaitForFpVoteCastAtHeight(t *testing.T, fpIns *service.FinalityProviderInstance, height uint64) {
	var lastVotedHeight uint64
	require.Eventually(t, func() bool {
		votedHeight := fpIns.GetLastVotedHeight()
		if votedHeight >= height {
			lastVotedHeight = votedHeight
			return true
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("the fp voted at height %d", lastVotedHeight)
}

func (tm *TestManager) StopAndRestartFpAfterNBlocks(t *testing.T, n int, fpIns *service.FinalityProviderInstance) {
	blockBeforeStop, err := tm.BBNConsumerClient.QueryLatestBlockHeight(context.Background())
	require.NoError(t, err)
	err = fpIns.Stop()
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		headerAfterStop, err := tm.BBNConsumerClient.QueryLatestBlockHeight(context.Background())
		if err != nil {
			return false
		}

		return headerAfterStop >= uint64(n)+blockBeforeStop
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Log("restarting the finality-provider instance")

	err = fpIns.Start()
	require.NoError(t, err)
}

func (tm *TestManager) WaitForNBlocks(t *testing.T, n int) uint64 {
	beforeHeight, err := tm.BBNConsumerClient.QueryLatestBlockHeight(context.Background())
	require.NoError(t, err)

	var afterHeight uint64
	require.Eventually(t, func() bool {
		height, err := tm.BBNConsumerClient.QueryLatestBlockHeight(context.Background())
		if err != nil {
			return false
		}

		if height >= uint64(n)+beforeHeight {
			afterHeight = height
			return true
		}

		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	return afterHeight
}

func (tm *TestManager) WaitForNFinalizedBlocks(t *testing.T, n uint) *types.BlockInfo {
	var (
		firstFinalizedBlock types.BlockDescription
		err                 error
		lastFinalizedBlock  types.BlockDescription
	)

	require.Eventually(t, func() bool {
		lastFinalizedBlock, err = tm.BBNConsumerClient.QueryLatestFinalizedBlock(context.Background())
		if err != nil {
			t.Logf("failed to get the latest finalized block: %s", err.Error())
			return false
		}
		if lastFinalizedBlock == nil {
			return false
		}
		if firstFinalizedBlock == nil {
			firstFinalizedBlock = lastFinalizedBlock
		}
		return lastFinalizedBlock.GetHeight()-firstFinalizedBlock.GetHeight() >= uint64(n-1)
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("the block is finalized at %v", lastFinalizedBlock.GetHeight())

	return types.NewBlockInfo(lastFinalizedBlock.GetHeight(), lastFinalizedBlock.GetHash(), lastFinalizedBlock.IsFinalized())
}

func newDescription(moniker string) *stakingtypes.Description {
	dec := stakingtypes.NewDescription(moniker, "", "", "", "")
	return &dec
}

func NewEOTSManagerGrpcClientWithRetry(t *testing.T, cfg *eotsconfig.Config) *client.EOTSManagerGRpcClient {
	var err error
	var eotsCli *client.EOTSManagerGRpcClient
	err = retry.Do(func() error {
		eotsCli, err = client.NewEOTSManagerGRpcClient(cfg.RPCListener, cfg.HMACKey)
		return err
	}, retry.Attempts(5))
	require.NoError(t, err)

	return eotsCli
}

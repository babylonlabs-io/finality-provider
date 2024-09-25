package e2etest_wasmd

import (
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
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	bbncc "github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	cwcc "github.com/babylonlabs-io/finality-provider/clientcontroller/cosmwasm"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	eotsconfig "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type WasmdTestManager struct {
	BabylonHandler      *e2eutils.BabylonNodeHandler
	FpConfig            *fpcfg.Config
	BBNClient           *bbncc.BabylonController
	WasmdHandler        *WasmdNodeHandler
	WasmdConsumerClient *cwcc.CosmwasmConsumerController
	StakingParams       *types.StakingParams
	EOTSServerHandler   *e2eutils.EOTSServerHandler
	EOTSConfig          *eotsconfig.Config
	Fpa                 *service.FinalityProviderApp
	EOTSClient          *client.EOTSManagerGRpcClient
	CovenantPrivKeys    []*btcec.PrivateKey
	baseDir             string
}

func StartWasmdTestManager(t *testing.T) *WasmdTestManager {
	// Setup consumer test manager
	testDir, err := e2eutils.BaseDir("fpe2etest")
	require.NoError(t, err)

	logger := zap.NewNop()

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

	// 3. setup wasmd node
	wh := NewWasmdNodeHandler(t)
	err = wh.Start()
	require.NoError(t, err)
	cfg.CosmwasmConfig = fpcfg.DefaultCosmwasmConfig()
	cfg.CosmwasmConfig.KeyDirectory = wh.dataDir
	// TODO: make random contract addresses for now to avoid validation errors
	//  later in the e2e tests we would upload the contract and update the addresses
	//  investigate if there is a better way to handle this
	cfg.CosmwasmConfig.BtcStakingContractAddress = datagen.GenRandomAccount().GetAddress().String()
	cfg.ChainType = fpcc.WasmConsumerChainType
	cfg.CosmwasmConfig.AccountPrefix = "wasm"
	cfg.CosmwasmConfig.ChainID = wasmdChainID
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

	ctm := &WasmdTestManager{
		BabylonHandler:      bh,
		FpConfig:            cfg,
		BBNClient:           bc,
		WasmdHandler:        wh,
		WasmdConsumerClient: wcc,
		EOTSServerHandler:   eh,
		EOTSConfig:          eotsCfg,
		Fpa:                 fpApp,
		EOTSClient:          eotsCli,
		CovenantPrivKeys:    covenantPrivKeys,
		baseDir:             testDir,
	}

	ctm.WaitForServicesStart(t)
	return ctm
}

func (ctm *WasmdTestManager) WaitForServicesStart(t *testing.T) {
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
		blockHeight, err := ctm.WasmdHandler.GetLatestBlockHeight()
		if err != nil {
			t.Logf("failed to get latest block height from wasmd %s", err.Error())
			return false
		}
		return blockHeight > 2
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
	t.Logf("Wasmd node is started")
}

func (ctm *WasmdTestManager) Stop(t *testing.T) {
	err := ctm.Fpa.Stop()
	require.NoError(t, err)
	err = ctm.BabylonHandler.Stop()
	require.NoError(t, err)
	ctm.EOTSServerHandler.Stop()
	ctm.WasmdHandler.Stop(t)
	err = os.RemoveAll(ctm.baseDir)
	require.NoError(t, err)
}

func StartConsumerManagerWithFps(t *testing.T, n int) (*WasmdTestManager, []*service.FinalityProviderInstance) {
	ctm := StartWasmdTestManager(t)
	app := ctm.Fpa

	for i := 0; i < n; i++ {
		fpName := e2eutils.FpNamePrefix + strconv.Itoa(i)
		moniker := e2eutils.MonikerPrefix + strconv.Itoa(i)
		commission := sdkmath.LegacyZeroDec()
		desc := e2eutils.NewDescription(moniker)
		cfg := app.GetConfig()
		_, err := service.CreateChainKey(cfg.BabylonConfig.KeyDirectory, cfg.BabylonConfig.ChainID, fpName, keyring.BackendTest, e2eutils.Passphrase, e2eutils.HdPath, "")
		require.NoError(t, err)
		res, err := app.CreateFinalityProvider(fpName, e2eutils.ChainID, e2eutils.Passphrase, e2eutils.HdPath, nil, desc, &commission)
		require.NoError(t, err)
		fpPk, err := bbntypes.NewBIP340PubKeyFromHex(res.FpInfo.BtcPkHex)
		require.NoError(t, err)
		_, err = app.RegisterFinalityProvider(fpPk.MarshalHex())
		require.NoError(t, err)
		err = app.StartHandlingFinalityProvider(fpPk, e2eutils.Passphrase)
		require.NoError(t, err)
		fpIns, err := app.GetFinalityProviderInstance(fpPk)
		require.NoError(t, err)
		require.True(t, fpIns.IsRunning())
		require.NoError(t, err)

		// check finality providers on Babylon side
		require.Eventually(t, func() bool {
			fps, err := ctm.BBNClient.QueryFinalityProviders()
			if err != nil {
				t.Logf("failed to query finality providers from Babylon %s", err.Error())
				return false
			}

			if len(fps) != i+1 {
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
	}

	fpInsList := app.ListFinalityProviderInstances()
	require.Equal(t, n, len(fpInsList))

	t.Logf("the consumer test manager is running with %v finality-provider(s)", len(fpInsList))

	return ctm, fpInsList
}

func (ctm *WasmdTestManager) CreateFinalityProvidersForChain(t *testing.T, chainID string, n int) []*service.FinalityProviderInstance {
	app := ctm.Fpa
	cfg := app.GetConfig()

	// register all finality providers
	fpPKs := make([]*bbntypes.BIP340PubKey, 0, n)
	for i := 0; i < n; i++ {
		fpName := e2eutils.FpNamePrefix + chainID + "-" + strconv.Itoa(i)
		moniker := e2eutils.MonikerPrefix + chainID + "-" + strconv.Itoa(i)
		commission := sdkmath.LegacyZeroDec()
		desc := e2eutils.NewDescription(moniker)
		_, err := service.CreateChainKey(cfg.BabylonConfig.KeyDirectory, chainID, fpName, keyring.BackendTest, e2eutils.Passphrase, e2eutils.HdPath, "")
		require.NoError(t, err)
		res, err := app.CreateFinalityProvider(fpName, chainID, e2eutils.Passphrase, e2eutils.HdPath, nil, desc, &commission)
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
		fps, err := ctm.BBNClient.QueryFinalityProviders()
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

	fpInsList := app.ListFinalityProviderInstancesForChain(chainID)
	require.Equal(t, n, len(fpInsList))

	t.Logf("the consumer test manager is running with %v finality-provider(s)", len(fpInsList))

	return fpInsList
}

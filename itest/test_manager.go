package e2etest

import (
	"context"
	"encoding/hex"
	"fmt"
	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"

	"github.com/babylonlabs-io/finality-provider/itest/container"
	"github.com/babylonlabs-io/finality-provider/testutil"

	sdkmath "cosmossdk.io/math"
	"github.com/babylonlabs-io/babylon/btcstaking"
	txformat "github.com/babylonlabs-io/babylon/btctxformatter"
	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	ckpttypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	eotsconfig "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/types"
)

var (
	eventuallyWaitTimeOut = 5 * time.Minute
	eventuallyPollTime    = 500 * time.Millisecond
	btcNetworkParams      = &chaincfg.SimNetParams

	testMoniker  = "test-moniker"
	testChainID  = "chain-test"
	passphrase   = "testpass"
	hdPath       = ""
	simnetParams = &chaincfg.SimNetParams
)

type TestManager struct {
	Wg                sync.WaitGroup
	EOTSServerHandler *EOTSServerHandler
	EOTSHomeDir       string
	FpConfig          *fpcfg.Config
	Fps               []*service.FinalityProviderApp
	EOTSClient        *client.EOTSManagerGRpcClient
	BBNClient         *fpcc.BabylonController
	StakingParams     *types.StakingParams
	CovenantPrivKeys  []*btcec.PrivateKey
	baseDir           string
	manager           *container.Manager
	logger            *zap.Logger
	babylond          *dockertest.Resource
}

type TestDelegationData struct {
	FPAddr           sdk.AccAddress
	DelegatorPrivKey *btcec.PrivateKey
	DelegatorKey     *btcec.PublicKey
	SlashingTx       *bstypes.BTCSlashingTx
	StakingTx        *wire.MsgTx
	StakingTxInfo    *btcctypes.TransactionInfo
	DelegatorSig     *bbntypes.BIP340Signature
	FpPks            []*btcec.PublicKey

	SlashingPkScript []byte
	ChangeAddr       string
	StakingTime      uint16
	StakingAmount    int64
}

func StartManager(t *testing.T, ctx context.Context) *TestManager {
	manager, err := container.NewManager(t)
	require.NoError(t, err)
	testDir, err := tempDir(t, "fp-e2e-test-*")
	require.NoError(t, err)

	loggerConfig := zap.NewDevelopmentConfig()
	loggerConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	logger, err := loggerConfig.Build()
	require.NoError(t, err)

	// 1. generate covenant committee
	covenantQuorum := 2
	numCovenants := 3
	covenantPrivKeys, covenantPubKeys := generateCovenantCommittee(numCovenants, t)

	// 2. prepare Babylon node
	babylonDir, err := tempDir(t, "babylon-test-*")
	require.NoError(t, err)
	babylond, err := manager.RunBabylondResource(t, babylonDir, covenantQuorum, covenantPubKeys)
	require.NoError(t, err)

	keyDir := filepath.Join(babylonDir, "node0", "babylond")
	fpHomeDir := filepath.Join(testDir, "fp-home")
	cfg := defaultFpConfig(keyDir, fpHomeDir)
	// update ports with the dynamically allocated ones from docker
	cfg.BabylonConfig.RPCAddr = fmt.Sprintf("http://localhost:%s", babylond.GetPort("26657/tcp"))
	cfg.BabylonConfig.GRPCAddr = fmt.Sprintf("https://localhost:%s", babylond.GetPort("9090/tcp"))

	bbnCfg := fpcfg.BBNConfigToBabylonConfig(cfg.BabylonConfig)
	bbnCl, err := bbnclient.New(&bbnCfg, logger)
	require.NoError(t, err)

	var bc *fpcc.BabylonController
	require.Eventually(t, func() bool {
		bc = fpcc.NewBabylonController(bbnCl, cfg.BabylonConfig, &cfg.BTCNetParams, logger)
		err = bc.Start()
		if err != nil {
			t.Log(err)
		}
		return err == nil
	}, 10*time.Second, eventuallyPollTime)

	// 3. prepare EOTS manager
	eotsHomeDir := filepath.Join(testDir, "eots-home")
	eotsCfg := eotsconfig.DefaultConfigWithHomePath(eotsHomeDir)
	eotsCfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eotsCfg.Metrics.Port = testutil.AllocateUniquePort(t)
	eh := NewEOTSServerHandler(t, eotsCfg, eotsHomeDir)
	eh.Start(ctx)
	cfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eotsCli, err := client.NewEOTSManagerGRpcClient(eotsCfg.RPCListener)
	require.NoError(t, err)

	tm := &TestManager{
		EOTSServerHandler: eh,
		EOTSHomeDir:       eotsHomeDir,
		FpConfig:          cfg,
		EOTSClient:        eotsCli,
		BBNClient:         bc,
		CovenantPrivKeys:  covenantPrivKeys,
		baseDir:           testDir,
		manager:           manager,
		logger:            logger,
		babylond:          babylond,
	}

	tm.WaitForServicesStart(t)

	return tm
}

func (tm *TestManager) AddFinalityProvider(t *testing.T, ctx context.Context) *service.FinalityProviderInstance {
	r := rand.New(rand.NewSource(time.Now().Unix()))

	// create eots key
	eotsKeyName := fmt.Sprintf("eots-key-%s", datagen.GenRandomHexStr(r, 4))
	eotsPkBz, err := tm.EOTSClient.CreateKey(eotsKeyName, passphrase, hdPath)
	require.NoError(t, err)
	eotsPk, err := bbntypes.NewBIP340PubKey(eotsPkBz)
	require.NoError(t, err)

	// create fp babylon key
	fpKeyName := fmt.Sprintf("fp-key-%s", datagen.GenRandomHexStr(r, 4))
	fpHomeDir := filepath.Join(tm.baseDir, fmt.Sprintf("fp-%s", datagen.GenRandomHexStr(r, 4)))
	cfg := defaultFpConfig(tm.baseDir, fpHomeDir)
	cfg.BabylonConfig.Key = fpKeyName
	cfg.BabylonConfig.RPCAddr = fmt.Sprintf("http://localhost:%s", tm.babylond.GetPort("26657/tcp"))
	cfg.BabylonConfig.GRPCAddr = fmt.Sprintf("https://localhost:%s", tm.babylond.GetPort("9090/tcp"))
	fpBbnKeyInfo, err := testutil.CreateChainKey(cfg.BabylonConfig.KeyDirectory, cfg.BabylonConfig.ChainID, cfg.BabylonConfig.Key, cfg.BabylonConfig.KeyringBackend, passphrase, hdPath, "")
	require.NoError(t, err)

	// add some funds for new fp pay for fees '-'
	_, _, err = tm.manager.BabylondTxBankSend(t, fpBbnKeyInfo.AccAddress.String(), "1000000ubbn", "node0")
	require.NoError(t, err)

	// create and start finality provider app
	eotsCli, err := client.NewEOTSManagerGRpcClient(tm.EOTSServerHandler.cfg.RPCListener)
	require.NoError(t, err)
	cc, err := fpcc.NewClientController(cfg.ChainType, cfg.BabylonConfig, &cfg.BTCNetParams, tm.logger)
	require.NoError(t, err)
	err = cc.Start()
	require.NoError(t, err)
	fpdb, err := cfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)
	fpApp, err := service.NewFinalityProviderApp(cfg, cc, eotsCli, fpdb, tm.logger)
	require.NoError(t, err)
	err = fpApp.Start()
	require.NoError(t, err)

	// create and register the finality provider
	commission := sdkmath.LegacyZeroDec()
	desc := newDescription(testMoniker)
	_, err = fpApp.CreateFinalityProvider(cfg.BabylonConfig.Key, testChainID, passphrase, eotsPk, desc, &commission)
	require.NoError(t, err)

	cfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	cfg.Metrics.Port = testutil.AllocateUniquePort(t)

	err = fpApp.StartFinalityProvider(eotsPk, passphrase)
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
	// wait for Babylon node starts
	require.Eventually(t, func() bool {
		params, err := tm.BBNClient.QueryStakingParams()
		if err != nil {
			return false
		}
		tm.StakingParams = params
		return true
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("Babylon node is started")
}

func StartManagerWithFinalityProvider(t *testing.T, n int, ctx context.Context) (*TestManager, []*service.FinalityProviderInstance) {
	tm := StartManager(t, ctx)

	var runningFps []*service.FinalityProviderInstance
	for i := 0; i < n; i++ {
		fpIns := tm.AddFinalityProvider(t, ctx)
		runningFps = append(runningFps, fpIns)
	}

	// check finality providers on Babylon side
	require.Eventually(t, func() bool {
		fps, err := tm.BBNClient.QueryFinalityProviders()
		if err != nil {
			t.Logf("failed to query finality providers from Babylon %s", err.Error())
			return false
		}

		return len(fps) == n
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("the test manager is running with %d finality provider", n)

	return tm, runningFps
}

func (tm *TestManager) Stop(t *testing.T) {
	for _, fpApp := range tm.Fps {
		err := fpApp.Stop()
		require.NoError(t, err)
	}
	err := tm.manager.ClearResources()
	require.NoError(t, err)
	err = os.RemoveAll(tm.baseDir)
	require.NoError(t, err)
}

func (tm *TestManager) WaitForFpPubRandTimestamped(t *testing.T, fpIns *service.FinalityProviderInstance) {
	var lastCommittedHeight uint64
	var err error

	require.Eventually(t, func() bool {
		lastCommittedHeight, err = fpIns.GetLastCommittedHeight()
		if err != nil {
			return false
		}
		return lastCommittedHeight > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("public randomness is successfully committed, last committed height: %d", lastCommittedHeight)

	// wait until the last registered epoch is finalised
	currentEpoch, err := tm.BBNClient.QueryCurrentEpoch()
	require.NoError(t, err)

	tm.FinalizeUntilEpoch(t, currentEpoch)

	res, err := tm.BBNClient.GetBBNClient().LatestEpochFromStatus(ckpttypes.Finalized)
	require.NoError(t, err)
	t.Logf("last finalized epoch: %d", res.RawCheckpoint.EpochNum)

	t.Logf("public randomness is successfully timestamped, last finalized epoch: %d", currentEpoch)
}

func (tm *TestManager) WaitForNPendingDels(t *testing.T, n int) []*bstypes.BTCDelegationResponse {
	var (
		dels []*bstypes.BTCDelegationResponse
		err  error
	)
	require.Eventually(t, func() bool {
		dels, err = tm.BBNClient.QueryPendingDelegations(
			100,
		)
		if err != nil {
			return false
		}
		return len(dels) == n
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("delegations are pending")

	return dels
}

func (tm *TestManager) WaitForNActiveDels(t *testing.T, n int) []*bstypes.BTCDelegationResponse {
	var (
		dels []*bstypes.BTCDelegationResponse
		err  error
	)
	require.Eventually(t, func() bool {
		dels, err = tm.BBNClient.QueryActiveDelegations(
			100,
		)
		if err != nil {
			return false
		}
		return len(dels) == n
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("delegations are active")

	return dels
}

func generateCovenantCommittee(numCovenants int, t *testing.T) ([]*btcec.PrivateKey, []*bbntypes.BIP340PubKey) {
	var (
		covenantPrivKeys []*btcec.PrivateKey
		covenantPubKeys  []*bbntypes.BIP340PubKey
	)

	for i := 0; i < numCovenants; i++ {
		privKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		covenantPrivKeys = append(covenantPrivKeys, privKey)
		pubKey := bbntypes.NewBIP340PubKeyFromBTCPK(privKey.PubKey())
		covenantPubKeys = append(covenantPubKeys, pubKey)
	}

	return covenantPrivKeys, covenantPubKeys
}

func (tm *TestManager) CheckBlockFinalization(t *testing.T, height uint64, num int) {
	// we need to ensure votes are collected at the given height
	require.Eventually(t, func() bool {
		votes, err := tm.BBNClient.QueryVotesAtHeight(height)
		if err != nil {
			t.Logf("failed to get the votes at height %v: %s", height, err.Error())
			return false
		}
		return len(votes) == num
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// as the votes have been collected, the block should be finalized
	require.Eventually(t, func() bool {
		b, err := tm.BBNClient.QueryBlock(height)
		if err != nil {
			t.Logf("failed to query block at height %v: %s", height, err.Error())
			return false
		}
		return b.Finalized
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func (tm *TestManager) WaitForFpVoteCast(t *testing.T, fpIns *service.FinalityProviderInstance) uint64 {
	var lastVotedHeight uint64
	require.Eventually(t, func() bool {
		if fpIns.GetLastVotedHeight() > 0 {
			lastVotedHeight = fpIns.GetLastVotedHeight()
			return true
		} else {
			return false
		}
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	return lastVotedHeight
}

func (tm *TestManager) WaitForNFinalizedBlocks(t *testing.T, n int) []*types.BlockInfo {
	var (
		blocks []*types.BlockInfo
		err    error
	)
	require.Eventually(t, func() bool {
		blocks, err = tm.BBNClient.QueryLatestFinalizedBlocks(uint64(n))
		if err != nil {
			t.Logf("failed to get the latest finalized block: %s", err.Error())
			return false
		}
		return len(blocks) == n
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("the block is finalized at %v", blocks[0].Height)

	return blocks
}

func (tm *TestManager) StopAndRestartFpAfterNBlocks(t *testing.T, n int, fpIns *service.FinalityProviderInstance) {
	blockBeforeStop, err := tm.BBNClient.QueryBestBlock()
	require.NoError(t, err)
	err = fpIns.Stop()
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		headerAfterStop, err := tm.BBNClient.QueryBestBlock()
		if err != nil {
			return false
		}

		return headerAfterStop.Height >= uint64(n)+blockBeforeStop.Height
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Log("restarting the finality-provider instance")

	err = fpIns.Start()
	require.NoError(t, err)
}

func (tm *TestManager) GetFpPrivKey(t *testing.T, fpPk []byte) *btcec.PrivateKey {
	record, err := tm.EOTSClient.KeyRecord(fpPk, passphrase)
	require.NoError(t, err)
	return record.PrivKey
}

func (tm *TestManager) InsertCovenantSigForDelegation(t *testing.T, btcDel *bstypes.BTCDelegation) {
	slashingTx := btcDel.SlashingTx
	stakingTx := btcDel.StakingTx
	stakingMsgTx, err := bbntypes.NewBTCTxFromBytes(stakingTx)
	require.NoError(t, err)

	params := tm.StakingParams

	stakingInfo, err := btcstaking.BuildStakingInfo(
		btcDel.BtcPk.MustToBTCPK(),
		[]*btcec.PublicKey{btcDel.FpBtcPkList[0].MustToBTCPK()},
		params.CovenantPks,
		params.CovenantQuorum,
		uint16(btcDel.EndHeight-btcDel.StartHeight),
		btcutil.Amount(btcDel.TotalSat),
		simnetParams,
	)
	require.NoError(t, err)
	stakingTxUnbondingPathInfo, err := stakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)

	idx, err := bbntypes.GetOutputIdxInBTCTx(stakingMsgTx, stakingInfo.StakingOutput)
	require.NoError(t, err)

	require.NoError(t, err)
	slashingPathInfo, err := stakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)
	// get covenant private key from the keyring
	valEncKey, err := asig.NewEncryptionKeyFromBTCPK(btcDel.FpBtcPkList[0].MustToBTCPK())
	require.NoError(t, err)

	unbondingMsgTx, err := bbntypes.NewBTCTxFromBytes(btcDel.BtcUndelegation.UnbondingTx)
	require.NoError(t, err)
	unbondingInfo, err := btcstaking.BuildUnbondingInfo(
		btcDel.BtcPk.MustToBTCPK(),
		[]*btcec.PublicKey{btcDel.FpBtcPkList[0].MustToBTCPK()},
		params.CovenantPks,
		params.CovenantQuorum,
		uint16(btcDel.UnbondingTime),
		btcutil.Amount(unbondingMsgTx.TxOut[0].Value),
		simnetParams,
	)
	require.NoError(t, err)

	// Covenant 0 signatures
	covenantAdaptorStakingSlashing1, err := slashingTx.EncSign(
		stakingMsgTx,
		idx,
		slashingPathInfo.RevealedLeaf.Script,
		tm.CovenantPrivKeys[0],
		valEncKey,
	)
	require.NoError(t, err)
	covenantUnbondingSig1, err := btcstaking.SignTxWithOneScriptSpendInputFromTapLeaf(
		unbondingMsgTx,
		stakingInfo.StakingOutput,
		tm.CovenantPrivKeys[0],
		stakingTxUnbondingPathInfo.RevealedLeaf,
	)
	require.NoError(t, err)

	// slashing unbonding tx sig
	unbondingTxSlashingPathInfo, err := unbondingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)
	covenantAdaptorUnbondingSlashing1, err := btcDel.BtcUndelegation.SlashingTx.EncSign(
		unbondingMsgTx,
		0,
		unbondingTxSlashingPathInfo.RevealedLeaf.Script,
		tm.CovenantPrivKeys[0],
		valEncKey,
	)
	require.NoError(t, err)

	_, err = tm.BBNClient.SubmitCovenantSigs(
		tm.CovenantPrivKeys[0].PubKey(),
		stakingMsgTx.TxHash().String(),
		[][]byte{covenantAdaptorStakingSlashing1.MustMarshal()},
		covenantUnbondingSig1,
		[][]byte{covenantAdaptorUnbondingSlashing1.MustMarshal()},
	)
	require.NoError(t, err)

	// Covenant 1 signatures
	covenantAdaptorStakingSlashing2, err := slashingTx.EncSign(
		stakingMsgTx,
		idx,
		slashingPathInfo.RevealedLeaf.Script,
		tm.CovenantPrivKeys[1],
		valEncKey,
	)
	require.NoError(t, err)
	covenantUnbondingSig2, err := btcstaking.SignTxWithOneScriptSpendInputFromTapLeaf(
		unbondingMsgTx,
		stakingInfo.StakingOutput,
		tm.CovenantPrivKeys[1],
		stakingTxUnbondingPathInfo.RevealedLeaf,
	)
	require.NoError(t, err)

	// slashing unbonding tx sig

	covenantAdaptorUnbondingSlashing2, err := btcDel.BtcUndelegation.SlashingTx.EncSign(
		unbondingMsgTx,
		0,
		unbondingTxSlashingPathInfo.RevealedLeaf.Script,
		tm.CovenantPrivKeys[1],
		valEncKey,
	)

	require.NoError(t, err)
	_, err = tm.BBNClient.SubmitCovenantSigs(
		tm.CovenantPrivKeys[1].PubKey(),
		stakingMsgTx.TxHash().String(),
		[][]byte{covenantAdaptorStakingSlashing2.MustMarshal()},
		covenantUnbondingSig2,
		[][]byte{covenantAdaptorUnbondingSlashing2.MustMarshal()},
	)
	require.NoError(t, err)
}

func (tm *TestManager) InsertWBTCHeaders(t *testing.T, r *rand.Rand) {
	params, err := tm.BBNClient.QueryStakingParams()
	require.NoError(t, err)
	btcTipResp, err := tm.BBNClient.QueryBtcLightClientTip()
	require.NoError(t, err)
	tipHeader, err := bbntypes.NewBTCHeaderBytesFromHex(btcTipResp.HeaderHex)
	require.NoError(t, err)
	kHeaders := datagen.NewBTCHeaderChainFromParentInfo(r, &btclctypes.BTCHeaderInfo{
		Header: &tipHeader,
		Hash:   tipHeader.Hash(),
		Height: btcTipResp.Height,
		Work:   &btcTipResp.Work,
	}, uint32(params.FinalizationTimeoutBlocks))
	_, err = tm.BBNClient.InsertBtcBlockHeaders(kHeaders.ChainToBytes())
	require.NoError(t, err)
}

func (tm *TestManager) FinalizeUntilEpoch(t *testing.T, epoch uint64) {
	bbnClient := tm.BBNClient.GetBBNClient()

	// wait until the checkpoint of this epoch is sealed
	require.Eventually(t, func() bool {
		lastSealedCkpt, err := bbnClient.LatestEpochFromStatus(ckpttypes.Sealed)
		if err != nil {
			return false
		}
		return epoch <= lastSealedCkpt.RawCheckpoint.EpochNum
	}, eventuallyWaitTimeOut, 1*time.Second)

	t.Logf("start finalizing epochs till %d", epoch)
	// Random source for the generation of BTC data
	r := rand.New(rand.NewSource(time.Now().Unix()))

	// get all checkpoints of these epochs
	pagination := &sdkquerytypes.PageRequest{
		Key:   ckpttypes.CkptsObjectKey(0),
		Limit: epoch,
	}
	resp, err := bbnClient.RawCheckpoints(pagination)
	require.NoError(t, err)
	require.Equal(t, int(epoch), len(resp.RawCheckpoints))

	submitter := tm.BBNClient.GetKeyAddress()

	for _, checkpoint := range resp.RawCheckpoints {
		currentBtcTipResp, err := tm.BBNClient.QueryBtcLightClientTip()
		require.NoError(t, err)
		tipHeader, err := bbntypes.NewBTCHeaderBytesFromHex(currentBtcTipResp.HeaderHex)
		require.NoError(t, err)

		rawCheckpoint, err := checkpoint.Ckpt.ToRawCheckpoint()
		require.NoError(t, err)

		btcCheckpoint, err := ckpttypes.FromRawCkptToBTCCkpt(rawCheckpoint, submitter)
		require.NoError(t, err)

		babylonTagBytes, err := hex.DecodeString("01020304")
		require.NoError(t, err)

		p1, p2, err := txformat.EncodeCheckpointData(
			babylonTagBytes,
			txformat.CurrentVersion,
			btcCheckpoint,
		)
		require.NoError(t, err)

		tx1 := datagen.CreatOpReturnTransaction(r, p1)

		opReturn1 := datagen.CreateBlockWithTransaction(r, tipHeader.ToBlockHeader(), tx1)
		tx2 := datagen.CreatOpReturnTransaction(r, p2)
		opReturn2 := datagen.CreateBlockWithTransaction(r, opReturn1.HeaderBytes.ToBlockHeader(), tx2)

		// insert headers and proofs
		_, err = tm.BBNClient.InsertBtcBlockHeaders([]bbntypes.BTCHeaderBytes{
			opReturn1.HeaderBytes,
			opReturn2.HeaderBytes,
		})
		require.NoError(t, err)

		_, err = tm.BBNClient.InsertSpvProofs(submitter.String(), []*btcctypes.BTCSpvProof{
			opReturn1.SpvProof,
			opReturn2.SpvProof,
		})
		require.NoError(t, err)

		// wait until this checkpoint is submitted
		require.Eventually(t, func() bool {
			ckpt, err := bbnClient.RawCheckpoint(checkpoint.Ckpt.EpochNum)
			if err != nil {
				return false
			}
			return ckpt.RawCheckpoint.Status == ckpttypes.Submitted
		}, eventuallyWaitTimeOut, eventuallyPollTime)
	}

	// insert w BTC headers
	tm.InsertWBTCHeaders(t, r)

	// wait until the checkpoint of this epoch is finalised
	require.Eventually(t, func() bool {
		lastFinalizedCkpt, err := bbnClient.LatestEpochFromStatus(ckpttypes.Finalized)
		if err != nil {
			t.Logf("failed to get last finalized epoch: %v", err)
			return false
		}
		return epoch <= lastFinalizedCkpt.RawCheckpoint.EpochNum
	}, eventuallyWaitTimeOut, 1*time.Second)

	t.Logf("epoch %d is finalised", epoch)
}

func (tm *TestManager) InsertBTCDelegation(t *testing.T, fpPks []*btcec.PublicKey, stakingTime uint16, stakingAmount int64) *TestDelegationData {
	params := tm.StakingParams
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// delegator BTC key pairs, staking tx and slashing tx
	delBtcPrivKey, delBtcPubKey, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)

	unbondingTime := uint16(tm.StakingParams.UnbondingTime)
	testStakingInfo := datagen.GenBTCStakingSlashingInfo(
		r,
		t,
		btcNetworkParams,
		delBtcPrivKey,
		fpPks,
		params.CovenantPks,
		params.CovenantQuorum,
		stakingTime,
		stakingAmount,
		params.SlashingPkScript,
		params.SlashingRate,
		unbondingTime,
	)

	stakerAddr := tm.BBNClient.GetKeyAddress()

	// proof-of-possession
	pop, err := bstypes.NewPoPBTC(stakerAddr, delBtcPrivKey)
	require.NoError(t, err)

	// create and insert BTC headers which include the staking tx to get staking tx info
	btcTipHeaderResp, err := tm.BBNClient.QueryBtcLightClientTip()
	require.NoError(t, err)
	tipHeader, err := bbntypes.NewBTCHeaderBytesFromHex(btcTipHeaderResp.HeaderHex)
	require.NoError(t, err)
	blockWithStakingTx := datagen.CreateBlockWithTransaction(r, tipHeader.ToBlockHeader(), testStakingInfo.StakingTx)
	accumulatedWork := btclctypes.CalcWork(&blockWithStakingTx.HeaderBytes)
	accumulatedWork = btclctypes.CumulativeWork(accumulatedWork, btcTipHeaderResp.Work)
	parentBlockHeaderInfo := &btclctypes.BTCHeaderInfo{
		Header: &blockWithStakingTx.HeaderBytes,
		Hash:   blockWithStakingTx.HeaderBytes.Hash(),
		Height: btcTipHeaderResp.Height + 1,
		Work:   &accumulatedWork,
	}
	headers := make([]bbntypes.BTCHeaderBytes, 0)
	headers = append(headers, blockWithStakingTx.HeaderBytes)
	for i := 0; i < int(params.ComfirmationTimeBlocks); i++ {
		headerInfo := datagen.GenRandomValidBTCHeaderInfoWithParent(r, *parentBlockHeaderInfo)
		headers = append(headers, *headerInfo.Header)
		parentBlockHeaderInfo = headerInfo
	}
	_, err = tm.BBNClient.InsertBtcBlockHeaders(headers)
	require.NoError(t, err)
	btcHeader := blockWithStakingTx.HeaderBytes
	serializedStakingTx, err := bbntypes.SerializeBTCTx(testStakingInfo.StakingTx)
	require.NoError(t, err)
	txInfo := btcctypes.NewTransactionInfo(&btcctypes.TransactionKey{Index: 1, Hash: btcHeader.Hash()}, serializedStakingTx, blockWithStakingTx.SpvProof.MerkleNodes)

	slashignSpendInfo, err := testStakingInfo.StakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	// delegator sig
	delegatorSig, err := testStakingInfo.SlashingTx.Sign(
		testStakingInfo.StakingTx,
		0,
		slashignSpendInfo.GetPkScriptPath(),
		delBtcPrivKey,
	)
	require.NoError(t, err)

	unbondingValue := stakingAmount - 1000
	stakingTxHash := testStakingInfo.StakingTx.TxHash()

	testUnbondingInfo := datagen.GenBTCUnbondingSlashingInfo(
		r,
		t,
		btcNetworkParams,
		delBtcPrivKey,
		fpPks,
		params.CovenantPks,
		params.CovenantQuorum,
		wire.NewOutPoint(&stakingTxHash, 0),
		unbondingTime,
		unbondingValue,
		params.SlashingPkScript,
		params.SlashingRate,
		unbondingTime,
	)

	unbondingTxMsg := testUnbondingInfo.UnbondingTx

	unbondingSlashingPathInfo, err := testUnbondingInfo.UnbondingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	unbondingSig, err := testUnbondingInfo.SlashingTx.Sign(
		unbondingTxMsg,
		0,
		unbondingSlashingPathInfo.GetPkScriptPath(),
		delBtcPrivKey,
	)
	require.NoError(t, err)

	serializedUnbondingTx, err := bbntypes.SerializeBTCTx(testUnbondingInfo.UnbondingTx)
	require.NoError(t, err)

	// submit the BTC delegation to Babylon
	_, err = tm.BBNClient.CreateBTCDelegation(
		bbntypes.NewBIP340PubKeyFromBTCPK(delBtcPubKey),
		fpPks,
		pop,
		uint32(stakingTime),
		stakingAmount,
		txInfo,
		testStakingInfo.SlashingTx,
		delegatorSig,
		serializedUnbondingTx,
		uint32(unbondingTime),
		unbondingValue,
		testUnbondingInfo.SlashingTx,
		unbondingSig,
	)
	require.NoError(t, err)

	t.Log("successfully submitted a BTC delegation")

	return &TestDelegationData{
		DelegatorPrivKey: delBtcPrivKey,
		DelegatorKey:     delBtcPubKey,
		FpPks:            fpPks,
		StakingTx:        testStakingInfo.StakingTx,
		SlashingTx:       testStakingInfo.SlashingTx,
		StakingTxInfo:    txInfo,
		DelegatorSig:     delegatorSig,
		SlashingPkScript: params.SlashingPkScript,
		StakingTime:      stakingTime,
		StakingAmount:    stakingAmount,
	}
}

func defaultFpConfig(keyringDir, homeDir string) *fpcfg.Config {
	cfg := fpcfg.DefaultConfigWithHome(homeDir)

	cfg.NumPubRand = 1000
	cfg.NumPubRandMax = 1000
	cfg.TimestampingDelayBlocks = 0

	cfg.BitcoinNetwork = "simnet"
	cfg.BTCNetParams = chaincfg.SimNetParams

	cfg.PollerConfig.PollInterval = 1 * time.Millisecond
	// babylon configs for sending transactions
	cfg.BabylonConfig.KeyDirectory = keyringDir
	// need to use this one to send otherwise we will have account sequence mismatch
	// errors
	cfg.BabylonConfig.Key = "test-spending-key"
	// Big adjustment to make sure we have enough gas in our transactions
	cfg.BabylonConfig.GasAdjustment = 20

	return &cfg
}

func newDescription(moniker string) *stakingtypes.Description {
	dec := stakingtypes.NewDescription(moniker, "", "", "", "")
	return &dec
}

// ParseRespBTCDelToBTCDel parses an BTC delegation response to BTC Delegation
// adapted from
// https://github.com/babylonlabs-io/babylon/blob/1a3c50da64885452c8d669fcea2a2fad78c8a028/test/e2e/btc_staking_e2e_test.go#L548
func ParseRespBTCDelToBTCDel(resp *bstypes.BTCDelegationResponse) (btcDel *bstypes.BTCDelegation, err error) {
	stakingTx, err := hex.DecodeString(resp.StakingTxHex)
	if err != nil {
		return nil, err
	}

	delSig, err := bbntypes.NewBIP340SignatureFromHex(resp.DelegatorSlashSigHex)
	if err != nil {
		return nil, err
	}

	slashingTx, err := bstypes.NewBTCSlashingTxFromHex(resp.SlashingTxHex)
	if err != nil {
		return nil, err
	}

	btcDel = &bstypes.BTCDelegation{
		// missing BabylonPk, Pop
		// these fields are not sent out to the client on BTCDelegationResponse
		BtcPk:            resp.BtcPk,
		FpBtcPkList:      resp.FpBtcPkList,
		StartHeight:      resp.StartHeight,
		EndHeight:        resp.EndHeight,
		TotalSat:         resp.TotalSat,
		StakingTx:        stakingTx,
		DelegatorSig:     delSig,
		StakingOutputIdx: resp.StakingOutputIdx,
		CovenantSigs:     resp.CovenantSigs,
		UnbondingTime:    resp.UnbondingTime,
		SlashingTx:       slashingTx,
	}

	if resp.UndelegationResponse != nil {
		ud := resp.UndelegationResponse
		unbondTx, err := hex.DecodeString(ud.UnbondingTxHex)
		if err != nil {
			return nil, err
		}

		slashTx, err := bstypes.NewBTCSlashingTxFromHex(ud.SlashingTxHex)
		if err != nil {
			return nil, err
		}

		delSlashingSig, err := bbntypes.NewBIP340SignatureFromHex(ud.DelegatorSlashingSigHex)
		if err != nil {
			return nil, err
		}

		btcDel.BtcUndelegation = &bstypes.BTCUndelegation{
			UnbondingTx:              unbondTx,
			CovenantUnbondingSigList: ud.CovenantUnbondingSigList,
			CovenantSlashingSigs:     ud.CovenantSlashingSigs,
			SlashingTx:               slashTx,
			DelegatorSlashingSig:     delSlashingSig,
		}
	}

	return btcDel, nil
}

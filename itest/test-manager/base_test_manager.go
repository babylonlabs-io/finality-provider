package test_manager

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	bbncc "github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	eotsclient "github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	eotsconfig "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/testutil"
)

type BaseTestManager struct {
	BBNClient        *bbncc.BabylonController
	CovenantPrivKeys []*btcec.PrivateKey
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

	SlashingPkScript string
	ChangeAddr       string
	StakingTime      uint16
	StakingAmount    int64
}

func (tm *BaseTestManager) InsertBTCDelegation(t *testing.T, fpPks []*btcec.PublicKey, stakingTime uint16, stakingAmount int64) *TestDelegationData {
	params, err := tm.BBNClient.QueryStakingParams()
	require.NoError(t, err)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// delegator BTC key pairs, staking tx and slashing tx
	delBtcPrivKey, delBtcPubKey, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)

	testStakingInfo := datagen.GenBTCStakingSlashingInfo(
		r,
		t,
		e2eutils.BtcNetworkParams,
		delBtcPrivKey,
		fpPks,
		params.CovenantPks,
		params.CovenantQuorum,
		stakingTime,
		stakingAmount,
		params.SlashingPkScript,
		params.SlashingRate,
		uint16(params.UnbondingTime),
	)

	stakerAddr := tm.BBNClient.GetKeyAddress()

	// proof-of-possession
	pop, err := datagen.NewPoPBTC(stakerAddr, delBtcPrivKey)
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
		e2eutils.BtcNetworkParams,
		delBtcPrivKey,
		fpPks,
		params.CovenantPks,
		params.CovenantQuorum,
		wire.NewOutPoint(&stakingTxHash, 0),
		uint16(params.UnbondingTime),
		unbondingValue,
		params.SlashingPkScript,
		params.SlashingRate,
		uint16(params.UnbondingTime),
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
		uint32(params.UnbondingTime),
		unbondingValue,
		testUnbondingInfo.SlashingTx,
		unbondingSig)
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
		SlashingPkScript: hex.EncodeToString(params.SlashingPkScript),
		StakingTime:      stakingTime,
		StakingAmount:    stakingAmount,
	}
}

func (tm *BaseTestManager) WaitForNPendingDels(t *testing.T, n int) []*bstypes.BTCDelegationResponse {
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
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

	t.Logf("delegations are pending")

	return dels
}

func (tm *BaseTestManager) WaitForNActiveDels(t *testing.T, n int) []*bstypes.BTCDelegationResponse {
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
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

	t.Logf("delegations are active")

	return dels
}

func (tm *BaseTestManager) WaitForFpPubRandTimestamped(t *testing.T, fpIns *service.FinalityProviderInstance) {
	var lastCommittedHeight uint64
	var err error

	require.Eventually(t, func() bool {
		lastCommittedHeight, err = fpIns.GetLastCommittedHeight()
		if err != nil {
			return false
		}
		return lastCommittedHeight > 0
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

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

// check the BTC delegations are pending
// send covenant sigs to each of the delegations
// check the BTC delegations are active
func (tm *BaseTestManager) WaitForDelegations(t *testing.T, n int) {
	delsResp := tm.WaitForNPendingDels(t, n)
	require.Equal(t, n, len(delsResp))

	for _, delResp := range delsResp {
		d, err := e2eutils.ParseRespBTCDelToBTCDel(delResp)
		require.NoError(t, err)

		// send covenant sigs
		tm.InsertCovenantSigForDelegation(t, d)
	}

	// check the BTC delegations are active
	tm.WaitForNActiveDels(t, n)
}

func (tm *BaseTestManager) InsertWBTCHeaders(t *testing.T, r *rand.Rand) {
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

func (tm *BaseTestManager) InsertCovenantSigForDelegation(t *testing.T, btcDel *bstypes.BTCDelegation) {
	slashingTx := btcDel.SlashingTx
	stakingTx := btcDel.StakingTx
	stakingMsgTx, err := bbntypes.NewBTCTxFromBytes(stakingTx)
	require.NoError(t, err)

	params, err := tm.BBNClient.QueryStakingParams()
	require.NoError(t, err)

	var fpKeys []*btcec.PublicKey
	for _, v := range btcDel.FpBtcPkList {
		fpKeys = append(fpKeys, v.MustToBTCPK())
	}

	stakingInfo, err := btcstaking.BuildStakingInfo(
		btcDel.BtcPk.MustToBTCPK(),
		fpKeys,
		params.CovenantPks,
		params.CovenantQuorum,
		uint16(btcDel.EndHeight-btcDel.StartHeight),
		btcutil.Amount(btcDel.TotalSat),
		e2eutils.BtcNetworkParams,
	)
	require.NoError(t, err)
	stakingTxUnbondingPathInfo, err := stakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)

	idx, err := bbntypes.GetOutputIdxInBTCTx(stakingMsgTx, stakingInfo.StakingOutput)
	require.NoError(t, err)

	slashingPathInfo, err := stakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	var valEncKeys []*asig.EncryptionKey
	for _, v := range btcDel.FpBtcPkList {
		// get covenant private key from the keyring
		valEncKey, err := asig.NewEncryptionKeyFromBTCPK(v.MustToBTCPK())
		require.NoError(t, err)
		valEncKeys = append(valEncKeys, valEncKey)
	}

	unbondingMsgTx, err := bbntypes.NewBTCTxFromBytes(btcDel.BtcUndelegation.UnbondingTx)
	require.NoError(t, err)
	unbondingInfo, err := btcstaking.BuildUnbondingInfo(
		btcDel.BtcPk.MustToBTCPK(),
		fpKeys,
		params.CovenantPks,
		params.CovenantQuorum,
		uint16(btcDel.UnbondingTime),
		btcutil.Amount(unbondingMsgTx.TxOut[0].Value),
		e2eutils.BtcNetworkParams,
	)
	require.NoError(t, err)

	var covenantAdaptorStakingSlashing1List [][]byte
	for _, v := range valEncKeys {
		// Covenant 0 signatures
		covenantAdaptorStakingSlashing1, err := slashingTx.EncSign(
			stakingMsgTx,
			idx,
			slashingPathInfo.RevealedLeaf.Script,
			tm.CovenantPrivKeys[0],
			v,
		)
		require.NoError(t, err)
		covenantAdaptorStakingSlashing1List = append(covenantAdaptorStakingSlashing1List, covenantAdaptorStakingSlashing1.MustMarshal())
	}

	covenantUnbondingSig1, err := btcstaking.SignTxWithOneScriptSpendInputFromTapLeaf(
		unbondingMsgTx,
		stakingInfo.StakingOutput,
		tm.CovenantPrivKeys[0],
		stakingTxUnbondingPathInfo.RevealedLeaf,
	)
	require.NoError(t, err)

	var covenantAdaptorUnbondingSlashing1List [][]byte
	for _, v := range valEncKeys {
		// slashing unbonding tx sig
		unbondingTxSlashingPathInfo, err := unbondingInfo.SlashingPathSpendInfo()
		require.NoError(t, err)
		covenantAdaptorUnbondingSlashing1, err := btcDel.BtcUndelegation.SlashingTx.EncSign(
			unbondingMsgTx,
			0,
			unbondingTxSlashingPathInfo.RevealedLeaf.Script,
			tm.CovenantPrivKeys[0],
			v,
		)
		require.NoError(t, err)
		covenantAdaptorUnbondingSlashing1List = append(covenantAdaptorUnbondingSlashing1List, covenantAdaptorUnbondingSlashing1.MustMarshal())
	}

	_, err = tm.BBNClient.SubmitCovenantSigs(
		tm.CovenantPrivKeys[0].PubKey(),
		stakingMsgTx.TxHash().String(),
		covenantAdaptorStakingSlashing1List,
		covenantUnbondingSig1,
		covenantAdaptorUnbondingSlashing1List,
	)
	require.NoError(t, err)

	var covenantAdaptorStakingSlashing2List [][]byte
	for _, v := range valEncKeys {
		// Covenant 1 signatures
		covenantAdaptorStakingSlashing2, err := slashingTx.EncSign(
			stakingMsgTx,
			idx,
			slashingPathInfo.RevealedLeaf.Script,
			tm.CovenantPrivKeys[1],
			v,
		)
		require.NoError(t, err)
		covenantAdaptorStakingSlashing2List = append(covenantAdaptorStakingSlashing2List, covenantAdaptorStakingSlashing2.MustMarshal())
	}

	covenantUnbondingSig2, err := btcstaking.SignTxWithOneScriptSpendInputFromTapLeaf(
		unbondingMsgTx,
		stakingInfo.StakingOutput,
		tm.CovenantPrivKeys[1],
		stakingTxUnbondingPathInfo.RevealedLeaf,
	)
	require.NoError(t, err)

	var covenantAdaptorUnbondingSlashing2List [][]byte
	for _, v := range valEncKeys {
		// slashing unbonding tx sig
		unbondingTxSlashingPathInfo, err := unbondingInfo.SlashingPathSpendInfo()
		require.NoError(t, err)
		covenantAdaptorUnbondingSlashing2, err := btcDel.BtcUndelegation.SlashingTx.EncSign(
			unbondingMsgTx,
			0,
			unbondingTxSlashingPathInfo.RevealedLeaf.Script,
			tm.CovenantPrivKeys[1],
			v,
		)
		require.NoError(t, err)
		covenantAdaptorUnbondingSlashing2List = append(covenantAdaptorUnbondingSlashing2List, covenantAdaptorUnbondingSlashing2.MustMarshal())
	}

	_, err = tm.BBNClient.SubmitCovenantSigs(
		tm.CovenantPrivKeys[1].PubKey(),
		stakingMsgTx.TxHash().String(),
		covenantAdaptorStakingSlashing2List,
		covenantUnbondingSig2,
		covenantAdaptorUnbondingSlashing2List,
	)
	require.NoError(t, err)
}

func (tm *BaseTestManager) FinalizeUntilEpoch(t *testing.T, epoch uint64) {
	bbnClient := tm.BBNClient.GetBBNClient()

	// wait until the checkpoint of this epoch is sealed
	require.Eventually(t, func() bool {
		lastSealedCkpt, err := bbnClient.LatestEpochFromStatus(ckpttypes.Sealed)
		if err != nil {
			return false
		}

		return epoch <= lastSealedCkpt.RawCheckpoint.EpochNum
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

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
		}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
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
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

	t.Logf("epoch %d is finalised", epoch)
}

func StartEotsManagers(
	t *testing.T,
	ctx context.Context,
	logger *zap.Logger,
	testDir string,
	babylonFpCfg *fpcfg.Config,
	consumerFpCfg *fpcfg.Config,
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

	eh := e2eutils.NewEOTSServerHandler(t, eotsConfigs[0], eotsHomeDirs[0])
	eh.Start(ctx)

	// create EOTS clients
	for i := 0; i < len(fpCfgs); i++ {
		// wait for EOTS servers to start
		// see https://github.com/babylonchain/finality-provider/pull/517
		var eotsCli *eotsclient.EOTSManagerGRpcClient
		var err error
		require.Eventually(t, func() bool {
			eotsCli, err = eotsclient.NewEOTSManagerGRpcClient(fpCfgs[i].EOTSManagerAddress, fpCfgs[i].HMACKey)
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

func CreateAndStartFpApp(
	t *testing.T,
	logger *zap.Logger,
	cfg *fpcfg.Config,
	cc api.ConsumerController,
	eotsCli *client.EOTSManagerGRpcClient,
) *service.FinalityProviderApp {
	bc, err := fpcc.NewBabylonController(cfg, logger)
	require.NoError(t, err)

	fpdb, err := cfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)

	fpMetrics := metrics.NewFpMetrics()
	poller := service.NewChainPoller(logger, cfg.PollerConfig, cc, fpMetrics)

	fpApp, err := service.NewFinalityProviderApp(cfg, bc, cc, eotsCli, poller, fpMetrics, fpdb, logger)
	require.NoError(t, err)

	err = fpApp.Start()
	require.NoError(t, err)

	return fpApp
}

func CreateAndRegisterFinalityProvider(t *testing.T, fpApp *service.FinalityProviderApp, chainId string, eotsPk *bbntypes.BIP340PubKey) {
	fpCfg := fpApp.GetConfig()
	keyName := fpCfg.BabylonConfig.Key
	moniker := fmt.Sprintf("%s-%s", chainId, e2eutils.MonikerPrefix)
	commission := testutil.ZeroCommissionRate()
	desc := e2eutils.NewDescription(moniker)

	_, err := fpApp.CreateFinalityProvider(
		keyName,
		chainId,
		eotsPk,
		desc,
		commission,
	)
	require.NoError(t, err)
}

func TempDir(t *testing.T, pattern string) (string, error) {
	tempName, err := os.MkdirTemp(os.TempDir(), pattern)
	if err != nil {
		return "", err
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(tempName)
	})

	if err = os.Chmod(tempName, 0755); err != nil {
		return "", err
	}

	return tempName, nil
}

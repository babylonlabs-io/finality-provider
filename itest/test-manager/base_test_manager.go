package test_manager

import (
	"math/rand"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/btcstaking"
	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	bbncc "github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	"github.com/babylonlabs-io/finality-provider/types"
)

type BaseTestManager struct {
	BBNClient        *bbncc.BabylonController
	StakingParams    *types.StakingParams
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

	SlashingAddr  string
	ChangeAddr    string
	StakingTime   uint16
	StakingAmount int64
}

func (tm *BaseTestManager) InsertBTCDelegation(t *testing.T, fpPks []*btcec.PublicKey, stakingTime uint16, stakingAmount int64) *TestDelegationData {
	params := tm.StakingParams
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// delegator BTC key pairs, staking tx and slashing tx
	delBtcPrivKey, delBtcPubKey, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)

	unbondingTime := uint16(tm.StakingParams.MinimumUnbondingTime()) + 1
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
		params.SlashingAddress.String(),
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
		e2eutils.BtcNetworkParams,
		delBtcPrivKey,
		fpPks,
		params.CovenantPks,
		params.CovenantQuorum,
		wire.NewOutPoint(&stakingTxHash, 0),
		unbondingTime,
		unbondingValue,
		params.SlashingAddress.String(),
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
		SlashingAddr:     params.SlashingAddress.String(),
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

func (tm *BaseTestManager) InsertCovenantSigForDelegation(t *testing.T, btcDel *bstypes.BTCDelegation) {
	slashingTx := btcDel.SlashingTx
	stakingTx := btcDel.StakingTx
	stakingMsgTx, err := bbntypes.NewBTCTxFromBytes(stakingTx)
	require.NoError(t, err)

	params := tm.StakingParams

	var fpKeys []*btcec.PublicKey
	for _, v := range btcDel.FpBtcPkList {
		fpKeys = append(fpKeys, v.MustToBTCPK())
	}

	stakingInfo, err := btcstaking.BuildStakingInfo(
		btcDel.BtcPk.MustToBTCPK(),
		fpKeys,
		params.CovenantPks,
		params.CovenantQuorum,
		btcDel.GetStakingTime(),
		btcutil.Amount(btcDel.TotalSat),
		e2eutils.BtcNetworkParams,
	)
	require.NoError(t, err)
	stakingTxUnbondingPathInfo, err := stakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)

	idx, err := bbntypes.GetOutputIdxInBTCTx(stakingMsgTx, stakingInfo.StakingOutput)
	require.NoError(t, err)

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

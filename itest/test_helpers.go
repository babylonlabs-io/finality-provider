package e2e_utils

import (
	"math/rand"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/babylonlabs-io/babylon/crypto/eots"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	ftypes "github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/cosmwasm"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/cometbft/cometbft/crypto/merkle"
)

func GenBtcStakingExecMsg(fpHex string) cosmwasm.ExecMsg {
	// generate random delegation and finality provider
	_, newDel := genRandomBtcDelegation()
	newFp := genRandomFinalityProvider()

	// replace fields so delegation and finality provider are linked
	newFp.BTCPKHex = fpHex
	newDel.FpBtcPkList = []string{fpHex}

	// create the ExecMsg instance with BtcStaking set
	executeMessage := cosmwasm.ExecMsg{
		BtcStaking: &cosmwasm.BtcStaking{
			NewFP:       []cosmwasm.NewFinalityProvider{newFp},
			ActiveDel:   []cosmwasm.ActiveBtcDelegation{newDel},
			SlashedDel:  []cosmwasm.SlashedBtcDelegation{},
			UnbondedDel: []cosmwasm.UnbondedBtcDelegation{},
		},
	}

	return executeMessage
}

func GenBtcStakingFpExecMsg(fpPKHex string) cosmwasm.ExecMsg {
	// generate random finality provider
	newFp := genRandomFinalityProvider()

	// replace field so finality provider is linked
	newFp.BTCPKHex = fpPKHex

	// create the ExecMsg instance with BtcStaking set for NewFP
	executeMessage := cosmwasm.ExecMsg{
		BtcStaking: &cosmwasm.BtcStaking{
			NewFP:       []cosmwasm.NewFinalityProvider{newFp},
			ActiveDel:   []cosmwasm.ActiveBtcDelegation{},
			SlashedDel:  []cosmwasm.SlashedBtcDelegation{},
			UnbondedDel: []cosmwasm.UnbondedBtcDelegation{},
		},
	}

	return executeMessage
}

func GenBtcStakingDelExecMsg(fpHex string) cosmwasm.ExecMsg {
	// generate random delegation
	_, newDel := genRandomBtcDelegation()

	// replace field so delegation is linked to finality provider
	newDel.FpBtcPkList = []string{fpHex}

	// create the ExecMsg instance with BtcStaking set for ActiveDel
	executeMessage := cosmwasm.ExecMsg{
		BtcStaking: &cosmwasm.BtcStaking{
			NewFP:       []cosmwasm.NewFinalityProvider{},
			ActiveDel:   []cosmwasm.ActiveBtcDelegation{newDel},
			SlashedDel:  []cosmwasm.SlashedBtcDelegation{},
			UnbondedDel: []cosmwasm.UnbondedBtcDelegation{},
		},
	}

	return executeMessage
}

func GenPubRandomnessExecMsg(fpHex string, commitment, sig []byte, startHeight, numPubRand uint64) cosmwasm.ExecMsg {
	// create the ExecMsg instance with CommitPublicRandomness set
	executeMessage := cosmwasm.ExecMsg{
		CommitPublicRandomness: &cosmwasm.CommitPublicRandomness{
			FPPubKeyHex: fpHex,
			StartHeight: startHeight,
			NumPubRand:  numPubRand,
			Commitment:  commitment,
			Signature:   sig,
		},
	}

	return executeMessage
}

func GenFinalitySigExecMsg(startHeight, blockHeight uint64, randListInfo *datagen.RandListInfo, sk *btcec.PrivateKey) cosmwasm.ExecMsg {
	fmsg := genAddFinalitySig(startHeight, blockHeight, randListInfo, sk)
	msg := cosmwasm.ExecMsg{
		SubmitFinalitySignature: &cosmwasm.SubmitFinalitySignature{
			FpPubkeyHex: fmsg.FpBtcPk.MarshalHex(),
			Height:      fmsg.BlockHeight,
			PubRand:     fmsg.PubRand.MustMarshal(),
			Proof:       *fmsg.Proof,
			BlockHash:   fmsg.BlockAppHash,
			Signature:   fmsg.FinalitySig.MustMarshal(),
		},
	}

	return msg
}

func genRandomFinalityProvider() cosmwasm.NewFinalityProvider {
	return cosmwasm.NewFinalityProvider{
		Description: &cosmwasm.FinalityProviderDescription{
			Moniker:         "fp1",
			Identity:        "Finality Provider 1",
			Website:         "https://fp1.com",
			SecurityContact: "security_contact",
			Details:         "details",
		},
		Commission: "0.05",
		Addr:       datagen.GenRandomAccount().Address,
		BTCPKHex:   "1",
		Pop: &cosmwasm.ProofOfPossessionBtc{
			BTCSigType: 0,
			BTCSig:     []byte("mock_btc_sig"),
		},
		ConsumerID: "osmosis-1",
	}
}

func genRandomBtcDelegation() (*bstypes.Params, cosmwasm.ActiveBtcDelegation) {
	var net = &chaincfg.RegressionNetParams
	r := rand.New(rand.NewSource(time.Now().Unix()))
	t := &testing.T{}

	delSK, _, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)

	// restaked to a random number of finality providers
	numRestakedFPs := int(datagen.RandomInt(r, 10) + 1)
	_, fpPKs, err := datagen.GenRandomBTCKeyPairs(r, numRestakedFPs)
	require.NoError(t, err)
	fpBTCPKs := bbn.NewBIP340PKsFromBTCPKs(fpPKs)

	// (3, 5) covenant committee
	covenantSKs, covenantPKs, err := datagen.GenRandomBTCKeyPairs(r, 5)
	require.NoError(t, err)
	covenantQuorum := uint32(3)

	stakingTimeBlocks := uint16(50000)
	stakingValue := int64(2 * 10e8)
	slashingAddress, err := datagen.GenRandomBTCAddress(r, net)
	require.NoError(t, err)
	slashingPkScript, err := txscript.PayToAddrScript(slashingAddress)
	require.NoError(t, err)

	slashingRate := sdkmath.LegacyNewDecWithPrec(int64(datagen.RandomInt(r, 41)+10), 2)
	unbondingTime := uint16(100) + 1
	slashingChangeLockTime := unbondingTime

	bsParams := &bstypes.Params{
		CovenantPks:      bbn.NewBIP340PKsFromBTCPKs(covenantPKs),
		CovenantQuorum:   covenantQuorum,
		SlashingPkScript: slashingPkScript,
	}

	// only the quorum of signers provided the signatures
	covenantSigners := covenantSKs[:covenantQuorum]

	// construct the BTC delegation with everything
	btcDel, err := datagen.GenRandomBTCDelegation(
		r,
		t,
		net,
		fpBTCPKs,
		delSK,
		covenantSigners,
		covenantPKs,
		covenantQuorum,
		slashingPkScript,
		1,
		uint64(1000+stakingTimeBlocks),
		uint64(stakingValue),
		slashingRate,
		slashingChangeLockTime,
	)
	require.NoError(t, err)

	activeDel := convertBTCDelegationToActiveBtcDelegation(btcDel)
	return bsParams, activeDel
}

func convertBTCDelegationToActiveBtcDelegation(mockDel *bstypes.BTCDelegation) cosmwasm.ActiveBtcDelegation {
	var fpBtcPkList []string
	for _, pk := range mockDel.FpBtcPkList {
		fpBtcPkList = append(fpBtcPkList, pk.MarshalHex())
	}

	var covenantSigs []cosmwasm.CovenantAdaptorSignatures
	for _, cs := range mockDel.CovenantSigs {
		covenantSigs = append(covenantSigs, cosmwasm.CovenantAdaptorSignatures{
			CovPK:       cs.CovPk.MustMarshal(),
			AdaptorSigs: cs.AdaptorSigs,
		})
	}

	var covenantUnbondingSigs []cosmwasm.SignatureInfo
	for _, sigInfo := range mockDel.BtcUndelegation.CovenantUnbondingSigList {
		covenantUnbondingSigs = append(covenantUnbondingSigs, cosmwasm.SignatureInfo{
			PK:  sigInfo.Pk.MustMarshal(),
			Sig: sigInfo.Sig.MustMarshal(),
		})
	}

	var covenantSlashingSigs []cosmwasm.CovenantAdaptorSignatures
	for _, cs := range mockDel.BtcUndelegation.CovenantSlashingSigs {
		covenantSlashingSigs = append(covenantSlashingSigs, cosmwasm.CovenantAdaptorSignatures{
			CovPK:       cs.CovPk.MustMarshal(),
			AdaptorSigs: cs.AdaptorSigs,
		})
	}

	undelegationInfo := cosmwasm.BtcUndelegationInfo{
		UnbondingTx:           mockDel.BtcUndelegation.UnbondingTx,
		SlashingTx:            mockDel.BtcUndelegation.SlashingTx.MustMarshal(),
		DelegatorSlashingSig:  mockDel.BtcUndelegation.DelegatorSlashingSig.MustMarshal(),
		DelegatorUnbondingSig: make([]byte, 0), // FIXME: Add delegator unbonding sig proper
		CovenantUnbondingSigs: covenantUnbondingSigs,
		CovenantSlashingSigs:  covenantSlashingSigs,
	}

	return cosmwasm.ActiveBtcDelegation{
		StakerAddr:           mockDel.StakerAddr,
		BTCPkHex:             mockDel.BtcPk.MarshalHex(),
		FpBtcPkList:          fpBtcPkList,
		StartHeight:          mockDel.StartHeight,
		EndHeight:            mockDel.EndHeight,
		TotalSat:             mockDel.TotalSat,
		StakingTx:            mockDel.StakingTx,
		SlashingTx:           mockDel.SlashingTx.MustMarshal(),
		DelegatorSlashingSig: mockDel.DelegatorSig.MustMarshal(),
		CovenantSigs:         covenantSigs,
		StakingOutputIdx:     mockDel.StakingOutputIdx,
		UnbondingTime:        mockDel.UnbondingTime,
		UndelegationInfo:     undelegationInfo,
		ParamsVersion:        mockDel.ParamsVersion,
	}
}

func GenCommitPubRandListMsg(r *rand.Rand, fpSk *btcec.PrivateKey, startHeight uint64, numPubRand uint64) (*datagen.RandListInfo, *ftypes.MsgCommitPubRandList, error) {
	randListInfo, err := genRandomPubRandList(r, numPubRand)
	if err != nil {
		return nil, nil, err
	}
	msg := &ftypes.MsgCommitPubRandList{
		Signer:      datagen.GenRandomAccount().Address,
		FpBtcPk:     bbn.NewBIP340PubKeyFromBTCPK(fpSk.PubKey()),
		StartHeight: startHeight,
		NumPubRand:  numPubRand,
		Commitment:  randListInfo.Commitment,
	}
	hash, err := msg.HashToSign()
	if err != nil {
		return nil, nil, err
	}
	schnorrSig, err := schnorr.Sign(fpSk, hash)
	if err != nil {
		panic(err)
	}
	msg.Sig = bbn.NewBIP340SignatureFromBTCSig(schnorrSig)

	return randListInfo, msg, nil
}

func genRandomPubRandList(r *rand.Rand, numPubRand uint64) (*datagen.RandListInfo, error) {
	// generate a list of secret/public randomness
	var srList []*eots.PrivateRand
	var prList []bbn.SchnorrPubRand
	for i := uint64(0); i < numPubRand; i++ {
		eotsSR, eotsPR, err := eots.RandGen(r)
		if err != nil {
			return nil, err
		}
		pr := bbn.NewSchnorrPubRandFromFieldVal(eotsPR)
		srList = append(srList, eotsSR)
		prList = append(prList, *pr)
	}

	var prByteList [][]byte
	for i := range prList {
		prByteList = append(prByteList, prList[i])
	}

	// generate the commitment to these public randomness
	commitment, proofList := merkle.ProofsFromByteSlices(prByteList)

	return &datagen.RandListInfo{SRList: srList, PRList: prList, Commitment: commitment, ProofList: proofList}, nil
}

func genAddFinalitySig(startHeight uint64, blockHeight uint64, randListInfo *datagen.RandListInfo, sk *btcec.PrivateKey) *ftypes.MsgAddFinalitySig {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	blockHash := datagen.GenRandomByteArray(r, 32)

	signer := datagen.GenRandomAccount().Address
	msg, err := newMsgAddFinalitySig(signer, sk, startHeight, blockHeight, randListInfo, blockHash)
	if err != nil {
		panic(err)
	}

	return msg
}

func newMsgAddFinalitySig(
	signer string,
	sk *btcec.PrivateKey,
	startHeight uint64,
	blockHeight uint64,
	randListInfo *datagen.RandListInfo,
	blockAppHash []byte,
) (*ftypes.MsgAddFinalitySig, error) {
	idx := blockHeight - startHeight

	msg := &ftypes.MsgAddFinalitySig{
		Signer:       signer,
		FpBtcPk:      bbn.NewBIP340PubKeyFromBTCPK(sk.PubKey()),
		PubRand:      &randListInfo.PRList[idx],
		Proof:        randListInfo.ProofList[idx].ToProto(),
		BlockHeight:  blockHeight,
		BlockAppHash: blockAppHash,
		FinalitySig:  nil,
	}
	msgToSign := msg.MsgToSign()
	sig, err := eots.Sign(sk, randListInfo.SRList[idx], msgToSign)
	if err != nil {
		return nil, err
	}
	msg.FinalitySig = bbn.NewSchnorrEOTSSigFromModNScalar(sig)

	return msg, nil
}

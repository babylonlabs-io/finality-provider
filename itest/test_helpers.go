package e2e_utils

import (
	"math/rand"
	"time"

	"github.com/babylonlabs-io/babylon/v4/crypto/eots"
	"github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/v4/types"
	ftypes "github.com/babylonlabs-io/babylon/v4/x/finality/types"
	"github.com/btcsuite/btcd/btcec/v2"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cometbft/cometbft/crypto/merkle"
)

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

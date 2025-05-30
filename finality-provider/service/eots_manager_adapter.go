package service

import (
	"fmt"
	"strings"

	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cometbft/cometbft/crypto/tmhash"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	"github.com/babylonlabs-io/finality-provider/types"
)

const failedPreconditionErrStr = "FailedPrecondition"

// InitEOTSManagerClient initializes an EOTS manager client with HMAC authentication
func InitEOTSManagerClient(address string, hmacKey string) (eotsmanager.EOTSManager, error) {
	return client.NewEOTSManagerGRpcClient(address, hmacKey)
}

func (fp *FinalityProviderInstance) GetPubRandList(startHeight uint64, numPubRand uint32) ([]*btcec.FieldVal, error) {
	pubRandList, err := fp.em.CreateRandomnessPairList(
		fp.btcPk.MustMarshal(),
		fp.GetChainID(),
		startHeight,
		numPubRand,
	)
	if err != nil {
		return nil, err
	}

	return pubRandList, nil
}

func getHashToSignForCommitPubRand(startHeight uint64, numPubRand uint64, commitment []byte) ([]byte, error) {
	hasher := tmhash.New()
	if _, err := hasher.Write(sdk.Uint64ToBigEndian(startHeight)); err != nil {
		return nil, err
	}
	if _, err := hasher.Write(sdk.Uint64ToBigEndian(numPubRand)); err != nil {
		return nil, err
	}
	if _, err := hasher.Write(commitment); err != nil {
		return nil, err
	}

	return hasher.Sum(nil), nil
}

func (fp *FinalityProviderInstance) SignPubRandCommit(startHeight uint64, numPubRand uint64, commitment []byte) (*schnorr.Signature, error) {
	hash, err := getHashToSignForCommitPubRand(startHeight, numPubRand, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the commit public randomness message: %w", err)
	}

	// sign the message hash using the finality-provider's BTC private key
	return fp.em.SignSchnorrSig(fp.btcPk.MustMarshal(), hash)
}

func getMsgToSignForVote(blockHeight uint64, blockHash []byte) []byte {
	return append(sdk.Uint64ToBigEndian(blockHeight), blockHash...)
}

func (fp *FinalityProviderInstance) SignFinalitySig(b *types.BlockInfo) (*bbntypes.SchnorrEOTSSig, error) {
	// build proper finality signature request
	msgToSign := getMsgToSignForVote(b.Height, b.Hash)
	sig, err := fp.em.SignEOTS(fp.btcPk.MustMarshal(), fp.GetChainID(), msgToSign, b.Height)
	if err != nil {
		if strings.Contains(err.Error(), failedPreconditionErrStr) {
			return nil, ErrFailedPrecondition
		}

		return nil, fmt.Errorf("failed to sign EOTS: %w", err)
	}

	return bbntypes.NewSchnorrEOTSSigFromModNScalar(sig), nil
}

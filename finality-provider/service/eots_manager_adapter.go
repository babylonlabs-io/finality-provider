package service

import (
	"context"
	"fmt"
	"strings"

	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
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
	client, err := client.NewEOTSManagerGRpcClient(address, hmacKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create EOTS manager client: %w", err)
	}

	return client, nil
}

func (fp *FinalityProviderInstance) GetPubRandList(startHeight uint64, numPubRand uint32) ([]*btcec.FieldVal, error) {
	pubRandList, err := fp.em.CreateRandomnessPairList(
		fp.btcPk.MustMarshal(),
		fp.GetChainID(),
		startHeight,
		numPubRand,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create randomness pair list: %w", err)
	}

	return pubRandList, nil
}

func getHashToSignForCommitPubRandWithContext(signingContext string, startHeight, numPubRand uint64, commitment []byte) ([]byte, error) {
	hasher := tmhash.New()
	if len(signingContext) > 0 {
		if _, err := hasher.Write([]byte(signingContext)); err != nil {
			return nil, fmt.Errorf("failed to write signing context to hasher: %w", err)
		}
	}

	if _, err := hasher.Write(sdk.Uint64ToBigEndian(startHeight)); err != nil {
		return nil, fmt.Errorf("failed to write start height to hasher: %w", err)
	}
	if _, err := hasher.Write(sdk.Uint64ToBigEndian(numPubRand)); err != nil {
		return nil, fmt.Errorf("failed to write number of public randomness to hasher: %w", err)
	}
	if _, err := hasher.Write(commitment); err != nil {
		return nil, fmt.Errorf("failed to write commitment to hasher: %w", err)
	}

	return hasher.Sum(nil), nil
}

func (fp *FinalityProviderInstance) SignPubRandCommit(ctx context.Context, startHeight uint64, numPubRand uint64, commitment []byte) (*schnorr.Signature, error) {
	var (
		hash []byte
		err  error
	)

	latestHeight, err := LatestBlockHeightWithRetry(ctx, fp.consumerCon, fp.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to query the latest block: %w", err)
	}

	// For BSNs we always use the ctx signing
	if fp.consumerCon.IsBSN() || latestHeight >= fp.cfg.ContextSigningHeight {
		signCtx := fp.consumerCon.GetFpRandCommitContext()
		hash, err = getHashToSignForCommitPubRandWithContext(signCtx, startHeight, numPubRand, commitment)
		if err != nil {
			return nil, fmt.Errorf("failed to sign the commit public randomness message: %w", err)
		}
	} else {
		hash, err = getHashToSignForCommitPubRandWithContext("", startHeight, numPubRand, commitment)
		if err != nil {
			return nil, fmt.Errorf("failed to sign the commit public randomness message: %w", err)
		}
	}

	// sign the message hash using the finality-provider's BTC private key
	sig, err := fp.em.SignSchnorrSig(fp.btcPk.MustMarshal(), hash)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the commit public randomness message: %w", err)
	}

	return sig, nil
}

func (fp *FinalityProviderInstance) SignFinalitySig(ctx context.Context, b types.BlockDescription) (*bbntypes.SchnorrEOTSSig, error) {
	latestHeight, err := LatestBlockHeightWithRetry(ctx, fp.consumerCon, fp.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to query the latest block: %w", err)
	}
	// build proper finality signature request
	var msgToSign []byte
	// For BSNs we always use the ctx signing
	if fp.consumerCon.IsBSN() || latestHeight >= fp.cfg.ContextSigningHeight {
		signCtx := fp.consumerCon.GetFpFinVoteContext()
		msgToSign = b.MsgToSign(signCtx)
	} else {
		msgToSign = b.MsgToSign("")
	}

	sig, err := fp.em.SignEOTS(fp.btcPk.MustMarshal(), fp.GetChainID(), msgToSign, b.GetHeight())
	if err != nil {
		if strings.Contains(err.Error(), failedPreconditionErrStr) {
			return nil, ErrFailedPrecondition
		}

		return nil, fmt.Errorf("failed to sign EOTS: %w", err)
	}

	return bbntypes.NewSchnorrEOTSSigFromModNScalar(sig), nil
}

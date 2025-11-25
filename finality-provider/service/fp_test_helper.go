package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	bbntypes "github.com/babylonlabs-io/babylon/v4/types"
	ftypes "github.com/babylonlabs-io/babylon/v4/x/finality/types"
	ccapi "github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/gogo/protobuf/jsonpb"
	"go.uber.org/zap"
)

// FinalityProviderTestHelper provides testing utilities for FinalityProviderInstance
// This struct is designed to be used in testing/devops scenarios and should not be used in production
type FinalityProviderTestHelper struct {
	fp *FinalityProviderInstance
}

// NewFinalityProviderTestHelper creates a new test helper for the given FinalityProviderInstance
func NewFinalityProviderTestHelper(fp *FinalityProviderInstance) *FinalityProviderTestHelper {
	return &FinalityProviderTestHelper{
		fp: fp,
	}
}

// CommitPubRand is exposed for devops/testing purpose to allow manual committing public randomness in cases
// where FP is stuck due to lack of public randomness.
//
// Note:
// - this function is similar to the internal CommitPubRand but should not be used in the main pubrand submission loop.
// - it will always start from the last committed height + 1
// - if targetBlockHeight is too large, it will commit multiple fp.cfg.NumPubRand pairs in a loop until reaching the targetBlockHeight
func (th *FinalityProviderTestHelper) CommitPubRand(ctx context.Context, targetBlockHeight uint64) error {
	var startHeight uint64
	lastCommittedHeight, err := th.fp.GetLastCommittedHeight(ctx)
	if err != nil {
		return err
	}

	if lastCommittedHeight >= targetBlockHeight {
		return fmt.Errorf(
			"finality provider has already committed pubrand to target block height (pk: %s, target: %d, last committed: %d)",
			th.fp.GetBtcPkHex(),
			targetBlockHeight,
			lastCommittedHeight,
		)
	}

	if lastCommittedHeight == uint64(0) {
		// Note: it can also be the case that the finality-provider has committed 1 pubrand before (but in practice, we
		// will never set cfg.NumPubRand to 1. so we can safely assume it has never committed before)
		startHeight = 0
	} else {
		startHeight = lastCommittedHeight + 1
	}

	return th.CommitPubRandWithStartHeight(ctx, startHeight, targetBlockHeight)
}

// CommitPubRandWithStartHeight is exposed for devops/testing purpose to allow manual committing public randomness
// in cases where FP is stuck due to lack of public randomness.
func (th *FinalityProviderTestHelper) CommitPubRandWithStartHeight(ctx context.Context, startHeight uint64, targetBlockHeight uint64) error {
	if startHeight > targetBlockHeight {
		return fmt.Errorf("start height should not be greater than target block height")
	}

	var lastCommittedHeight uint64
	lastCommittedHeight, err := th.fp.GetLastCommittedHeight(ctx)
	if err != nil {
		return err
	}
	if lastCommittedHeight >= startHeight {
		return fmt.Errorf(
			"finality provider has already committed pubrand at the start height (pk: %s, startHeight: %d, lastCommittedHeight: %d)",
			th.fp.GetBtcPkHex(),
			startHeight,
			lastCommittedHeight,
		)
	}

	th.fp.logger.Info("Start committing pubrand from block height", zap.Uint64("start_height", startHeight))

	for startHeight <= targetBlockHeight {
		_, err = th.fp.CommitPubRand(ctx, startHeight)
		if err != nil {
			return err
		}
		lastCommittedHeight = startHeight + uint64(th.fp.cfg.NumPubRand) - 1
		startHeight = lastCommittedHeight + 1
		th.fp.logger.Info("Committed pubrand to block height", zap.Uint64("height", lastCommittedHeight))
	}

	// no error. success
	return nil
}

// SubmitFinalitySignatureAndExtractPrivKey is exposed for presentation/testing purpose to allow manual sending finality signature
// this API is the same as SubmitBatchFinalitySignatures except that we don't constraint the voting height and update status
// Note: this should not be used in the submission loop
func (th *FinalityProviderTestHelper) SubmitFinalitySignatureAndExtractPrivKey(
	ctx context.Context,
	b *types.BlockInfo,
	useSafeEOTSFunc bool,
) (*types.TxResponse, *btcec.PrivateKey, error) {
	// get public randomness
	prList, err := th.fp.GetPubRandList(b.GetHeight(), 1)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get public randomness list: %w", err)
	}
	pubRand := prList[0]

	// get proof
	proofBytes, err := th.fp.pubRandState.getPubRandProof(th.fp.btcPk.MustMarshal(), th.fp.GetChainID(), b.GetHeight())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get public randomness inclusion proof: %w", err)
	}

	eotsSignerFunc := func(_ context.Context, b types.BlockDescription) (*bbntypes.SchnorrEOTSSig, error) {
		sig, err := th.fp.em.UnsafeSignEOTS(th.fp.btcPk.MustMarshal(), th.fp.GetChainID(), b.MsgToSign(""), b.GetHeight())
		if err != nil {
			return nil, fmt.Errorf("failed to sign EOTS: %w", err)
		}

		return bbntypes.NewSchnorrEOTSSigFromModNScalar(sig), nil
	}

	if useSafeEOTSFunc {
		eotsSignerFunc = th.fp.SignFinalitySig
	}

	// sign block
	eotsSig, err := eotsSignerFunc(ctx, b)
	if err != nil {
		return nil, nil, err
	}

	// send finality signature to the consumer chain
	res, err := th.fp.consumerCon.SubmitBatchFinalitySigs(ctx, &ccapi.SubmitBatchFinalitySigsRequest{
		FpPk:        th.fp.GetBtcPk(),
		Blocks:      []types.BlockDescription{b},
		PubRandList: []*btcec.FieldVal{pubRand},
		ProofList:   [][]byte{proofBytes},
		Sigs:        []*btcec.ModNScalar{eotsSig.ToModNScalar()},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send finality signature to the consumer chain: %w", err)
	}

	if res.TxHash == "" {
		return res, nil, nil
	}

	// try to extract the private key
	var privKey *btcec.PrivateKey
	for _, ev := range res.Events {
		if strings.Contains(ev.EventType, "EventSlashedFinalityProvider") {
			evidenceStr := ev.Attributes["evidence"]
			th.fp.logger.Debug("found slashing evidence")
			var evidence ftypes.Evidence
			if err := jsonpb.UnmarshalString(evidenceStr, &evidence); err != nil {
				return nil, nil, fmt.Errorf("failed to decode evidence bytes to evidence: %s", err.Error())
			}
			privKey, err = evidence.ExtractBTCSK()
			if err != nil {
				return nil, nil, fmt.Errorf("failed to extract private key: %s", err.Error())
			}

			break
		}
	}

	return res, privKey, nil
}

// GetFinalityProviderInstance returns the underlying FinalityProviderInstance
// This can be useful for accessing other methods or properties if needed
func (th *FinalityProviderTestHelper) GetFinalityProviderInstance() *FinalityProviderInstance {
	return th.fp
}

func (th *FinalityProviderTestHelper) SubmitBatchFinalitySignatures(t *testing.T, blocks []types.BlockDescription) (*types.TxResponse, error) {
	t.Helper()

	res, err := th.fp.finalitySubmitter.SubmitBatchFinalitySignatures(t.Context(), blocks)
	if err != nil {
		return nil, fmt.Errorf("failed to submit batch finality signatures: %w", err)
	}

	return res, nil
}

func (th *FinalityProviderTestHelper) MustUpdateStateAfterFinalitySigSubmission(t *testing.T, height uint64) {
	t.Helper()
	if err := th.fp.fpState.SetLastVotedHeight(height); err != nil {
		t.Fatalf("failed to update state after finality sig submission: %s", err.Error())
	}
}

// SubmitBatchFinalitySignaturesAndExtractPrivKey is the batch version of SubmitFinalitySignatureAndExtractPrivKey
// This allows testing batch submission with a mix of duplicate and new votes
// NOTE: the input blocks should be in the ascending order of height
func (th *FinalityProviderTestHelper) SubmitBatchFinalitySignaturesAndExtractPrivKey(
	ctx context.Context,
	blocks []types.BlockDescription,
	useSafeEOTSFunc bool,
) (*types.TxResponse, *btcec.PrivateKey, error) {
	if len(blocks) == 0 {
		return nil, nil, fmt.Errorf("should not submit batch finality signature with zero blocks")
	}

	if len(blocks) > int(^uint32(0)) {
		return nil, nil, fmt.Errorf("should not submit batch finality signature with too many blocks")
	}

	// get public randomness list
	numPubRand := len(blocks)
	// #nosec G115 -- performed the conversion check above
	prList, err := th.fp.GetPubRandList(blocks[0].GetHeight(), uint32(numPubRand))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get public randomness list: %w", err)
	}

	// get proof list
	proofBytesList := make([][]byte, 0, numPubRand)
	for _, b := range blocks {
		proofBytes, err := th.fp.pubRandState.getPubRandProof(th.fp.btcPk.MustMarshal(), th.fp.GetChainID(), b.GetHeight())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get public randomness inclusion proof: %w", err)
		}
		proofBytesList = append(proofBytesList, proofBytes)
	}

	// Determine which signer function to use
	eotsSignerFunc := func(_ context.Context, b types.BlockDescription) (*bbntypes.SchnorrEOTSSig, error) {
		sig, err := th.fp.em.UnsafeSignEOTS(th.fp.btcPk.MustMarshal(), th.fp.GetChainID(), b.MsgToSign(""), b.GetHeight())
		if err != nil {
			return nil, fmt.Errorf("failed to sign EOTS: %w", err)
		}

		return bbntypes.NewSchnorrEOTSSigFromModNScalar(sig), nil
	}

	if useSafeEOTSFunc {
		eotsSignerFunc = th.fp.SignFinalitySig
	}

	// sign all blocks
	sigList := make([]*btcec.ModNScalar, 0, len(blocks))
	for _, b := range blocks {
		eotsSig, err := eotsSignerFunc(ctx, b)
		if err != nil {
			return nil, nil, err
		}
		sigList = append(sigList, eotsSig.ToModNScalar())
	}

	// send batch finality signatures to the consumer chain
	res, err := th.fp.consumerCon.SubmitBatchFinalitySigs(ctx, &ccapi.SubmitBatchFinalitySigsRequest{
		FpPk:        th.fp.GetBtcPk(),
		Blocks:      blocks,
		PubRandList: prList,
		ProofList:   proofBytesList,
		Sigs:        sigList,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send batch finality signatures to the consumer chain: %w", err)
	}

	if res.TxHash == "" {
		return res, nil, nil
	}

	// try to extract the private key if slashing occurred
	var privKey *btcec.PrivateKey
	for _, ev := range res.Events {
		if strings.Contains(ev.EventType, "EventSlashedFinalityProvider") {
			evidenceStr := ev.Attributes["evidence"]
			th.fp.logger.Debug("found slashing evidence")
			var evidence ftypes.Evidence
			if err := jsonpb.UnmarshalString(evidenceStr, &evidence); err != nil {
				return nil, nil, fmt.Errorf("failed to decode evidence bytes to evidence: %s", err.Error())
			}
			privKey, err = evidence.ExtractBTCSK()
			if err != nil {
				return nil, nil, fmt.Errorf("failed to extract private key: %s", err.Error())
			}

			break
		}
	}

	return res, privKey, nil
}

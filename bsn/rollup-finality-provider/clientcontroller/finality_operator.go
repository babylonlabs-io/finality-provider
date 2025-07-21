package clientcontroller

import (
	"context"
	"encoding/json"
	"fmt"

	sdkErr "cosmossdk.io/errors"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	ckpttypes "github.com/babylonlabs-io/babylon/v3/x/checkpointing/types"
	finalitytypes "github.com/babylonlabs-io/babylon/v3/x/finality/types"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/finality-provider/signingcontext"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"go.uber.org/zap"
)

func (cc *RollupBSNController) GetFpFinVoteContext() string {
	return signingcontext.FpFinVoteContextV0(cc.bbnClient.GetConfig().ChainID, cc.Cfg.FinalityContractAddress)
}

func convertProof(cmtProof cmtcrypto.Proof) Proof {
	return Proof{
		Total:    uint64(cmtProof.Total), // #nosec G115
		Index:    uint64(cmtProof.Index), // #nosec G115
		LeafHash: cmtProof.LeafHash,
		Aunts:    cmtProof.Aunts,
	}
}

// SubmitBatchFinalitySigs submits a batch of finality signatures
func (cc *RollupBSNController) SubmitBatchFinalitySigs(
	ctx context.Context,
	req *api.SubmitBatchFinalitySigsRequest,
) (*types.TxResponse, error) {
	if len(req.Blocks) != len(req.Sigs) {
		return nil, fmt.Errorf("the number of blocks %v should match the number of finality signatures %v", len(req.Blocks), len(req.Sigs))
	}
	msgs := make([]sdk.Msg, 0, len(req.Blocks))
	fpPkHex := bbntypes.NewBIP340PubKeyFromBTCPK(req.FpPk).MarshalHex()
	for i, block := range req.Blocks {
		cmtProof := cmtcrypto.Proof{}
		if err := cmtProof.Unmarshal(req.ProofList[i]); err != nil {
			return nil, fmt.Errorf("failed to unmarshal proof: %w", err)
		}

		msg := SubmitFinalitySignatureMsg{
			SubmitFinalitySignature: SubmitFinalitySignatureMsgParams{
				FpPubkeyHex: fpPkHex,
				Height:      block.GetHeight(),
				PubRand:     bbntypes.NewSchnorrPubRandFromFieldVal(req.PubRandList[i]).MustMarshal(),
				Proof:       convertProof(cmtProof),
				BlockHash:   block.Hash,
				Signature:   bbntypes.NewSchnorrEOTSSigFromModNScalar(req.Sigs[i]).MustMarshal(),
			},
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal finality signature message: %w", err)
		}
		execMsg := &wasmtypes.MsgExecuteContract{
			Sender:   cc.bbnClient.MustGetAddr(),
			Contract: cc.Cfg.FinalityContractAddress,
			Msg:      payload,
		}
		msgs = append(msgs, execMsg)
	}

	expectedErrs := []*sdkErr.Error{
		finalitytypes.ErrDuplicatedFinalitySig,
		finalitytypes.ErrSigHeightOutdated,
	}

	res, err := cc.reliablySendMsgs(ctx, msgs, expectedErrs, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send finality signature messages: %w", err)
	}
	cc.logger.Debug(
		"Successfully submitted finality signatures in a batch",
		zap.String("fp_pk_hex", fpPkHex),
		zap.Uint64("start_height", req.Blocks[0].GetHeight()),
		zap.Uint64("end_height", req.Blocks[len(req.Blocks)-1].GetHeight()),
	)

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

// QueryFinalityProviderHasPower queries whether the finality provider has voting power at a given height
func (cc *RollupBSNController) QueryFinalityProviderHasPower(
	ctx context.Context,
	req *api.QueryFinalityProviderHasPowerRequest,
) (bool, error) {
	// 1. Check whether there is timestamped pub rand at this height
	hasPubRand, err := cc.hasTimestampedPubRandAtHeight(ctx, req.FpPk, req.BlockHeight)
	if err != nil {
		return false, fmt.Errorf("failed to query timestamped pub rand at height: %w", err)
	}
	if !hasPubRand {
		return false, nil
	}

	// 2. Check whether there is >= 1 active BTC delegation under this FP currently
	hasActiveDelegation, err := cc.hasActiveBTCDelegation(ctx, req.FpPk)
	if err != nil {
		return false, err
	}
	if !hasActiveDelegation {
		return false, nil
	}

	return true, nil
}

// hasPubRandAtHeight checks if the finality provider has timestamped public randomness committed at the given block height.
func (cc *RollupBSNController) hasTimestampedPubRandAtHeight(ctx context.Context, fpPk *btcec.PublicKey, blockHeight uint64) (bool, error) {
	pubRand, err := cc.queryLastPublicRandCommit(ctx, fpPk)
	if err != nil {
		return false, fmt.Errorf("failed to query last public rand commit: %w", err)
	}
	if pubRand == nil {
		return false, nil
	}

	lastCommittedPubRandHeight := pubRand.EndHeight()
	cc.logger.Debug(
		"FP last committed public randomness",
		zap.Uint64("height", lastCommittedPubRandHeight),
	)

	// For now, we only check if the requested height is less than or equal to the last committed height.
	if blockHeight > lastCommittedPubRandHeight {
		cc.logger.Debug(
			"FP has 0 voting power because there is no public randomness at this height",
			zap.Uint64("height", blockHeight),
		)

		return false, nil
	}

	lastFinalizedCkpt, err := cc.bbnClient.LatestEpochFromStatus(ckpttypes.Finalized)
	if err != nil {
		return false, fmt.Errorf("failed to query last finalized checkpoint: %w", err)
	}
	if pubRand.BabylonEpoch > lastFinalizedCkpt.RawCheckpoint.EpochNum {
		cc.logger.Debug(
			"the pub rand's corresponding epoch hasn't been finalized yet, last finalized epoch",
			zap.Uint64("pub_rand_epoch", pubRand.BabylonEpoch),
			zap.Uint64("last_finalized_epoch", lastFinalizedCkpt.RawCheckpoint.EpochNum),
		)

		return false, nil
	}

	return true, nil
}

// hasActiveBTCDelegation checks if there is at least one active BTC delegation for the given FP.
func (cc *RollupBSNController) hasActiveBTCDelegation(_ context.Context, fpPk *btcec.PublicKey) (bool, error) {
	fpBtcPkHex := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex()
	var nextKey []byte

	btcStakingParams, err := cc.bbnClient.QueryClient.BTCStakingParams()
	if err != nil {
		return false, fmt.Errorf("failed to query BTC staking params: %w", err)
	}

	for {
		resp, err := cc.bbnClient.QueryClient.FinalityProviderDelegations(fpBtcPkHex, &sdkquerytypes.PageRequest{Key: nextKey, Limit: 100})
		if err != nil {
			return false, fmt.Errorf("failed to query finality provider delegations: %w", err)
		}

		for _, btcDels := range resp.BtcDelegatorDelegations {
			for _, btcDel := range btcDels.Dels {
				active, err := cc.isDelegationActive(btcStakingParams, btcDel)
				if err != nil {
					continue
				}
				if active {
					return true, nil
				}
			}
		}

		if resp.Pagination == nil || resp.Pagination.NextKey == nil {
			break
		}
		nextKey = resp.Pagination.NextKey
	}

	cc.logger.Debug(
		"FP has 0 voting power because there is no BTC delegation",
		zap.String("fp_btc_pk", fpBtcPkHex),
	)

	return false, nil
}

func (cc *RollupBSNController) QueryFinalityProviderHighestVotedHeight(_ context.Context, _ *btcec.PublicKey) (uint64, error) {
	// TODO: implement highest voted height feature in OP stack L2
	return 0, nil
}

func (cc *RollupBSNController) QueryFinalityProviderStatus(_ context.Context, _ *btcec.PublicKey) (*api.FinalityProviderStatusResponse, error) {
	// TODO: implement slashed or jailed feature in OP stack L2
	return &api.FinalityProviderStatusResponse{
		Slashed: false,
		Jailed:  false,
	}, nil
}

func (cc *RollupBSNController) UnjailFinalityProvider(_ context.Context, _ *btcec.PublicKey) (*types.TxResponse, error) {
	// TODO: implement unjail feature in OP stack L2
	return nil, nil
}

// nolint:unparam
func (cc *RollupBSNController) isDelegationActive(
	btcStakingParams *btcstakingtypes.QueryParamsResponse,
	btcDel *btcstakingtypes.BTCDelegationResponse,
) (bool, error) {
	covQuorum := btcStakingParams.GetParams().CovenantQuorum
	ud := btcDel.UndelegationResponse

	if ud.DelegatorUnbondingInfoResponse != nil {
		return false, nil
	}

	if len(btcDel.CovenantSigs) < int(covQuorum) { // #nosec G115
		return false, nil
	}
	if len(ud.CovenantUnbondingSigList) < int(covQuorum) { // #nosec G115
		return false, nil
	}
	if len(ud.CovenantSlashingSigs) < int(covQuorum) { // #nosec G115
		return false, nil
	}

	return true, nil
}

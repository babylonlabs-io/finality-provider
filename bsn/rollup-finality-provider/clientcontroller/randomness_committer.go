package clientcontroller

import (
	"context"
	"encoding/json"
	"fmt"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/finality-provider/signingcontext"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"go.uber.org/zap"
)

func (cc *RollupBSNController) GetFpRandCommitContext() string {
	return signingcontext.FpRandCommitContextV0(cc.bbnClient.GetConfig().ChainID, cc.Cfg.FinalityContractAddress)
}

// CommitPubRandList commits a list of Schnorr public randomness to Babylon CosmWasm contract
// it returns tx hash and error
func (cc *RollupBSNController) CommitPubRandList(
	ctx context.Context,
	req *api.CommitPubRandListRequest,
) (*types.TxResponse, error) {
	fpPkHex := bbntypes.NewBIP340PubKeyFromBTCPK(req.FpPk).MarshalHex()
	msg := CommitPublicRandomnessMsg{
		CommitPublicRandomness: CommitPublicRandomnessMsgParams{
			FpPubkeyHex: fpPkHex,
			StartHeight: req.StartHeight,
			NumPubRand:  req.NumPubRand,
			Commitment:  req.Commitment,
			Signature:   req.Sig.Serialize(),
		},
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal commit pubrand message: %w", err)
	}
	execMsg := &wasmtypes.MsgExecuteContract{
		Sender:   cc.bbnClient.MustGetAddr(),
		Contract: cc.Cfg.FinalityContractAddress,
		Msg:      payload,
	}

	res, err := cc.ReliablySendMsg(ctx, execMsg, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send commit pubrand message: %w", err)
	}
	cc.logger.Debug("Successfully committed public randomness",
		zap.String("fp_pk_hex", fpPkHex),
		zap.Uint64("start_height", req.StartHeight),
		zap.Uint64("num_pub_rand", req.NumPubRand),
	)

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

// QueryLastPublicRandCommit returns the last public randomness commitments
// It is fetched from the state of a CosmWasm contract OP finality gadget.
func (cc *RollupBSNController) QueryLastPublicRandCommit(ctx context.Context, fpPk *btcec.PublicKey) (*types.PubRandCommit, error) {
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)
	queryMsg := &QueryMsg{
		LastPubRandCommit: &PubRandCommit{
			BtcPkHex: fpPubKey.MarshalHex(),
		},
	}

	jsonData, err := json.Marshal(queryMsg)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling to JSON: %w", err)
	}

	stateResp, err := cc.QuerySmartContractState(ctx, cc.Cfg.FinalityContractAddress, string(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}
	if len(stateResp.Data) == 0 {
		return nil, nil
	}

	// TODO: have its own type with epoch number
	var resp *types.PubRandCommit
	err = json.Unmarshal(stateResp.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if resp == nil {
		return nil, nil
	}
	if err := resp.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate response: %w", err)
	}

	return resp, nil
}

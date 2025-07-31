package babylon

import (
	"context"
	"fmt"
	"strings"

	sdkErr "cosmossdk.io/errors"
	"github.com/babylonlabs-io/babylon/v3/client/babylonclient"
	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	finalitytypes "github.com/babylonlabs-io/babylon/v3/x/finality/types"
	"github.com/btcsuite/btcd/btcec/v2"
	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/signingcontext"
	"github.com/babylonlabs-io/finality-provider/types"
)

var _ api.ConsumerController = &BabylonConsumerController{}

//nolint:revive
type BabylonConsumerController struct {
	bbnClient *bbnclient.Client
	cfg       *fpcfg.BBNConfig
	logger    *zap.Logger
}

func NewBabylonConsumerController(
	cfg *fpcfg.BBNConfig,
	logger *zap.Logger,
) (*BabylonConsumerController, error) {
	bbnConfig := cfg.ToBabylonConfig()
	if err := bbnConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config for Babylon client: %w", err)
	}

	bc, err := bbnclient.New(
		&bbnConfig,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Babylon client: %w", err)
	}

	return &BabylonConsumerController{
		bc,
		cfg,
		logger,
	}, nil
}

func (bc *BabylonConsumerController) MustGetTxSigner() string {
	signer := bc.GetKeyAddress()
	prefix := bc.cfg.AccountPrefix

	return sdk.MustBech32ifyAddressBytes(prefix, signer)
}

func (bc *BabylonConsumerController) GetKeyAddress() sdk.AccAddress {
	// get key address, retrieves address based on key name which is configured in
	// cfg *stakercfg.BBNConfig. If this fails, it means we have misconfiguration problem
	// and we should panic.
	// This is checked at the start of BabylonConsumerController, so if it fails something is really wrong

	keyRec, err := bc.bbnClient.GetKeyring().Key(bc.cfg.Key)

	if err != nil {
		panic(fmt.Sprintf("Failed to get key address: %s", err))
	}

	addr, err := keyRec.GetAddress()

	if err != nil {
		panic(fmt.Sprintf("Failed to get key address: %s", err))
	}

	return addr
}

func (bc *BabylonConsumerController) reliablySendMsg(ctx context.Context, msg sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*babylonclient.RelayerTxResponse, error) {
	return bc.reliablySendMsgs(ctx, []sdk.Msg{msg}, expectedErrs, unrecoverableErrs)
}

func (bc *BabylonConsumerController) reliablySendMsgs(ctx context.Context, msgs []sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*babylonclient.RelayerTxResponse, error) {
	resp, err := bc.bbnClient.ReliablySendMsgs(
		ctx,
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to reliably send messages: %w", err)
	}

	return resp, nil
}

func (bc *BabylonConsumerController) GetFpRandCommitContext() string {
	return signingcontext.FpRandCommitContextV0(bc.cfg.ChainID, signingcontext.AccFinality.String())
}

func (bc *BabylonConsumerController) GetFpFinVoteContext() string {
	return signingcontext.FpFinVoteContextV0(bc.cfg.ChainID, signingcontext.AccFinality.String())
}

// CommitPubRandList commits a list of Schnorr public randomness via a MsgCommitPubRand to Babylon
// it returns tx hash and error
func (bc *BabylonConsumerController) CommitPubRandList(
	ctx context.Context,
	req *api.CommitPubRandListRequest,
) (*types.TxResponse, error) {
	msg := &finalitytypes.MsgCommitPubRandList{
		Signer:      bc.MustGetTxSigner(),
		FpBtcPk:     bbntypes.NewBIP340PubKeyFromBTCPK(req.FpPk),
		StartHeight: req.StartHeight,
		NumPubRand:  req.NumPubRand,
		Commitment:  req.Commitment,
		Sig:         bbntypes.NewBIP340SignatureFromBTCSig(req.Sig),
	}

	unrecoverableErrs := []*sdkErr.Error{
		finalitytypes.ErrInvalidPubRand,
		finalitytypes.ErrTooFewPubRand,
		finalitytypes.ErrNoPubRandYet,
		btcstakingtypes.ErrFpNotFound,
	}

	res, err := bc.reliablySendMsg(ctx, msg, emptyErrs, unrecoverableErrs)
	if err != nil {
		return nil, err
	}

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

// SubmitBatchFinalitySigs submits a batch of finality signatures to Babylon
func (bc *BabylonConsumerController) SubmitBatchFinalitySigs(
	ctx context.Context,
	req *api.SubmitBatchFinalitySigsRequest,
) (*types.TxResponse, error) {
	if len(req.Blocks) != len(req.Sigs) {
		return nil, fmt.Errorf("the number of blocks %v should match the number of finality signatures %v", len(req.Blocks), len(req.Sigs))
	}

	msgs := make([]sdk.Msg, 0, len(req.Blocks))
	for i, b := range req.Blocks {
		cmtProof := cmtcrypto.Proof{}
		if err := cmtProof.Unmarshal(req.ProofList[i]); err != nil {
			return nil, fmt.Errorf("failed to unmarshal proof: %w", err)
		}

		msg := &finalitytypes.MsgAddFinalitySig{
			Signer:       bc.MustGetTxSigner(),
			FpBtcPk:      bbntypes.NewBIP340PubKeyFromBTCPK(req.FpPk),
			BlockHeight:  b.GetHeight(),
			PubRand:      bbntypes.NewSchnorrPubRandFromFieldVal(req.PubRandList[i]),
			Proof:        &cmtProof,
			BlockAppHash: b.GetHash(),
			FinalitySig:  bbntypes.NewSchnorrEOTSSigFromModNScalar(req.Sigs[i]),
		}
		msgs = append(msgs, msg)
	}

	unrecoverableErrs := []*sdkErr.Error{
		finalitytypes.ErrInvalidFinalitySig,
		finalitytypes.ErrPubRandNotFound,
		btcstakingtypes.ErrFpAlreadySlashed,
	}

	expectedErrs := []*sdkErr.Error{
		finalitytypes.ErrDuplicatedFinalitySig,
		finalitytypes.ErrSigHeightOutdated,
	}

	res, err := bc.reliablySendMsgs(ctx, msgs, expectedErrs, unrecoverableErrs)
	if err != nil {
		return nil, err
	}

	if res == nil {
		return &types.TxResponse{}, nil
	}

	return &types.TxResponse{TxHash: res.TxHash, Events: res.Events}, nil
}

// QueryFinalityProviderHasPower queries whether the finality provider has voting power at a given height
func (bc *BabylonConsumerController) QueryFinalityProviderHasPower(
	_ context.Context,
	req *api.QueryFinalityProviderHasPowerRequest,
) (bool, error) {
	res, err := bc.bbnClient.QueryClient.FinalityProviderPowerAtHeight(
		bbntypes.NewBIP340PubKeyFromBTCPK(req.FpPk).MarshalHex(),
		req.BlockHeight,
	)
	if err != nil {
		// voting power table not updated indicates that no fp has voting power
		// therefore, it should be treated as the fp having 0 voting power
		if strings.Contains(err.Error(), finalitytypes.ErrVotingPowerTableNotUpdated.Error()) {
			bc.logger.Info("the voting power table not updated yet")

			return false, nil
		}

		return false, fmt.Errorf("failed to query the finality provider's voting power at height %d: %w", req.BlockHeight, err)
	}

	return res.VotingPower > 0, nil
}

func (bc *BabylonConsumerController) QueryLatestFinalizedBlock(_ context.Context) (types.BlockDescription, error) {
	blocks, err := bc.queryLatestBlocks(nil, 1, finalitytypes.QueriedBlockStatus_FINALIZED, true)
	if blocks == nil {
		return nil, err
	}

	return blocks[0], nil
}

func (bc *BabylonConsumerController) QueryBlocks(_ context.Context, req *api.QueryBlocksRequest) ([]types.BlockDescription, error) {
	if req.EndHeight < req.StartHeight {
		return nil, fmt.Errorf("the startHeight %v should not be higher than the endHeight %v", req.StartHeight, req.EndHeight)
	}
	count := req.EndHeight - req.StartHeight + 1
	if count > uint64(req.Limit) {
		count = uint64(req.Limit)
	}

	return bc.queryLatestBlocks(sdk.Uint64ToBigEndian(req.StartHeight), count, finalitytypes.QueriedBlockStatus_ANY, false)
}

func (bc *BabylonConsumerController) queryLatestBlocks(startKey []byte, count uint64, status finalitytypes.QueriedBlockStatus, reverse bool) ([]types.BlockDescription, error) {
	var blocks []types.BlockDescription
	pagination := &sdkquery.PageRequest{
		Limit:   count,
		Reverse: reverse,
		Key:     startKey,
	}

	res, err := bc.bbnClient.QueryClient.ListBlocks(status, pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to query finalized blocks: %w", err)
	}

	for _, b := range res.Blocks {
		blocks = append(blocks, types.NewBlockInfo(b.Height, b.AppHash, b.Finalized))
	}

	return blocks, nil
}

func (bc *BabylonConsumerController) QueryBlock(_ context.Context, height uint64) (types.BlockDescription, error) {
	res, err := bc.bbnClient.QueryClient.Block(height)
	if err != nil {
		return nil, fmt.Errorf("failed to query indexed block at height %v: %w", height, err)
	}

	return types.NewBlockInfo(height, res.Block.AppHash, res.Block.Finalized), nil
}

// QueryLastPublicRandCommit returns the last public randomness commitments
func (bc *BabylonConsumerController) QueryLastPublicRandCommit(_ context.Context, fpPk *btcec.PublicKey) (*types.PubRandCommit, error) {
	fpBtcPk := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)

	pagination := &sdkquery.PageRequest{
		Limit:   1,
		Reverse: true,
	}

	res, err := bc.bbnClient.QueryClient.ListPubRandCommit(fpBtcPk.MarshalHex(), pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to query committed public randomness: %w", err)
	}

	if len(res.PubRandCommitMap) == 0 {
		// expected when there is no PR commit at all
		return nil, nil
	}

	if len(res.PubRandCommitMap) > 1 {
		return nil, fmt.Errorf("expected length to be 1, but get :%d", len(res.PubRandCommitMap))
	}

	var commit *types.PubRandCommit
	for height, commitRes := range res.PubRandCommitMap {
		commit = &types.PubRandCommit{
			StartHeight: height,
			NumPubRand:  commitRes.NumPubRand,
			Commitment:  commitRes.Commitment,
		}
	}

	if commit == nil {
		return nil, fmt.Errorf("no public randomness commitment found for finality provider %s", fpBtcPk.MarshalHex())
	}

	if err := commit.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate public randomness commitment: %w", err)
	}

	return commit, nil
}

func (bc *BabylonConsumerController) QueryIsBlockFinalized(_ context.Context, height uint64) (bool, error) {
	res, err := bc.bbnClient.QueryClient.Block(height)
	if err != nil {
		return false, fmt.Errorf("failed to query indexed block at height %v: %w", height, err)
	}

	return res.Block.Finalized, nil
}

func (bc *BabylonConsumerController) QueryFinalityActivationBlockHeight(_ context.Context) (uint64, error) {
	res, err := bc.bbnClient.QueryClient.FinalityParams()
	if err != nil {
		return 0, fmt.Errorf("failed to query finality params to get finality activation block height: %w", err)
	}

	return res.Params.FinalityActivationHeight, nil
}

// QueryFinalityProviderHighestVotedHeight queries the highest voted height of the given finality provider
func (bc *BabylonConsumerController) QueryFinalityProviderHighestVotedHeight(_ context.Context, fpPk *btcec.PublicKey) (uint64, error) {
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)
	res, err := bc.bbnClient.QueryClient.FinalityProvider(fpPubKey.MarshalHex())
	if err != nil {
		return 0, fmt.Errorf("failed to query highest voted height for finality provider %s: %w", fpPubKey.MarshalHex(), err)
	}

	return uint64(res.FinalityProvider.HighestVotedHeight), nil
}

func (bc *BabylonConsumerController) QueryLatestBlock(ctx context.Context) (types.BlockDescription, error) {
	blocks, err := bc.queryLatestBlocks(nil, 1, finalitytypes.QueriedBlockStatus_ANY, true)
	if err != nil || len(blocks) != 1 {
		// try query comet block if the index block query is not available
		block, err := bc.queryCometBestBlock(ctx)
		if err != nil {
			return nil, err
		}

		return block, nil
	}

	return blocks[0], nil
}

func (bc *BabylonConsumerController) queryCometBestBlock(ctx context.Context) (*types.BlockInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, bc.cfg.Timeout)
	// this will return 20 items at max in the descending order (highest first)
	chainInfo, err := bc.bbnClient.RPCClient.BlockchainInfo(ctx, 0, 0)
	defer cancel()

	if err != nil {
		return nil, fmt.Errorf("failed to query comet best block: %w", err)
	}

	// Returning response directly, if header with specified number did not exist
	// at request will contain nil header
	// #nosec G115
	return types.NewBlockInfo(
		uint64(chainInfo.BlockMetas[0].Header.Height),
		chainInfo.BlockMetas[0].Header.AppHash,
		false,
	), nil
}

// QueryFinalityProviderStatus - returns if the fp has been slashed, jailed, err
func (bc *BabylonConsumerController) QueryFinalityProviderStatus(_ context.Context, fpPk *btcec.PublicKey) (*api.FinalityProviderStatusResponse, error) {
	fpPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)
	res, err := bc.bbnClient.QueryClient.FinalityProvider(fpPubKey.MarshalHex())
	if err != nil {
		return nil, fmt.Errorf("failed to query the finality provider %s: %w", fpPubKey.MarshalHex(), err)
	}

	return api.NewFinalityProviderStatusResponse(
		res.FinalityProvider.SlashedBtcHeight > 0,
		res.FinalityProvider.Jailed,
	), nil
}

func (bc *BabylonConsumerController) Close() error {
	if !bc.bbnClient.IsRunning() {
		return nil
	}

	if err := bc.bbnClient.Stop(); err != nil {
		return fmt.Errorf("failed to stop babylon client: %w", err)
	}

	return nil
}

// QueryFinalityProviderInAllowlist queries whether the finality provider is in the allowlist
// For Babylon, there is no FP allowlist feature - all FPs are allowed
func (bc *BabylonConsumerController) QueryFinalityProviderInAllowlist(_ context.Context, _ *btcec.PublicKey) (bool, error) {
	// No FP allowlist feature in Babylon - all FPs are allowed
	return true, nil
}

// UnjailFinalityProvider sends an unjail transaction to the consumer chain
func (bc *BabylonConsumerController) UnjailFinalityProvider(ctx context.Context, fpPk *btcec.PublicKey) (*types.TxResponse, error) {
	msg := &finalitytypes.MsgUnjailFinalityProvider{
		Signer:  bc.MustGetTxSigner(),
		FpBtcPk: bbntypes.NewBIP340PubKeyFromBTCPK(fpPk),
	}

	unrecoverableErrs := []*sdkErr.Error{
		btcstakingtypes.ErrFpNotFound,
		btcstakingtypes.ErrFpNotJailed,
		btcstakingtypes.ErrFpAlreadySlashed,
	}

	res, err := bc.reliablySendMsg(ctx, msg, emptyErrs, unrecoverableErrs)
	if err != nil {
		return nil, err
	}

	return &types.TxResponse{TxHash: res.TxHash, Events: res.Events}, nil
}

// QueryPublicRandCommitList returns the public randomness commitments list from the startHeight to the last commit
// the returned commits are ordered in the accenting order of the start height
func (bc *BabylonConsumerController) QueryPublicRandCommitList(fpPk *btcec.PublicKey, startHeight uint64) ([]*types.PubRandCommit, error) {
	fpBtcPk := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)

	pagination := &sdkquery.PageRequest{
		Limit:   1,
		Reverse: false,
	}

	commitList := make([]*types.PubRandCommit, 0)

	for {
		res, err := bc.bbnClient.QueryClient.ListPubRandCommit(fpBtcPk.MarshalHex(), pagination)
		if err != nil {
			return nil, fmt.Errorf("failed to query committed public randomness: %w", err)
		}

		if len(res.PubRandCommitMap) == 0 {
			// expected when there is no PR commit at all
			return nil, nil
		}
		if len(res.PubRandCommitMap) > 1 {
			return nil, fmt.Errorf("expected length to be 1, but get :%d", len(res.PubRandCommitMap))
		}
		var commit *types.PubRandCommit
		for height, commitRes := range res.PubRandCommitMap {
			commit = &types.PubRandCommit{
				StartHeight: height,
				NumPubRand:  commitRes.NumPubRand,
				Commitment:  commitRes.Commitment,
			}
		}
		if err := commit.Validate(); err != nil {
			return nil, fmt.Errorf("failed to validate public randomness commitment: %w", err)
		}

		if startHeight <= commit.EndHeight() {
			commitList = append(commitList, commit)
		}

		if res.Pagination == nil || res.Pagination.NextKey == nil {
			break
		}

		pagination.Key = res.Pagination.NextKey
	}

	return commitList, nil
}

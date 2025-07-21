//go:build e2e_rollup
// +build e2e_rollup

package e2e

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"

	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/stretchr/testify/require"
)

func TestPubRandCommitment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctm := StartRollupTestManager(t, ctx)
	defer ctm.Stop(t)

	// create and register Babylon FP and rollup BSN FP
	babylonFpPk, bsnFpPk := ctm.setupBabylonAndConsumerFp(t)

	// add the consumer FP to the allow list
	ctm.AddFPToAllowList(t, ctx, bsnFpPk)

	// send a BTC delegation and wait for activation
	ctm.delegateBTCAndWaitForActivation(t, babylonFpPk, bsnFpPk)

	// get the consumer FP instance
	consumerFpInstance := ctm.getConsumerFpInstance(t, bsnFpPk)

	// commit pub rand with start height 1
	// this will call consumer controller's CommitPubRandList function
	_, err := consumerFpInstance.CommitPubRand(ctx, 1)
	require.NoError(t, err)

	// query the last pub rand
	pubRand, err := ctm.RollupBSNController.QueryLastPublicRandCommit(ctx, bsnFpPk.MustToBTCPK())
	require.NoError(t, err)
	require.NotNil(t, pubRand)

	// check the end height of the pub rand
	// endHeight = startHeight + numberPubRand - 1
	// startHeight is 1 in this case, so EndHeight should equal NumPubRand
	consumerCfg := ctm.ConsumerFpApp.GetConfig()
	require.Equal(t, uint64(consumerCfg.NumPubRand), pubRand.EndHeight())
}

// TestFinalitySigSubmission tests the consumer controller's function:
// - SubmitBatchFinalitySigs
func TestFinalitySigSubmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctm := StartRollupTestManager(t, ctx)
	defer ctm.Stop(t)

	// create and register Babylon FP and rollup BSN FP
	babylonFpPk, bsnFpPk := ctm.setupBabylonAndConsumerFp(t)

	// send a BTC delegation and wait for activation
	ctm.delegateBTCAndWaitForActivation(t, babylonFpPk, bsnFpPk)

	// add the consumer FP to the allow list
	ctm.AddFPToAllowList(t, ctx, bsnFpPk)

	// get the consumer FP instance
	consumerFpInstance := ctm.getConsumerFpInstance(t, bsnFpPk)

	// commit pub rand with start height 1
	// this will call consumer controller's CommitPubRandList function
	_, err := consumerFpInstance.CommitPubRand(ctx, 1)
	require.NoError(t, err)

	// finalise this pub rand commit
	ctm.FinalizeUntilEpoch(t, ctm.GetCurrentEpoch(t))

	// mock batch of blocks with start height 1 and end height 3
	blocks := testutil.GenBlocksDesc(
		rand.New(rand.NewSource(time.Now().UnixNano())),
		1,
		3,
	)

	// submit finality signature
	// this will call consumer controller's SubmitBatchFinalitySignatures function
	_, err = consumerFpInstance.SubmitBatchFinalitySignatures(blocks)
	require.NoError(t, err)

	// fill the query message with the block height and hash
	queryMsg := map[string]interface{}{
		"block_voters": map[string]interface{}{
			"height": blocks[2].GetHeight(),
			// it requires the block hash without the 0x prefix
			"hash_hex": strings.TrimPrefix(hex.EncodeToString(blocks[2].GetHash()), "0x"),
		},
	}

	// query block_voters from finality gadget CW contract
	queryResponse := ctm.queryFinalityContract(t, ctx, queryMsg)
	// Define a struct matching the returned BlockVoterInfo
	type BlockVoterInfo struct {
		FpBtcPkHex        string          `json:"fp_btc_pk_hex"`
		PubRand           []byte          `json:"pub_rand"`
		FinalitySignature json.RawMessage `json:"finality_signature"`
	}

	var voters []BlockVoterInfo
	err = json.Unmarshal(queryResponse.Data, &voters)
	require.NoError(t, err)

	// check the voter, it should be the consumer FP public key
	require.Equal(t, 1, len(voters))
	require.Equal(t, bsnFpPk.MarshalHex(), voters[0].FpBtcPkHex)
}

// TestFinalityProviderHasPower tests the consumer controller's function:
// - QueryFinalityProviderHasPower
func TestFinalityProviderHasPower(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctm := StartRollupTestManager(t, ctx)
	defer ctm.Stop(t)

	// create and register Babylon FP and rollup BSN FP
	babylonFpPk, bsnFpPk := ctm.setupBabylonAndConsumerFp(t)

	// add the consumer FP to the allow list
	ctm.AddFPToAllowList(t, ctx, bsnFpPk)

	// query the finality provider has power
	hasPower, err := ctm.RollupBSNController.QueryFinalityProviderHasPower(ctx, api.NewQueryFinalityProviderHasPowerRequest(
		bsnFpPk.MustToBTCPK(),
		1,
	))
	require.NoError(t, err)
	require.False(t, hasPower)

	// send a BTC delegation and wait for activation
	ctm.delegateBTCAndWaitForActivation(t, babylonFpPk, bsnFpPk)

	// query the finality provider has power again
	// fp has 0 voting power b/c there is no public randomness at this height
	hasPower, err = ctm.RollupBSNController.QueryFinalityProviderHasPower(ctx, api.NewQueryFinalityProviderHasPowerRequest(
		bsnFpPk.MustToBTCPK(),
		1,
	))
	require.NoError(t, err)
	require.False(t, hasPower)

	// commit pub rand with start height 1
	consumerFpInstance := ctm.getConsumerFpInstance(t, bsnFpPk)
	_, err = consumerFpInstance.CommitPubRand(ctx, 1)
	require.NoError(t, err)

	// query the finality provider has power again
	// fp has voting power now
	hasPower, err = ctm.RollupBSNController.QueryFinalityProviderHasPower(ctx, api.NewQueryFinalityProviderHasPowerRequest(
		bsnFpPk.MustToBTCPK(),
		1,
	))
	require.NoError(t, err)
	require.True(t, hasPower)
}

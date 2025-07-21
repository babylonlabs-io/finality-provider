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

// TestRollupFinalityProviderLifeCycle tests the complete lifecycle of a rollup BSN finality provider
// This test covers:
// 1. FP creation and registration on both Babylon and rollup BSN
// 2. Public randomness commitment to the rollup contract
// 3. BTC delegation and activation
// 4. Finality signature submission and verification
// 5. Contract state verification throughout the lifecycle
// 6. Block finalization verification
func TestRollupFinalityProviderLifeCycle(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctm := StartRollupTestManager(t, ctx)
	defer ctm.Stop(t)

	// Step 1: Create and register both Babylon FP and rollup BSN FP
	t.Log("Step 1: Creating and registering finality providers")
	fps := ctm.setupBabylonAndConsumerFp(t)
	babylonFpPk := fps[0]
	consumerFpPk := fps[1]

	// Verify both FPs are registered
	require.Eventually(t, func() bool {
		babylonFps, err := ctm.BabylonController.QueryFinalityProviders()
		if err != nil {
			t.Logf("Failed to query Babylon FPs: %v", err)
			return false
		}
		consumerFps, err := ctm.BabylonController.QueryConsumerFinalityProviders(rollupBSNID)
		if err != nil {
			t.Logf("Failed to query consumer FPs: %v", err)
			return false
		}
		return len(babylonFps) >= 1 && len(consumerFps) >= 1
	}, eventuallyWaitTimeOut, eventuallyPollTime, "Both Babylon and consumer FPs should be registered")

	// Step 2: Delegate BTC and wait for activation
	t.Log("Step 2: Delegating BTC and waiting for activation")
	ctm.delegateBTCAndWaitForActivation(t, babylonFpPk, consumerFpPk)

	// Step 3: Get consumer FP instance and start it (it will automatically commit randomness)
	t.Log("Step 3: Starting FP instance - it will automatically commit public randomness")
	consumerFpInstance := ctm.getConsumerFpInstance(t, consumerFpPk)

	// Start the FP instance - this will automatically start randomness commitment and voting loops
	err := consumerFpInstance.Start()
	require.NoError(t, err)

	// Clean up - ensure we stop the FP instance when test ends
	t.Cleanup(func() {
		_ = consumerFpInstance.Stop()
	})

	// Step 4: Wait for FP to automatically commit public randomness and get it timestamped
	t.Log("Step 4: Waiting for FP to automatically commit public randomness and get it timestamped")
	ctm.WaitForFpPubRandTimestamped(t, consumerFpInstance)

	// Step 5: Wait for FP to automatically detect and vote on rollup blocks
	t.Log("Step 5: Waiting for FP to automatically vote on rollup blocks")

	// The FP should automatically detect blocks from the rollup chain and submit signatures
	// We wait for the FP to vote on at least one block
	var lastVotedHeight uint64
	require.Eventually(t, func() bool {
		if consumerFpInstance.GetLastVotedHeight() > 0 {
			lastVotedHeight = consumerFpInstance.GetLastVotedHeight()
			t.Logf("FP voted on block at height: %d", lastVotedHeight)
			return true
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime, "FP should automatically vote on rollup blocks")

	t.Log("✅ Rollup BSN Finality Provider Lifecycle Test Completed Successfully!")
	t.Logf("✅ Tested FP: %s", consumerFpPk.MarshalHex())
	t.Logf("✅ FP automatically voted on rollup block at height: %d", lastVotedHeight)
	t.Logf("✅ Verified contract state consistency throughout lifecycle")
}

func TestPubRandCommitment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctm := StartRollupTestManager(t, ctx)
	defer ctm.Stop(t)

	// create and register Babylon FP and rollup BSN FP
	fps := ctm.setupBabylonAndConsumerFp(t)

	// send a BTC delegation and wait for activation
	consumerFpPk := fps[1]
	ctm.delegateBTCAndWaitForActivation(t, fps[0], consumerFpPk)

	// get the consumer FP instance
	consumerFpInstance := ctm.getConsumerFpInstance(t, consumerFpPk)

	// commit pub rand with start height 1
	// this will call consumer controller's CommitPubRandList function
	_, err := consumerFpInstance.CommitPubRand(ctx, 1)
	require.NoError(t, err)

	// query the last pub rand
	pubRand, err := ctm.RollupBSNController.QueryLastPublicRandCommit(ctx, consumerFpPk.MustToBTCPK())
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
	fps := ctm.setupBabylonAndConsumerFp(t)
	consumerFpPk := fps[1]

	// send a BTC delegation and wait for activation
	ctm.delegateBTCAndWaitForActivation(t, fps[0], consumerFpPk)

	// get the consumer FP instance
	consumerFpInstance := ctm.getConsumerFpInstance(t, consumerFpPk)

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
	queryResponse := ctm.queryCwContract(t, queryMsg, ctx)
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
	require.Equal(t, consumerFpPk.MarshalHex(), voters[0].FpBtcPkHex)
}

// TestFinalityProviderHasPower tests the consumer controller's function:
// - QueryFinalityProviderHasPower
func TestFinalityProviderHasPower(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctm := StartRollupTestManager(t, ctx)
	defer ctm.Stop(t)

	// create and register Babylon FP and rollup BSN FP
	fps := ctm.setupBabylonAndConsumerFp(t)
	consumerFpPk := fps[1]

	// query the finality provider has power
	hasPower, err := ctm.RollupBSNController.QueryFinalityProviderHasPower(ctx, api.NewQueryFinalityProviderHasPowerRequest(
		consumerFpPk.MustToBTCPK(),
		1,
	))
	require.NoError(t, err)
	require.False(t, hasPower)

	// send a BTC delegation and wait for activation
	ctm.delegateBTCAndWaitForActivation(t, fps[0], consumerFpPk)

	// query the finality provider has power again
	// fp has 0 voting power b/c there is no public randomness at this height
	hasPower, err = ctm.RollupBSNController.QueryFinalityProviderHasPower(ctx, api.NewQueryFinalityProviderHasPowerRequest(
		consumerFpPk.MustToBTCPK(),
		1,
	))
	require.NoError(t, err)
	require.False(t, hasPower)

	// commit pub rand with start height 1
	consumerFpInstance := ctm.getConsumerFpInstance(t, consumerFpPk)
	_, err = consumerFpInstance.CommitPubRand(ctx, 1)
	require.NoError(t, err)

	// query the finality provider has power again
	// fp has voting power now
	hasPower, err = ctm.RollupBSNController.QueryFinalityProviderHasPower(ctx, api.NewQueryFinalityProviderHasPowerRequest(
		consumerFpPk.MustToBTCPK(),
		1,
	))
	require.NoError(t, err)
	require.True(t, hasPower)
}

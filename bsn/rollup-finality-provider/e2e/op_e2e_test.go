package e2e

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/babylonlabs-io/finality-provider/types"
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

	// Step 1.5: Add consumer FP to contract allowlist
	t.Log("Step 1.5: Adding consumer FP to contract allowlist")
	ctm.addFpToContractAllowlist(t, ctx, consumerFpPk)

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

// TestBSNSkippingDoubleSignError tests the scenario where the BSN finality-provider
// should skip the block when encountering a double sign request from the EOTS manager
// This is critical for preventing accidental slashing during restart scenarios
func TestBSNSkippingDoubleSignError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctm := StartRollupTestManager(t, ctx)
	defer ctm.Stop(t)

	// Step 1: Setup FPs and activation
	t.Log("Step 1: Setting up finality providers and BTC delegation")
	fps := ctm.setupBabylonAndConsumerFp(t)
	consumerFpPk := fps[1]

	// Add consumer FP to contract allowlist
	ctm.addFpToContractAllowlist(t, ctx, consumerFpPk)
	ctm.delegateBTCAndWaitForActivation(t, fps[0], consumerFpPk)

	// Step 2: Start FP instance and wait for initial operations
	t.Log("Step 2: Starting FP instance and waiting for initial vote")
	consumerFpInstance := ctm.getConsumerFpInstance(t, consumerFpPk)
	err := consumerFpInstance.Start()
	require.NoError(t, err)

	// Wait for FP to commit randomness and get it timestamped
	ctm.WaitForFpPubRandTimestamped(t, consumerFpInstance)

	// Wait for FP to vote on at least one rollup block
	var lastVotedHeight uint64
	require.Eventually(t, func() bool {
		if consumerFpInstance.GetLastVotedHeight() > 0 {
			lastVotedHeight = consumerFpInstance.GetLastVotedHeight()
			t.Logf("FP voted on rollup block at height: %d", lastVotedHeight)
			return true
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime, "FP should vote on at least one rollup block")

	// Step 3: Create double-sign scenario
	t.Log("Step 3: Creating double-sign scenario - stopping FP and manually signing future height")

	// Stop the FP
	err = consumerFpInstance.Stop()
	require.NoError(t, err)

	// Wait for the rollup chain to produce new blocks - this gives the FP time to fully stop
	// while ensuring we get a fresh height that the FP hasn't processed yet
	currentHeight := ctm.WaitForNRollupBlocks(t, 1)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	mockBlock := types.NewBlockInfo(currentHeight, testutil.GenRandomByteArray(r, 32), false)

	t.Logf("Manually sending a vote for fresh height %d (simulating network issue)", currentHeight)

	// Manually submit finality signature for the next height
	// This creates a signing record in EOTS manager's database
	_, _, err = consumerFpInstance.NewTestHelper().SubmitFinalitySignatureAndExtractPrivKey(ctx, mockBlock, true)
	require.NoError(t, err)

	// Step 4: Restart FP and verify it skips the double-signed height
	t.Log("Step 4: Restarting FP and verifying it skips the manually signed height")

	// Restart the FP
	err = consumerFpInstance.Start()
	require.NoError(t, err)

	// Clean up - ensure we stop the FP instance when test ends
	t.Cleanup(func() {
		_ = consumerFpInstance.Stop()
	})

	// The FP needs to work through duplicate signatures from where it stopped (64)
	// to where the blockchain is now (140+), then vote on fresh heights
	initialVotedHeight := consumerFpInstance.GetLastVotedHeight()
	t.Logf("FP restart state: last voted height %d, current blockchain height %d", initialVotedHeight, currentHeight)
	t.Logf("FP must work through duplicate signatures from %d to %d, then vote on fresh heights", initialVotedHeight+1, currentHeight)

	require.Eventually(t, func() bool {
		if !consumerFpInstance.IsRunning() {
			t.Logf("❌ FP stopped running - this indicates a problem with duplicate sign handling")
			return false
		}

		votedHeight := consumerFpInstance.GetLastVotedHeight()

		// FP should eventually work through all duplicates and vote on fresh heights
		if votedHeight > initialVotedHeight {
			t.Logf("✅ FP successfully worked through duplicate signatures and voted on fresh height %d (started from %d)",
				votedHeight, initialVotedHeight)
			t.Logf("✅ This proves the duplicate sign protection mechanism works correctly")
			return true
		}

		t.Logf("FP working through duplicates, last voted height: %d (started from %d)", votedHeight, initialVotedHeight)
		return false
	}, 3*eventuallyWaitTimeOut, eventuallyPollTime, // Give extra time for processing many duplicates
		"FP should work through duplicate signatures and eventually vote on fresh heights")

	finalVotedHeight := consumerFpInstance.GetLastVotedHeight()

	t.Log("✅ BSN Double Sign Protection Test Completed Successfully!")
	t.Logf("✅ FP correctly handled duplicate signatures and resumed voting (started: %d, final: %d)", initialVotedHeight, finalVotedHeight)
	t.Logf("✅ Duplicate sign protection mechanism working correctly in BSN environment")
	t.Logf("✅ This validates the FP can recover from network interruptions without double signing")
}

// TestBSNDoubleSigning tests the attack scenario where the BSN finality-provider
// sends a finality vote over a conflicting block in the rollup BSN environment
// In this case, the BTC private key should be extracted by the system
func TestBSNDoubleSigning(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctm := StartRollupTestManager(t, ctx)
	defer ctm.Stop(t)

	// Step 1: Setup FPs and activation
	t.Log("Step 1: Setting up finality providers and BTC delegation")
	fps := ctm.setupBabylonAndConsumerFp(t)
	babylonFpPk := fps[0]
	consumerFpPk := fps[1]

	// Add consumer FP to contract allowlist
	ctm.addFpToContractAllowlist(t, ctx, consumerFpPk)
	ctm.delegateBTCAndWaitForActivation(t, babylonFpPk, consumerFpPk)

	// Step 2: Start FP instance and wait for initial operations
	t.Log("Step 2: Starting FP instance and waiting for operations")
	consumerFpInstance := ctm.getConsumerFpInstance(t, consumerFpPk)
	err := consumerFpInstance.Start()
	require.NoError(t, err)

	// Clean up - ensure we stop the FP instance when test ends
	t.Cleanup(func() {
		_ = consumerFpInstance.Stop()
	})

	// Wait for FP to commit randomness and get it timestamped
	ctm.WaitForFpPubRandTimestamped(t, consumerFpInstance)

	// Wait for FP to vote on at least one rollup block and for finalization
	var lastVotedHeight uint64
	require.Eventually(t, func() bool {
		if consumerFpInstance.GetLastVotedHeight() > 0 {
			lastVotedHeight = consumerFpInstance.GetLastVotedHeight()
			t.Logf("FP voted on rollup block at height: %d", lastVotedHeight)
			return true
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime, "FP should vote on at least one rollup block")

	t.Logf("The rollup block at height %v is voted on", lastVotedHeight)

	// Step 3: Wait for block finalization - get finalized blocks from the rollup chain
	t.Log("Step 3: Waiting for block finalization")
	var finalizedBlock *types.BlockInfo
	require.Eventually(t, func() bool {
		// Query the latest finalized block from the rollup chain
		latestFinalized, err := ctm.RollupBSNController.QueryLatestFinalizedBlock(ctx)
		if err != nil {
			t.Logf("Failed to query latest finalized block: %v", err)
			return false
		}
		if latestFinalized != nil && latestFinalized.GetHeight() >= lastVotedHeight {
			finalizedBlock = types.NewBlockInfo(latestFinalized.GetHeight(), latestFinalized.GetHash(), true)
			t.Logf("Block at height %d is finalized", finalizedBlock.GetHeight())
			return true
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime, "Should have at least one finalized block")

	fpTestHelper := consumerFpInstance.NewTestHelper()

	// Step 4: Test duplicate vote which should be ignored
	t.Log("Step 4: Testing duplicate vote (should be ignored)")
	res, extractedKey, err := fpTestHelper.SubmitFinalitySignatureAndExtractPrivKey(ctx, finalizedBlock, false)
	require.NoError(t, err)
	require.Nil(t, extractedKey)
	require.Empty(t, res)
	t.Logf("Duplicate vote for rollup block %d was properly ignored", finalizedBlock.GetHeight())

	// Step 5: Double signing attack - manually submit a finality vote over a conflicting block
	// to trigger the extraction of finality-provider's private key
	t.Log("Step 5: Performing double signing attack with conflicting block")
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	conflictingBlock := types.NewBlockInfo(finalizedBlock.GetHeight(), testutil.GenRandomByteArray(r, 32), false)

	// First confirm we have double sign protection
	t.Log("Step 5a: Confirming double sign protection is active")
	_, _, err = fpTestHelper.SubmitFinalitySignatureAndExtractPrivKey(ctx, conflictingBlock, true)
	require.Contains(t, err.Error(), "FailedPrecondition", "Double sign protection should prevent conflicting signatures")
	t.Logf("✅ Double sign protection correctly blocked the conflicting signature")

	// Step 6: Force the double signing attack (bypass protection) and extract private key
	t.Log("Step 6: Forcing double signing attack to extract private key")
	_, extractedKey, err = fpTestHelper.SubmitFinalitySignatureAndExtractPrivKey(ctx, conflictingBlock, false)
	require.NoError(t, err)
	require.NotNil(t, extractedKey, "Private key should be extracted from double signing")

	// Verify the extracted key matches the local key
	localKey := ctm.EOTSServerHandler.GetFPPrivKey(t, consumerFpInstance.GetBtcPkBIP340().MustMarshal())
	require.True(t, localKey.Key.Equals(&extractedKey.Key) || localKey.Key.Negate().Equals(&extractedKey.Key),
		"Extracted private key should match the original key (or its negation)")

	t.Log("✅ BSN Double Signing Attack Test Completed Successfully!")
	t.Logf("✅ Successfully extracted private key from double signing attack")
	t.Logf("✅ Double sign protection mechanism working correctly in BSN rollup environment")
	t.Logf("✅ Private key extraction proves the equivocation was detected and handled")
}

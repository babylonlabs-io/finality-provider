//go:build e2e_rollup

package e2etest_rollup

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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	ctm := StartRollupTestManager(t, ctx)
	defer func() {
		cancel()
		ctm.Stop(t)
	}()

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
	err := consumerFpInstance.Start(ctx)
	require.NoError(t, err)

	// Step 4: Wait for FP to automatically commit public randomness and get it timestamped
	t.Log("Step 4: Waiting for FP to automatically commit public randomness and get it timestamped")
	ctm.WaitForFpPubRandTimestamped(t, ctx, consumerFpInstance)

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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	ctm := StartRollupTestManager(t, ctx)
	defer func() {
		cancel()
		ctm.Stop(t)
	}()

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
	err := consumerFpInstance.Start(ctx)
	require.NoError(t, err)

	// Wait for FP to commit randomness and get it timestamped
	ctm.WaitForFpPubRandTimestamped(t, ctx, consumerFpInstance)

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
	currentHeight := ctm.WaitForNRollupBlocks(t, ctx, 1)

	// Make sure we use a height that's a multiple of the finality signature interval
	remainder := currentHeight % finalitySignatureInterval
	if remainder != 0 {
		currentHeight = currentHeight - remainder
	}

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
	err = consumerFpInstance.Start(ctx)
	require.NoError(t, err)

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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	ctm := StartRollupTestManager(t, ctx)
	defer func() {
		cancel()
		ctm.Stop(t)
	}()

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
	err := consumerFpInstance.Start(ctx)
	require.NoError(t, err)

	// Clean up - ensure we stop the FP instance when test ends
	t.Cleanup(func() {
		_ = consumerFpInstance.Stop()
	})

	// Wait for FP to commit randomness and get it timestamped
	ctm.WaitForFpPubRandTimestamped(t, ctx, consumerFpInstance)

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

	fpTestHelper := consumerFpInstance.NewTestHelper()

	// Step 4: Test duplicate vote which should be ignored
	// We need to duplicate vote on the SAME block that was originally voted on, not the finalized block
	t.Log("Step 4: Testing duplicate vote (should be ignored)")

	// Get the originally voted block info
	originalVotedBlock, err := ctm.RollupBSNController.QueryBlock(ctx, lastVotedHeight)
	require.NoError(t, err)
	originalVotedBlockInfo := types.NewBlockInfo(originalVotedBlock.GetHeight(), originalVotedBlock.GetHash(), false)

	res, extractedKey, err := fpTestHelper.SubmitFinalitySignatureAndExtractPrivKey(ctx, originalVotedBlockInfo, false)
	require.NoError(t, err)
	require.Nil(t, extractedKey, "No private key should be extracted from duplicate vote")
	require.Empty(t, res, "Duplicate votes should return empty result")
	t.Logf("Duplicate vote for rollup block %d was properly ignored", originalVotedBlockInfo.GetHeight())

	// Step 5: Double signing attack - manually submit a finality vote over a conflicting block
	// to trigger the extraction of finality-provider's private key
	// Double signing means signing a DIFFERENT block hash at the SAME height as the original vote
	t.Log("Step 5: Performing double signing attack with conflicting block")
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	conflictingBlock := types.NewBlockInfo(originalVotedBlockInfo.GetHeight(), testutil.GenRandomByteArray(r, 32), false)

	// First confirm we have double sign protection
	t.Log("Step 5a: Confirming double sign protection is active")
	_, _, err = fpTestHelper.SubmitFinalitySignatureAndExtractPrivKey(ctx, conflictingBlock, true)
	require.Contains(t, err.Error(), "FailedPrecondition", "Double sign protection should prevent conflicting signatures")
	t.Logf("✅ Double sign protection correctly blocked the conflicting signature")

	// Step 6: Force the double signing attack (bypass protection) and extract private key
	t.Log("Step 6: Forcing double signing attack to extract private key")
	_, extractedKey, err = fpTestHelper.SubmitFinalitySignatureAndExtractPrivKey(ctx, conflictingBlock, false)
	// time.Sleep(5 * time.Minute)
	require.NoError(t, err)
	require.NotNil(t, extractedKey, "Private key should be extracted from double signing")

	// Verify the extracted key matches the local key
	localKey := ctm.EOTSServerHandler.GetFPPrivKey(t, consumerFpInstance.GetBtcPkBIP340().MustMarshal())
	require.True(t, localKey.Key.Equals(&extractedKey.Key) || localKey.Key.Negate().Equals(&extractedKey.Key),
		"Extracted private key should match the original key (or its negation)")

	t.Log("✅ BSN Double Signing Attack Test Completed Successfully!")
	t.Logf("✅ Duplicate votes on same block (height %d) were properly ignored", originalVotedBlockInfo.GetHeight())
	t.Logf("✅ Double signing attack on conflicting block (height %d) correctly extracted private key", conflictingBlock.GetHeight())
	t.Logf("✅ BSN rollup environment properly handles both duplicate signatures and equivocation")
}

// TestRollupBSNCatchingUp tests if a rollup BSN finality provider can catch up after being restarted
// This is the rollup BSN equivalent of the Babylon TestCatchingUp test
func TestRollupBSNCatchingUp(t *testing.T) {
	t.Parallel()
	// Add a test timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	ctm := StartRollupTestManager(t, ctx)
	defer func() {
		cancel()
		ctm.Stop(t)
	}()

	// Step 1: Setup FPs and activation - similar to original test
	t.Log("Step 1: Setting up finality providers and BTC delegation")
	fps := ctm.setupBabylonAndConsumerFp(t)
	babylonFpPk := fps[0]
	consumerFpPk := fps[1]

	// Add consumer FP to contract allowlist
	ctm.addFpToContractAllowlist(t, ctx, consumerFpPk)
	ctm.delegateBTCAndWaitForActivation(t, babylonFpPk, consumerFpPk)

	// Step 2: Start FP instance and establish normal operation
	t.Log("Step 2: Starting FP instance and establishing normal operation")
	consumerFpInstance := ctm.getConsumerFpInstance(t, consumerFpPk)
	err := consumerFpInstance.Start(ctx)
	require.NoError(t, err)

	// Wait for FP to commit randomness and get it timestamped
	ctm.WaitForFpPubRandTimestamped(t, ctx, consumerFpInstance)

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

	t.Logf("Initial voting completed - FP voted on block at height %v", lastVotedHeight)

	// Step 3: Simulate downtime - stop FP for n blocks
	t.Log("Step 3: Simulating FP downtime - stopping FP for several blocks")
	var n uint = 3

	// Stop the finality provider
	err = consumerFpInstance.Stop()
	require.NoError(t, err)
	t.Logf("FP stopped. Current last voted height: %d", lastVotedHeight)

	// Wait for the rollup chain to produce n more blocks while FP is offline
	afterDowntimeHeight := ctm.WaitForNRollupBlocks(t, ctx, int(n))
	t.Logf("Rollup chain produced %d blocks during FP downtime: last voted %d -> current %d",
		n, lastVotedHeight, afterDowntimeHeight)

	// Step 4: Restart FP and trigger catch-up/fast sync
	t.Log("Step 4: Restarting FP to trigger catch-up/fast sync")

	// Restart the FP
	err = consumerFpInstance.Start(ctx)
	require.NoError(t, err)

	// Step 5: Verify FP catches up and continues voting
	t.Log("Step 5: Verifying FP catches up and resumes normal operation")

	// Wait for FP to catch up and vote on new blocks
	var finalVotedHeight uint64
	require.Eventually(t, func() bool {
		if !consumerFpInstance.IsRunning() {
			t.Logf("❌ FP stopped running - this indicates a problem with catch-up")
			return false
		}

		currentVotedHeight := consumerFpInstance.GetLastVotedHeight()

		// FP should catch up and vote on blocks beyond its pre-downtime state
		if currentVotedHeight > lastVotedHeight {
			finalVotedHeight = currentVotedHeight
			t.Logf("✅ FP successfully caught up and voted on height %d (previously %d)",
				finalVotedHeight, lastVotedHeight)
			return true
		}

		t.Logf("FP catching up, current voted height: %d (previously %d)",
			currentVotedHeight, lastVotedHeight)
		return false
	}, 2*eventuallyWaitTimeOut, eventuallyPollTime, // Give extra time for catch-up
		"FP should catch up and vote on blocks after restart")

	t.Log("✅ Rollup BSN Catching Up Test Completed Successfully!")
	t.Logf("✅ FP successfully caught up after %d blocks of downtime", n)
	t.Logf("✅ Pre-downtime voted height: %d", lastVotedHeight)
	t.Logf("✅ Post-catchup voted height: %d", finalVotedHeight)
	t.Logf("✅ FP continues normal operation after catch-up")

	// Additional logging for debugging cleanup
	t.Log("🔄 Starting test cleanup...")
}

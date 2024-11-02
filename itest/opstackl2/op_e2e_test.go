//go:build e2e_op
// +build e2e_op

package e2etest_op

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	fgtypes "github.com/babylonlabs-io/finality-gadget/types"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	"github.com/babylonlabs-io/finality-provider/testutil/log"
	"github.com/stretchr/testify/require"
)

// TestOpFpNoVotingPower tests that the FP has no voting power if it has no BTC delegation
func TestOpFpNoVotingPower(t *testing.T) {
	ctm := StartOpL2ConsumerManager(t, 1)
	defer ctm.Stop(t)

	consumerFpPkList := ctm.RegisterConsumerFinalityProvider(t, 1)
	// start consumer chain FP
	fpList := ctm.StartConsumerFinalityProvider(t, consumerFpPkList)
	fpInstance := fpList[0]

	e2eutils.WaitForFpPubRandCommitted(t, fpInstance)
	// query the first committed pub rand
	opcc := ctm.getOpCCAtIndex(0)
	committedPubRand, err := queryFirstPublicRandCommit(opcc, fpInstance.GetBtcPk())
	require.NoError(t, err)
	committedStartHeight := committedPubRand.StartHeight
	t.Logf(log.Prefix("First committed pubrandList startHeight %d"), committedStartHeight)
	testBlocks := ctm.WaitForNBlocksAndReturn(t, committedStartHeight, 1)
	testBlock := testBlocks[0]

	queryBlock := &fgtypes.Block{
		BlockHeight:    testBlock.Height,
		BlockHash:      hex.EncodeToString(testBlock.Hash),
		BlockTimestamp: 12345, // doesn't matter b/c the BTC client is mocked
	}

	// no BTC delegation, so the FP has no voting power
	hasPower, err := opcc.QueryFinalityProviderHasPower(fpInstance.GetBtcPk(), queryBlock.BlockHeight)
	require.NoError(t, err)
	require.Equal(t, false, hasPower)

	_, err = ctm.FinalityGadget.QueryIsBlockBabylonFinalized(queryBlock)
	require.ErrorIs(t, err, fgtypes.ErrBtcStakingNotActivated)
}

// TestFinalityProviderLifeCycle tests the whole life cycle of a finality-provider
// creation -> registration -> randomness commitment ->
// activation with BTC delegation and Covenant sig ->
// vote submission -> block finalization
func TestFinalityProviderLifeCycle(t *testing.T) {
	ctm := StartOpL2ConsumerManager(t, 1)
	defer ctm.Stop(t)

	// register, get BTC delegations, and start FPs
	n := 1
	fpList := ctm.SetupFinalityProviders(t, n, []stakingParam{
		{e2eutils.StakingTime, e2eutils.StakingAmount},
	})

	// BTC delegations are activated after SetupFinalityProviders
	l2BlockAfterActivation, err := ctm.getOpCCAtIndex(0).QueryLatestBlockHeight()
	require.NoError(t, err)

	// check both FPs have committed their first public randomness
	// TODO: we might use go routine to do this in parallel
	for i := 0; i < n; i++ {
		// wait for the first block to be finalized since BTC staking is activated
		e2eutils.WaitForFpPubRandCommittedReachTargetHeight(t, fpList[i], l2BlockAfterActivation)
	}

	// FP will sign the activation block
	ctm.WaitForFpVoteReachHeight(t, fpList[0], l2BlockAfterActivation)

	testBlock, err := ctm.getOpCCAtIndex(0).QueryBlock(l2BlockAfterActivation)
	require.NoError(t, err)
	queryBlock := &fgtypes.Block{
		BlockHeight:    testBlock.Height,
		BlockHash:      hex.EncodeToString(testBlock.Hash),
		BlockTimestamp: 12345, // doesn't matter b/c the BTC client is mocked
	}
	finalized, err := ctm.FinalityGadget.QueryIsBlockBabylonFinalized(queryBlock)
	require.NoError(t, err)
	require.Equal(t, true, finalized)
	t.Logf(log.Prefix("Test case: block %d is finalized"), testBlock.Height)
}

func TestFinalityStuckAndRecover(t *testing.T) {
	ctm := StartOpL2ConsumerManager(t, 1)
	defer ctm.Stop(t)

	// register, get BTC delegations, and start FPs
	n := 1
	fpList := ctm.SetupFinalityProviders(t, n, []stakingParam{
		{e2eutils.StakingTime, e2eutils.StakingAmount},
	})
	fpInstance := fpList[0]

	// BTC delegations are activated after SetupFinalityProviders
	l2BlockAfterActivation, err := ctm.getOpCCAtIndex(0).QueryLatestBlockHeight()
	require.NoError(t, err)

	// wait for the first block to be finalized since BTC staking is activated
	e2eutils.WaitForFpPubRandCommittedReachTargetHeight(t, fpInstance, l2BlockAfterActivation)
	ctm.WaitForBlockFinalized(t, l2BlockAfterActivation)

	// stop the FP instance
	fpStopErr := fpInstance.Stop()
	require.NoError(t, fpStopErr)
	// make sure the FP is stopped
	require.Eventually(t, func() bool {
		return !fpInstance.IsRunning()
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
	t.Logf(log.Prefix("Stopped the FP instance %s"), fpInstance.GetBtcPkHex())

	// get the last voted height
	lastVotedHeight := fpInstance.GetLastVotedHeight()
	t.Logf(log.Prefix("last voted height %d"), lastVotedHeight)
	// wait until the block finalized
	require.Eventually(t, func() bool {
		latestFinalizedBlock, err := ctm.getOpCCAtIndex(0).QueryLatestFinalizedBlock()
		require.NoError(t, err)
		if latestFinalizedBlock == nil {
			return false
		}
		stuckHeight := latestFinalizedBlock.Height
		return lastVotedHeight == stuckHeight
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

	// check the finality gets stuck. wait for a while to make sure it is stuck
	time.Sleep(5 * ctm.getL1BlockTime())
	latestFinalizedBlock, err := ctm.getOpCCAtIndex(0).QueryLatestFinalizedBlock()
	require.NoError(t, err)
	require.NotNil(t, latestFinalizedBlock)
	stuckHeight := latestFinalizedBlock.Height
	require.Equal(t, lastVotedHeight, stuckHeight)
	t.Logf(log.Prefix("OP chain block finalized head stuck at height %d"), stuckHeight)

	// restart the FP instance
	fpStartErr := fpInstance.Start()
	require.NoError(t, fpStartErr)
	// make sure the FP is running
	require.Eventually(t, func() bool {
		return fpInstance.IsRunning()
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
	t.Logf(log.Prefix("Restarted the FP instance %s"), fpInstance.GetBtcPkHex())

	// wait for next finalized block > stuckHeight
	nextFinalizedHeight := ctm.WaitForBlockFinalized(t, stuckHeight+1)
	t.Logf(log.Prefix(
		"OP chain fianlity is recovered, the latest finalized block height %d",
	), nextFinalizedHeight)
}

func TestFinalityGadgetServer(t *testing.T) {
	// start the consumer manager
	ctm := StartOpL2ConsumerManager(t, 1)
	defer ctm.Stop(t)

	// register, get BTC delegations, and start FPs
	n := 1
	fpList := ctm.SetupFinalityProviders(t, n, []stakingParam{
		{e2eutils.StakingTime, e2eutils.StakingAmount},
	})

	// BTC delegations are activated after SetupFinalityProviders
	l2BlockAfterActivation, err := ctm.getOpCCAtIndex(0).QueryLatestBlockHeight()
	require.NoError(t, err)

	// check both FPs have committed their first public randomness
	// TODO: we might use go routine to do this in parallel
	for i := 0; i < n; i++ {
		// wait for the first block to be finalized since BTC staking is activated
		e2eutils.WaitForFpPubRandCommittedReachTargetHeight(t, fpList[i], l2BlockAfterActivation)
	}

	// FP will sign the two block
	ctm.WaitForFpVoteReachHeight(t, fpList[0], l2BlockAfterActivation)
	ctm.WaitForFpVoteReachHeight(t, fpList[0], l2BlockAfterActivation+1)

	// start finality gadget processing blocks
	ctx, cancel := context.WithCancel(context.Background())
	err = ctm.FinalityGadget.Startup(ctx)
	require.NoError(t, err)
	go func() {
		err := ctm.FinalityGadget.ProcessBlocks(ctx)
		require.NoError(t, err)
	}()

	// check latest block
	require.Eventually(t, func() bool {
		block, err := ctm.FinalityGadget.QueryLatestFinalizedBlock()
		if block == nil {
			return false
		}
		require.NoError(t, err)
		// check N blocks are processed as finalized
		// we pick a small N = 5 here to minimize the test time
		return block.BlockHeight > l2BlockAfterActivation+5
	}, 40*time.Second, 5*time.Second, "Failed to process blocks")

	// stop the finality gadget
	cancel()
}

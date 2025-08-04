package e2etest_bcd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	bbnappparams "github.com/babylonlabs-io/babylon-sdk/demo/app/params"
	appparams "github.com/babylonlabs-io/babylon/v3/app/params"
	"github.com/stretchr/testify/require"

	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
)

// TestConsumerFpLifecycle tests the consumer finality provider lifecycle
// 1. Upload Babylon and BTC staking contracts to bcd chain
// 2. Instantiate Babylon contract with admin
// 3. Register consumer chain to Babylon
// 4. Inject consumer fp in BTC staking contract using admin
// 6. Start the finality provider daemon and app
// 7. Wait for fp daemon to submit public randomness and finality signature
// 8. Inject consumer delegation in BTC staking contract using admin, this will give voting power to fp
// 9. Ensure fp has voting power in smart contract
// 10. Ensure finality sigs are being submitted by fp daemon and block is finalized
// NOTE: the delegation is injected after ensuring pub randomness loop in fp daemon has started
// this order is necessary otherwise pub randomness loop takes time to start and due to this blocks won't get finalized.
func TestConsumerFpLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ctm := StartBcdTestManager(t, ctx)
	defer func() {
		cancel()
		ctm.Stop(t)
	}()

	// setup the contracts
	bbnContracts := ctm.setupContracts(ctx, t)

	// setup relayer
	ctm.BcdHandler.SetContractAddress(bbnContracts.BabylonContract)
	ctm.BcdHandler.SetBabylonConfig("chain-test", ctm.FpConfig.BabylonConfig.RPCAddr, ctm.babylonKeyDir)

	err := ctm.BcdHandler.StartRelayer(t)
	require.NoError(t, err)

	// register consumer to babylon
	_, err = ctm.BabylonController.RegisterConsumerChain(bcdConsumerID, "Consumer chain 1 (test)", "Test Consumer Chain 1", "")
	require.NoError(t, err)

	// zone concierge channel is created after registering consumer fp
	err = ctm.BcdHandler.createZoneConciergeChannel(t)
	require.NoError(t, err)

	ctm.waitForZoneConciergeChannel(t)

	// register consumer fps to babylon
	// this will be submitted to babylon once fp daemon starts
	fp := ctm.CreateConsumerFinalityProviders(ctx, t, bcdConsumerID)
	fpPk := fp.GetBtcPkBIP340()

	res, err := ctm.BcdConsumerClient.QueryFinalityProvidersByTotalActiveSats(ctx)
	require.NoError(t, err)
	require.NotNil(t, res)

	// ensure pub rand is submitted to smart contract
	require.Eventually(t, func() bool {
		fpPubRandResp, err := ctm.BcdConsumerClient.QueryLastPublicRandCommit(ctx, fpPk.MustToBTCPK())
		if err != nil {
			t.Logf("failed to query last committed public rand: %s", err.Error())
			return false
		}
		if fpPubRandResp == nil {
			return false
		}

		return true
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

	// inject delegation in smart contract using admin
	// HACK: set account prefix to ensure the staker's address uses bbn prefix
	appparams.SetAddressPrefixes()
	delMsg := e2eutils.GenBtcStakingDelExecMsg(fpPk.MarshalHex())
	bbnappparams.SetAddressPrefixes()
	delMsgBytes, err := json.Marshal(delMsg)
	require.NoError(t, err)
	_, err = ctm.BcdConsumerClient.ExecuteBTCStakingContract(ctx, delMsgBytes)
	require.NoError(t, err)

	// query delegations in smart contract
	consumerDelsResp, err := ctm.BcdConsumerClient.QueryDelegations(ctx)
	require.NoError(t, err)
	require.NotNil(t, consumerDelsResp)
	require.Len(t, consumerDelsResp.Delegations, 1)
	require.Empty(t, consumerDelsResp.Delegations[0].UndelegationInfo.DelegatorUnbondingSig) // assert there is no delegator unbonding sig
	require.Equal(t, delMsg.BtcStaking.ActiveDel[0].BTCPkHex, consumerDelsResp.Delegations[0].BtcPkHex)
	require.Equal(t, delMsg.BtcStaking.ActiveDel[0].StartHeight, consumerDelsResp.Delegations[0].StartHeight)
	require.Equal(t, delMsg.BtcStaking.ActiveDel[0].EndHeight, consumerDelsResp.Delegations[0].EndHeight)
	require.Equal(t, delMsg.BtcStaking.ActiveDel[0].TotalSat, consumerDelsResp.Delegations[0].TotalSat)
	require.Equal(t, delMsg.BtcStaking.ActiveDel[0].StakingTx, consumerDelsResp.Delegations[0].StakingTx)
	require.Equal(t, delMsg.BtcStaking.ActiveDel[0].SlashingTx, consumerDelsResp.Delegations[0].SlashingTx)

	// ensure fp has positive total active sats in smart contract
	consumerFpsByPowerResp, err := ctm.BcdConsumerClient.QueryFinalityProvidersByTotalActiveSats(ctx)
	require.NoError(t, err)
	require.NotNil(t, consumerFpsByPowerResp)
	require.Len(t, consumerFpsByPowerResp.Fps, 1)
	require.Equal(t, fpPk.MarshalHex(), consumerFpsByPowerResp.Fps[0].BtcPkHex)
	require.Equal(t, delMsg.BtcStaking.ActiveDel[0].TotalSat, consumerFpsByPowerResp.Fps[0].TotalActiveSats)

	// wait for public randomness to be BTC timestamped
	ctm.BaseTestManager.WaitForFpPubRandTimestamped(t, fp)
	require.Eventually(t, func() bool {
		res, err := ctm.BcdConsumerClient.QueryLastBTCTimestampedHeader(ctx)
		if err != nil {
			t.Logf("failed to query last BTC timestamped header: %s", err.Error())
			return false
		}
		t.Logf("QueryLastBTCTimestampedHeader: height %d, bbn epoch %d", res.Height, res.BabylonEpoch)

		return res.Height > 0
	}, e2eutils.EventuallyWaitTimeOut, 5*time.Second)

	// wait for FP to vote on block
	var lastVotedHeight uint64
	require.Eventually(t, func() bool {
		if fp.GetLastVotedHeight() > 0 {
			lastVotedHeight = fp.GetLastVotedHeight()
			t.Logf("FP voted on block at height: %d", lastVotedHeight)
			return true
		}
		return false
	}, e2eutils.EventuallyWaitTimeOut, 2*time.Second, "FP should automatically vote on rollup blocks")

	// wait for finality signature to be included in the smart contract
	require.Eventually(t, func() bool {
		fpSigsResponse, err := ctm.BcdConsumerClient.QueryFinalitySignature(ctx, fpPk.MarshalHex(), lastVotedHeight)
		if err != nil {
			t.Logf("failed to query finality signature: %s", err.Error())
			return false
		}
		if fpSigsResponse == nil || len(fpSigsResponse.Signature) == 0 {
			t.Logf("finality signature not found for height %d", lastVotedHeight)
			return false
		}
		return true
	}, e2eutils.EventuallyWaitTimeOut, 3*time.Second)

	// wait for the block to be finalized
	require.Eventually(t, func() bool {
		lastFin, err := ctm.BcdConsumerClient.QueryLatestFinalizedBlock(ctx)
		require.NoError(t, err)
		if lastFin != nil {
			t.Logf("Latest finalized block height: %d is finalized %v", lastFin.GetHeight(), lastFin.IsFinalized())
		}
		idxBlockedResponse, err := ctm.BcdConsumerClient.QueryIndexedBlock(ctx, lastVotedHeight)
		if err != nil {
			t.Logf("failed to query indexed block: %s", err.Error())
			return false
		}
		if idxBlockedResponse == nil {
			return false
		}

		// Additional if statement to check finalization status
		if idxBlockedResponse.Finalized {
			t.Logf("Block at height %d is now finalized", lastVotedHeight)
			return true
		}

		return false
	}, e2eutils.EventuallyWaitTimeOut, 2*time.Second)
}

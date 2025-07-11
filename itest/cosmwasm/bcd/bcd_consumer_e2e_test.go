//go:build e2e_bcd
// +build e2e_bcd

package e2etest_bcd

import (
	"context"
	"encoding/json"
	"testing"

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
	defer cancel()
	ctm := StartBcdTestManager(t, ctx)
	defer ctm.Stop(t)

	// store and instantiate Cosmos BSN contracts
	ctm.DeployContracts(t, ctx)

	// register consumer to babylon
	_, err := ctm.BBNClient.RegisterConsumerChain(bcdConsumerID, "Consumer chain 1 (test)", "Test Consumer Chain 1", "")
	require.NoError(t, err)

	// register consumer fps to babylon
	// this will be submitted to babylon once fp daemon starts
	fp := ctm.CreateConsumerFinalityProviders(t, bcdConsumerID)
	fpPk := fp.GetBtcPkBIP340()

	// ensure pub rand is submitted to smart contract
	require.Eventually(t, func() bool {
		fpPubRandResp, err := ctm.BcdConsumerClient.QueryLastPublicRandCommit(fpPk.MustToBTCPK())
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
	_, err = ctm.BcdConsumerClient.ExecuteBTCStakingContract(delMsgBytes)
	require.NoError(t, err)

	// query delegations in smart contract
	consumerDelsResp, err := ctm.BcdConsumerClient.QueryDelegations()
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

	// ensure fp has voting power in smart contract
	consumerFpsByPowerResp, err := ctm.BcdConsumerClient.QueryFinalityProvidersByPower()
	require.NoError(t, err)
	require.NotNil(t, consumerFpsByPowerResp)
	require.Len(t, consumerFpsByPowerResp.Fps, 1)
	require.Equal(t, fpPk.MarshalHex(), consumerFpsByPowerResp.Fps[0].BtcPkHex)
	require.Equal(t, delMsg.BtcStaking.ActiveDel[0].TotalSat, consumerFpsByPowerResp.Fps[0].Power)

	// get comet latest height
	wasmdNodeStatus, err := ctm.BcdConsumerClient.GetCometNodeStatus()
	require.NoError(t, err)
	// TODO: this is a hack as its possible that latest comet height is less than activated height
	//  and the sigs/finalization can only happen after activated height
	lookupHeight := wasmdNodeStatus.SyncInfo.LatestBlockHeight + 5

	// ensure finality signature is submitted to smart contract
	require.Eventually(t, func() bool {
		fpSigsResponse, err := ctm.BcdConsumerClient.QueryFinalitySignature(fpPk.MarshalHex(), uint64(lookupHeight))
		if err != nil {
			t.Logf("failed to query finality signature: %s", err.Error())
			return false
		}
		if fpSigsResponse == nil || fpSigsResponse.Signature == nil || len(fpSigsResponse.Signature) == 0 {
			return false
		}
		return true
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

	// ensure latest comet block is finalized
	require.Eventually(t, func() bool {
		idxBlockedResponse, err := ctm.BcdConsumerClient.QueryIndexedBlock(uint64(lookupHeight))
		if err != nil {
			t.Logf("failed to query indexed block: %s", err.Error())
			return false
		}
		if idxBlockedResponse == nil {
			return false
		}
		if !idxBlockedResponse.Finalized {
			return false
		}
		return true
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)
}

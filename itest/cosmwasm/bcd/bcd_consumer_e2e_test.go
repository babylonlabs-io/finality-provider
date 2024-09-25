//go:build e2e_bcd
// +build e2e_bcd

package e2etest_bcd

import (
	"encoding/json"
	"strings"
	"testing"

	sdkmath "cosmossdk.io/math"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
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
	ctm := StartBcdTestManager(t)
	defer ctm.Stop(t)

	// store babylon contract
	babylonContractPath := "../../bytecode/babylon_contract.wasm"
	err := ctm.BcdConsumerClient.StoreWasmCode(babylonContractPath)
	require.NoError(t, err)
	babylonContractWasmId, err := ctm.BcdConsumerClient.GetLatestCodeId()
	require.NoError(t, err)
	require.Equal(t, uint64(1), babylonContractWasmId)

	// store btc staking contract
	btcStakingContractPath := "../../bytecode/btc_staking.wasm"
	err = ctm.BcdConsumerClient.StoreWasmCode(btcStakingContractPath)
	require.NoError(t, err)
	btcStakingContractWasmId, err := ctm.BcdConsumerClient.GetLatestCodeId()
	require.NoError(t, err)
	require.Equal(t, uint64(2), btcStakingContractWasmId)

	// instantiate babylon contract with admin
	btcStakingInitMsg := map[string]interface{}{
		"admin": ctm.BcdConsumerClient.MustGetValidatorAddress(),
	}
	btcStakingInitMsgBytes, err := json.Marshal(btcStakingInitMsg)
	require.NoError(t, err)
	initMsg := map[string]interface{}{
		"network":                         "regtest",
		"babylon_tag":                     "01020304",
		"btc_confirmation_depth":          1,
		"checkpoint_finalization_timeout": 2,
		"notify_cosmos_zone":              false,
		"btc_staking_code_id":             btcStakingContractWasmId,
		"btc_staking_msg":                 btcStakingInitMsgBytes,
		"admin":                           ctm.BcdConsumerClient.MustGetValidatorAddress(),
	}
	initMsgBytes, err := json.Marshal(initMsg)
	require.NoError(t, err)
	err = ctm.BcdConsumerClient.InstantiateContract(babylonContractWasmId, initMsgBytes)
	require.NoError(t, err)

	// get btc staking contract address
	resp, err := ctm.BcdConsumerClient.ListContractsByCode(btcStakingContractWasmId, &sdkquerytypes.PageRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Contracts, 1)
	btcStakingContractAddr := sdk.MustAccAddressFromBech32(resp.Contracts[0])
	// update the contract address in config because during setup we had used a random address which is not valid
	ctm.BcdConsumerClient.SetBtcStakingContractAddress(btcStakingContractAddr.String())

	// register consumer to babylon
	_, err = ctm.BBNClient.RegisterConsumerChain(bcdChainID, "Consumer chain 1 (test)", "Test Consumer Chain 1")
	require.NoError(t, err)

	// register consumer fps to babylon
	// this will be submitted to babylon once fp daemon starts
	fps := ctm.CreateConsumerFinalityProviders(t, bcdChainID, 1)
	fpPk := fps[0].GetBtcPkBIP340()
	fpPkHex := fpPk.MarshalHex()

	// inject fp in smart contract using admin
	fpMsg := e2eutils.GenBtcStakingFpExecMsg(fpPkHex)
	fpMsgBytes, err := json.Marshal(fpMsg)
	require.NoError(t, err)
	_, err = ctm.BcdConsumerClient.ExecuteContract(fpMsgBytes)
	require.NoError(t, err)

	// query finality providers in smart contract
	consumerFpsResp, err := ctm.BcdConsumerClient.QueryFinalityProviders()
	require.NoError(t, err)
	require.NotNil(t, consumerFpsResp)
	require.Len(t, consumerFpsResp.Fps, 1)
	require.Equal(t, fpMsg.BtcStaking.NewFP[0].ConsumerID, consumerFpsResp.Fps[0].ConsumerId)
	require.Equal(t, fpMsg.BtcStaking.NewFP[0].BTCPKHex, consumerFpsResp.Fps[0].BtcPkHex)

	// ensure consumer finality providers are stored in Babylon
	// this will happen after the finality provider daemon has started
	require.Eventually(t, func() bool {
		fps, err := ctm.BBNClient.QueryConsumerFinalityProviders(bcdChainID)
		if err != nil {
			t.Logf("failed to query finality providers from Babylon %s", err.Error())
			return false
		}

		if len(fps) != 1 {
			return false
		}

		if !strings.Contains(fps[0].Description.Moniker, e2eutils.MonikerPrefix) {
			return false
		}
		if !fps[0].Commission.Equal(sdkmath.LegacyZeroDec()) {
			return false
		}

		return true
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

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
	delMsg := e2eutils.GenBtcStakingDelExecMsg(fpPk.MarshalHex())
	delMsgBytes, err := json.Marshal(delMsg)
	require.NoError(t, err)
	_, err = ctm.BcdConsumerClient.ExecuteContract(delMsgBytes)
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
	require.Equal(t, fpMsg.BtcStaking.NewFP[0].BTCPKHex, consumerFpsByPowerResp.Fps[0].BtcPkHex)
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

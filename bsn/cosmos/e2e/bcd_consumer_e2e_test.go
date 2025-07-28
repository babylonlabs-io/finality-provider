package e2etest_bcd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	wasmdtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"testing"

	"cosmossdk.io/errors"
	bbnappparams "github.com/babylonlabs-io/babylon-sdk/demo/app/params"
	appparams "github.com/babylonlabs-io/babylon/v3/app/params"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
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

	// store babylon contract
	babylonContractPath := "./bytecode/babylon_contract.wasm"
	err := ctm.BcdConsumerClient.StoreWasmCode(babylonContractPath)
	require.NoError(t, err)
	babylonContractWasmId, err := ctm.BcdConsumerClient.GetLatestCodeID()
	require.NoError(t, err)
	require.Equal(t, uint64(1), babylonContractWasmId)

	// store btc staking contract
	btcStakingContractPath := "./bytecode/btc_staking.wasm"
	err = ctm.BcdConsumerClient.StoreWasmCode(btcStakingContractPath)
	require.NoError(t, err)
	btcStakingContractWasmId, err := ctm.BcdConsumerClient.GetLatestCodeID()
	require.NoError(t, err)
	require.Equal(t, uint64(2), btcStakingContractWasmId)

	// store btc finality contract
	btcFinalityContractPath := "./bytecode/btc_finality.wasm"
	err = ctm.BcdConsumerClient.StoreWasmCode(btcFinalityContractPath)
	require.NoError(t, err)
	btcFinalityContractWasmId, err := ctm.BcdConsumerClient.GetLatestCodeID()
	require.NoError(t, err)
	require.Equal(t, uint64(3), btcFinalityContractWasmId)

	btcLightClientPath := "./bytecode/btc_light_client.wasm"
	err = ctm.BcdConsumerClient.StoreWasmCode(btcLightClientPath)
	require.NoError(t, err)
	btcLightClientWasmId, err := ctm.BcdConsumerClient.GetLatestCodeID()
	require.NoError(t, err)
	require.Equal(t, uint64(4), btcLightClientWasmId)

	network := "regtest"
	btcConfirmationDepth := 1
	btcFinalizationTimeout := 2
	admin := ctm.BcdConsumerClient.MustGetValidatorAddress()
	btcLightClientInitMsg := fmt.Sprintf(`{"network":"%s","btc_confirmation_depth":%d,"checkpoint_finalization_timeout":%d}`,
		network, btcConfirmationDepth, btcFinalizationTimeout)
	btcFinalityInitMsg := fmt.Sprintf(`{"admin":"%s"}`, admin)
	btcStakingInitMsg := fmt.Sprintf(`{"admin":"%s"}`, admin)
	btcLightClientInitMsgBz := base64.StdEncoding.EncodeToString([]byte(btcLightClientInitMsg))
	btcFinalityInitMsgBz := base64.StdEncoding.EncodeToString([]byte(btcFinalityInitMsg))
	btcStakingInitMsgBz := base64.StdEncoding.EncodeToString([]byte(btcStakingInitMsg))

	babylonInitMsg := map[string]interface{}{
		"network":                         network,
		"babylon_tag":                     "01020304",
		"btc_confirmation_depth":          btcConfirmationDepth,
		"checkpoint_finalization_timeout": btcFinalizationTimeout,
		"notify_cosmos_zone":              false,
		"btc_light_client_code_id":        btcLightClientWasmId,
		"btc_light_client_msg":            btcLightClientInitMsgBz,
		"btc_staking_code_id":             btcStakingContractWasmId,
		"btc_staking_msg":                 btcStakingInitMsgBz,
		"btc_finality_code_id":            btcFinalityContractWasmId,
		"btc_finality_msg":                btcFinalityInitMsgBz,
		"consumer_name":                   "test-consumer",
		"consumer_description":            "test-consumer-description",
	}
	babylonInitMsgBz, err := json.Marshal(babylonInitMsg)
	require.NoError(t, err)

	msg := &wasmdtypes.MsgInstantiateContract{
		Sender: ctm.BcdConsumerClient.MustGetValidatorAddress(),
		Admin:  ctm.BcdConsumerClient.MustGetValidatorAddress(),
		CodeID: babylonContractWasmId,
		Label:  "cw",
		Msg:    babylonInitMsgBz,
		Funds:  nil,
	}

	instResp, err := ctm.BcdConsumerClient.GetClient().ReliablySendMsg(ctx, msg, []*errors.Error{}, []*errors.Error{})
	require.NoError(t, err)
	require.NotNil(t, instResp)

	// get btc staking contract address
	resp, err := ctm.BcdConsumerClient.ListContractsByCode(btcStakingContractWasmId, &sdkquerytypes.PageRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Contracts, 1)
	btcStakingContractAddr := sdk.MustAccAddressFromBech32(resp.Contracts[0])
	// update the contract address
	btcStakingContractAddrStr := sdk.MustBech32ifyAddressBytes("bbnc", btcStakingContractAddr)
	ctm.BcdConsumerClient.SetBtcStakingContractAddress(btcStakingContractAddrStr)

	// get btc finality contract address
	resp, err = ctm.BcdConsumerClient.ListContractsByCode(btcFinalityContractWasmId, &sdkquerytypes.PageRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Contracts, 1)
	btcFinalityContractAddr := sdk.MustAccAddressFromBech32(resp.Contracts[0])
	// update the contract address
	btcFinalityContractAddrStr := sdk.MustBech32ifyAddressBytes("bbnc", btcFinalityContractAddr)
	ctm.BcdConsumerClient.SetBtcFinalityContractAddress(btcFinalityContractAddrStr)

	// register consumer to babylon
	_, err = ctm.BabylonController.RegisterConsumerChain(bcdConsumerID, "Consumer chain 1 (test)", "Test Consumer Chain 1", "")
	require.NoError(t, err)

	// register consumer fps to babylon
	// this will be submitted to babylon once fp daemon starts
	fp := ctm.CreateConsumerFinalityProviders(t, bcdConsumerID)
	fpPk := fp.GetBtcPkBIP340()

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

	// ensure fp has voting power in smart contract
	consumerFpsByPowerResp, err := ctm.BcdConsumerClient.QueryFinalityProvidersByPower(ctx)
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
		fpSigsResponse, err := ctm.BcdConsumerClient.QueryFinalitySignature(ctx, fpPk.MarshalHex(), uint64(lookupHeight))
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
		idxBlockedResponse, err := ctm.BcdConsumerClient.QueryIndexedBlock(context.Background(), uint64(lookupHeight))
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

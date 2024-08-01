//go:build e2e_wasmd
// +build e2e_wasmd

package e2etest_wasmd

import (
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	common "github.com/babylonlabs-io/finality-provider/itest"
	sdk "github.com/cosmos/cosmos-sdk/types"

	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
)

// TestConsumerFpDataInjection tests the finality provider lifecycle by manual injection using contract admin
// NOTE: this doesn't use the fp app or fp daemon
// 1. Upload Babylon and BTC staking contracts to wasmd chain
// 2. Instantiate Babylon contract with admin
// 3. Inject finality provider and delegation in BTC staking contract using admin
// 4. Inject public randomness commitment in BTC staking contract
// 5. Inject finality signature in BTC staking contract
func TestConsumerFpDataInjection(t *testing.T) {
	ctm := StartWasmdTestManager(t)
	defer ctm.Stop(t)

	// store babylon contract
	babylonContractPath := "../../bytecode/babylon_contract.wasm"
	err := ctm.WasmdConsumerClient.StoreWasmCode(babylonContractPath)
	require.NoError(t, err)
	babylonContractWasmId, err := ctm.WasmdConsumerClient.GetLatestCodeId()
	require.NoError(t, err)
	require.Equal(t, uint64(1), babylonContractWasmId)

	// store btc staking contract
	btcStakingContractPath := "../../bytecode/btc_staking.wasm"
	err = ctm.WasmdConsumerClient.StoreWasmCode(btcStakingContractPath)
	require.NoError(t, err)
	btcStakingContractWasmId, err := ctm.WasmdConsumerClient.GetLatestCodeId()
	require.NoError(t, err)
	require.Equal(t, uint64(2), btcStakingContractWasmId)

	// instantiate babylon contract with admin
	btcStakingInitMsg := map[string]interface{}{
		"admin": ctm.WasmdConsumerClient.MustGetValidatorAddress(),
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
		"admin":                           ctm.WasmdConsumerClient.MustGetValidatorAddress(),
	}
	initMsgBytes, err := json.Marshal(initMsg)
	require.NoError(t, err)
	err = ctm.WasmdConsumerClient.InstantiateContract(babylonContractWasmId, initMsgBytes)
	require.NoError(t, err)

	// get btc staking contract address
	resp, err := ctm.WasmdConsumerClient.ListContractsByCode(btcStakingContractWasmId, &sdkquerytypes.PageRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Contracts, 1)
	btcStakingContractAddr := sdk.MustAccAddressFromBech32(resp.Contracts[0])
	// update the contract address in config because during setup we had used a random address which is not valid
	ctm.WasmdConsumerClient.SetBtcStakingContractAddress(btcStakingContractAddr.String())

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	fpSk, _, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)
	randList, msgPub, err := common.GenCommitPubRandListMsg(r, fpSk, 1, 1000)
	require.NoError(t, err)

	// inject fp and delegation in smart contract using admin
	msg := common.GenBtcStakingExecMsg(msgPub.FpBtcPk.MarshalHex())
	msgBytes, err := json.Marshal(msg)
	require.NoError(t, err)
	_, err = ctm.WasmdConsumerClient.ExecuteContract(msgBytes)
	require.NoError(t, err)

	// query finality providers in smart contract
	consumerFpsResp, err := ctm.WasmdConsumerClient.QueryFinalityProviders()
	require.NoError(t, err)
	require.NotNil(t, consumerFpsResp)
	require.Len(t, consumerFpsResp.Fps, 1)
	require.Equal(t, msg.BtcStaking.NewFP[0].ConsumerID, consumerFpsResp.Fps[0].ConsumerId)
	require.Equal(t, msg.BtcStaking.NewFP[0].BTCPKHex, consumerFpsResp.Fps[0].BtcPkHex)

	// query delegations in smart contract
	consumerDelsResp, err := ctm.WasmdConsumerClient.QueryDelegations()
	require.NoError(t, err)
	require.NotNil(t, consumerDelsResp)
	require.Len(t, consumerDelsResp.Delegations, 1)
	require.Empty(t, consumerDelsResp.Delegations[0].UndelegationInfo.DelegatorUnbondingSig) // assert there is no delegator unbonding sig
	require.Equal(t, msg.BtcStaking.ActiveDel[0].BTCPkHex, consumerDelsResp.Delegations[0].BtcPkHex)
	require.Equal(t, msg.BtcStaking.ActiveDel[0].StartHeight, consumerDelsResp.Delegations[0].StartHeight)
	require.Equal(t, msg.BtcStaking.ActiveDel[0].EndHeight, consumerDelsResp.Delegations[0].EndHeight)
	require.Equal(t, msg.BtcStaking.ActiveDel[0].TotalSat, consumerDelsResp.Delegations[0].TotalSat)
	require.Equal(t, msg.BtcStaking.ActiveDel[0].StakingTx, consumerDelsResp.Delegations[0].StakingTx)
	require.Equal(t, msg.BtcStaking.ActiveDel[0].SlashingTx, consumerDelsResp.Delegations[0].SlashingTx)

	// ensure fp has voting power in smart contract
	consumerFpsByPowerResp, err := ctm.WasmdConsumerClient.QueryFinalityProvidersByPower()
	require.NoError(t, err)
	require.NotNil(t, consumerFpsByPowerResp)
	require.Len(t, consumerFpsByPowerResp.Fps, 1)
	require.Equal(t, msg.BtcStaking.NewFP[0].BTCPKHex, consumerFpsByPowerResp.Fps[0].BtcPkHex)
	require.Equal(t, consumerDelsResp.Delegations[0].TotalSat, consumerFpsByPowerResp.Fps[0].Power)

	// inject pub rand commitment in smart contract (admin is not required, although in the tests admin and sender are the same)
	msg2 := common.GenPubRandomnessExecMsg(
		msgPub.FpBtcPk.MarshalHex(),
		msgPub.Commitment,
		msgPub.Sig.MustMarshal(),
		msgPub.StartHeight,
		msgPub.NumPubRand,
	)
	msgBytes2, err := json.Marshal(msg2)
	require.NoError(t, err)
	_, err = ctm.WasmdConsumerClient.ExecuteContract(msgBytes2)
	require.NoError(t, err)

	// inject finality signature in smart contract (admin is not required, although in the tests admin and sender are the same)
	wasmdNodeStatus, err := ctm.WasmdConsumerClient.GetCometNodeStatus()
	require.NoError(t, err)
	cometLatestHeight := wasmdNodeStatus.SyncInfo.LatestBlockHeight
	finalitySigMsg := common.GenFinalitySigExecMsg(uint64(1), uint64(cometLatestHeight), randList, fpSk)
	finalitySigMsgBytes, err := json.Marshal(finalitySigMsg)
	require.NoError(t, err)
	_, err = ctm.WasmdConsumerClient.ExecuteContract(finalitySigMsgBytes)
	require.NoError(t, err)
	fpSigsResponse, err := ctm.WasmdConsumerClient.QueryFinalitySignature(msgPub.FpBtcPk.MarshalHex(), uint64(cometLatestHeight))
	require.NoError(t, err)
	require.NotNil(t, fpSigsResponse)
	require.NotNil(t, fpSigsResponse.Signature)
	require.Equal(t, finalitySigMsg.SubmitFinalitySignature.Signature, fpSigsResponse.Signature)
}

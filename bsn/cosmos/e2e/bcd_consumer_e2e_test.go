package e2etest_bcd

import (
	"context"
	"encoding/json"
	"github.com/babylonlabs-io/finality-provider/bsn/cosmos/cmd/cosmos-fpd/daemon"
	"github.com/babylonlabs-io/finality-provider/bsn/cosmos/config"
	cfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/types"
	goflags "github.com/jessevdk/go-flags"
	"os"
	"path/filepath"
	"testing"
	"time"

	bbnappparams "github.com/babylonlabs-io/babylon-sdk/demo/app/params"
	appparams "github.com/babylonlabs-io/babylon/v3/app/params"
	"github.com/stretchr/testify/require"

	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
)

// TestConsumerFpLifecycle tests the complete consumer finality provider lifecycle
// including contract setup, registration, delegation, and block finalization.
//
// Test Steps:
// 1. Upload Babylon and BTC staking contracts to bcd chain
// 2. Instantiate Babylon contract with admin
// 3. Setup relayer with contract addresses and Babylon configuration
// 4. Register consumer chain to Babylon
// 5. Create zone concierge channel for consumer communication
// 6. Register consumer finality provider (FP) to Babylon
// 7. Wait for FP daemon to submit public randomness commits to smart contract
// 8. Inject consumer delegation in BTC staking contract using admin (gives voting power to FP)
// 9. Verify FP has positive total active sats (voting power) in smart contract
// 10. Wait for current block to be BTC timestamped to finalize pub rand commits
// 11. Wait for FP to vote on rollup blocks and submit finality signatures
// 12. Verify finality signatures are included in the smart contract
// 13. Ensure blocks are finalized based on FP votes
//
// NOTE: The delegation is injected after ensuring the public randomness loop in the FP daemon
// has started. This order is critical because the pub randomness loop takes time to initialize,
// and without it, blocks won't get finalized properly.
func TestConsumerFpLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
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

	// ensure fp has positive total active sats in smart contract
	consumerFpsByPowerResp, err := ctm.BcdConsumerClient.QueryFinalityProvidersByTotalActiveSats(ctx)
	require.NoError(t, err)
	require.NotNil(t, consumerFpsByPowerResp)
	require.Len(t, consumerFpsByPowerResp.Fps, 1)
	require.Equal(t, fpPk.MarshalHex(), consumerFpsByPowerResp.Fps[0].BtcPkHex)
	require.Equal(t, delMsg.BtcStaking.ActiveDel[0].TotalSat, consumerFpsByPowerResp.Fps[0].TotalActiveSats)

	// wait for current block to be BTC timestamped
	// thus some pub rand commit will be finalized
	nodeStatus, err := ctm.BcdConsumerClient.GetClient().GetStatus(ctx)
	require.NoError(t, err)
	curHeight := uint64(nodeStatus.SyncInfo.LatestBlockHeight)
	ctm.WaitForTimestampedHeight(t, ctx, curHeight)

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

	list, err := ctm.BcdConsumerClient.QueryPublicRandCommitList(ctx, fpPk.MustToBTCPK(), 1)
	require.NoError(t, err)

	require.NotNil(t, list)

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

func TestConsumerRecoverRandProofCmd(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
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

	// ensure fp has positive total active sats in smart contract
	consumerFpsByPowerResp, err := ctm.BcdConsumerClient.QueryFinalityProvidersByTotalActiveSats(ctx)
	require.NoError(t, err)
	require.NotNil(t, consumerFpsByPowerResp)
	require.Len(t, consumerFpsByPowerResp.Fps, 1)
	require.Equal(t, fpPk.MarshalHex(), consumerFpsByPowerResp.Fps[0].BtcPkHex)
	require.Equal(t, delMsg.BtcStaking.ActiveDel[0].TotalSat, consumerFpsByPowerResp.Fps[0].TotalActiveSats)

	// wait for current block to be BTC timestamped
	// thus some pub rand commit will be finalized
	nodeStatus, err := ctm.BcdConsumerClient.GetClient().GetStatus(ctx)
	require.NoError(t, err)
	curHeight := uint64(nodeStatus.SyncInfo.LatestBlockHeight)
	ctm.WaitForTimestampedHeight(t, ctx, curHeight)

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

	var finalizedBlock types.BlockDescription
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
			finalizedBlock = lastFin
			return true
		}

		return false
	}, e2eutils.EventuallyWaitTimeOut, 2*time.Second)

	cosmosCfg := config.CosmosFPConfig{
		Cosmwasm: ctm.cfg,
		Common:   ctm.FpConfig,
	}
	// delete the db file
	dbPath := filepath.Join(cosmosCfg.Common.DatabaseConfig.DBPath, cosmosCfg.Common.DatabaseConfig.DBFileName)
	err = os.Remove(dbPath)
	require.NoError(t, err)

	cosmosCfg.Common.EOTSManagerAddress = ctm.EOTSServerHandler.Config().RPCListener
	fpHomePath := filepath.Dir(cosmosCfg.Common.DatabaseConfig.DBPath)
	fileParser := goflags.NewParser(&cosmosCfg, goflags.Default)
	err = goflags.NewIniParser(fileParser).WriteFile(cfg.CfgFile(fpHomePath), goflags.IniIncludeDefaults)
	require.NoError(t, err)

	cmd := daemon.CommandRecoverProof("cosmos")
	cmd.SetArgs([]string{
		fp.GetBtcPkHex(),
		"--home=" + fpHomePath,
		"--chain-id=" + bcdChainID,
	})
	err = cmd.Execute()
	require.NoError(t, err)

	// assert db exists
	_, err = os.Stat(dbPath)
	require.NoError(t, err)

	fpdb, err := cosmosCfg.Common.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)

	pubRandStore, err := store.NewPubRandProofStore(fpdb)
	require.NoError(t, err)
	_, err = pubRandStore.GetPubRandProof([]byte(bcdChainID), fp.GetBtcPkBIP340().MustMarshal(), finalizedBlock.GetHeight())
	require.NoError(t, err)
}

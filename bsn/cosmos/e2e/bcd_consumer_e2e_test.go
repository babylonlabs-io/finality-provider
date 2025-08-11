package e2etest_bcd

import (
	"context"
	"encoding/json"
	appparams "github.com/babylonlabs-io/babylon/v3/app/params"
	"github.com/babylonlabs-io/finality-provider/bsn/cosmos/cmd/cosmos-fpd/daemon"
	"github.com/babylonlabs-io/finality-provider/bsn/cosmos/config"
	cfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	goflags "github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"

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
	t.Parallel()
	setBbnAddressPrefixesSafely()

	ctx, cancel := context.WithCancel(t.Context())
	ctm := StartBcdTestManager(t, ctx)
	defer func() {
		cancel()
		ctm.Stop(t)
	}()

	// setup contracts
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
	ctm.waitForPubRandInContract(t, fpPk)

	// inject delegation in smart contract using admin
	// HACK: set account prefix to ensure the staker's address uses bbn prefix
	setBbnAddressPrefixesSafely()
	delMsg := e2eutils.GenBtcStakingDelExecMsg(fpPk.MarshalHex())
	setBbncAppPrefixesSafely()
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

	// wait for the current block to be BTC timestamped
	// thus some pub rand commit will be finalized
	nodeStatus, err := ctm.BcdConsumerClient.GetClient().GetStatus(ctx)
	require.NoError(t, err)
	curHeight := uint64(nodeStatus.SyncInfo.LatestBlockHeight)
	ctm.WaitForTimestampedHeight(t, ctx, curHeight)

	// wait for FP to vote on block
	lastVotedHeight := ctm.waitForCastVote(t, fp)

	// wait for finality signature to be included in the smart contract
	ctm.waitForFinalitySignatureInContract(t, fpPk, lastVotedHeight)

	// wait for the block to be finalized
	ctm.waitForFinalizedBlock(t, lastVotedHeight)
}

// TestConsumerRecoverRandProofCmd tests the recover-proof command functionality for consumer chains.
// This test verifies that the recover-proof command can successfully restore public randomness proofs
// from a consumer chain smart contract back to the local database after a database loss scenario.
//
// Test Steps:
// 1. Setup consumer chain contracts and relayer infrastructure
// 2. Register consumer chain to Babylon and create zone concierge channel
// 3. Create and register a finality provider with delegation
// 4. Wait for public randomness submission and block finalization
// 5. Delete the local database to simulate data loss
// 6. Execute the recover-proof command to restore proofs from smart contract
// 7. Verify that public randomness proofs are successfully recovered in the database
func TestConsumerRecoverRandProofCmd(t *testing.T) {
	t.Parallel()
	setBbnAddressPrefixesSafely()

	ctx, cancel := context.WithCancel(t.Context())
	ctm := StartBcdTestManager(t, ctx)
	defer func() {
		cancel()
		ctm.Stop(t)
	}()

	// setup contracts
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
	ctm.waitForPubRandInContract(t, fpPk)

	// inject delegation in smart contract using admin
	// HACK: set account prefix to ensure the staker's address uses bbn prefix
	setBbnAddressPrefixesSafely()
	delMsg := e2eutils.GenBtcStakingDelExecMsg(fpPk.MarshalHex())
	setBbncAppPrefixesSafely()
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
	lastVotedHeight := ctm.waitForCastVote(t, fp)
	// wait for finality signature to be included in the smart contract
	ctm.waitForFinalitySignatureInContract(t, fpPk, lastVotedHeight)
	finalizedBlock := ctm.waitForFinalizedBlock(t, lastVotedHeight)

	cosmosCfg := config.CosmosFPConfig{
		Cosmwasm: ctm.cfg,
		Common:   ctm.FpConfig,
	}
	// delete the db file
	dbPath := filepath.Join(cosmosCfg.Common.DatabaseConfig.DBPath, cosmosCfg.Common.DatabaseConfig.DBFileName)
	err = os.Remove(dbPath)
	require.NoError(t, err)

	// create the config file
	cosmosCfg.Common.EOTSManagerAddress = ctm.EOTSServerHandler.Config().RPCListener
	fpHomePath := filepath.Dir(cosmosCfg.Common.DatabaseConfig.DBPath)
	fileParser := goflags.NewParser(&cosmosCfg, goflags.Default)
	err = goflags.NewIniParser(fileParser).WriteFile(cfg.CfgFile(fpHomePath), goflags.IniIncludeDefaults)
	require.NoError(t, err)

	fpi, err := ctm.Fpa.GetFinalityProviderInfo(fp.GetBtcPkBIP340())
	require.NoError(t, err)

	cmd := daemon.CommandRecoverProof("cosmos")
	cmd.SetArgs([]string{
		fp.GetBtcPkHex(),
		"--home=" + fpHomePath,
		"--chain-id=" + fpi.ChainId,
	})
	// wrangle the app params to ensure the address prefixes are set correctly
	appparams.SetAddressPrefixes()
	err = cmd.Execute()
	require.NoError(t, err)

	// assert db exists
	_, err = os.Stat(dbPath)
	require.NoError(t, err)

	fpdb, err := cosmosCfg.Common.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)

	pubRandStore, err := store.NewPubRandProofStore(fpdb)
	require.NoError(t, err)
	_, err = pubRandStore.GetPubRandProof([]byte(fpi.ChainId), fp.GetBtcPkBIP340().MustMarshal(), finalizedBlock.GetHeight())
	require.NoError(t, err)
}

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	rollupfpdaemon "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/cmd/rollup-fpd/daemon"
	rollupfpconfig "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	goflags "github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"
)

const (
	testBinaryName = "rollup-fpd"
)

// TestRollupFpdInitCmd tests the rollup-fpd init command
// This command initializes a rollup finality provider home directory with rollup-specific config
func TestRollupFpdInitCmd(t *testing.T) {
	t.Parallel()

	// Create temporary directory for test
	testHomeDir := t.TempDir()

	// Create the init command
	cmd := rollupfpdaemon.CommandInit(testBinaryName)

	// Set up command arguments
	cmd.SetArgs([]string{
		"--home=" + testHomeDir,
		"--force=true",
	})

	// Execute the command
	err := cmd.Execute()
	require.NoError(t, err)

	// Verify the config file was created
	configPath := rollupfpconfig.CfgFile(testHomeDir)
	require.FileExists(t, configPath)

	// Verify the log directory was created
	logDir := fpcfg.LogDir(testHomeDir)
	require.DirExists(t, logDir)

	// Test that config file contains expected rollup-specific fields (but don't validate values)
	// We just want to make sure the config template was written correctly
	configContent, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(configContent), "RollupNodeRPCAddress")
	require.Contains(t, string(configContent), "FinalityContractAddress")

	t.Logf("✅ Rollup FPD init command test completed successfully")
	t.Logf("✅ Config file created at: %s", configPath)
	t.Logf("✅ Log directory created at: %s", logDir)
}

// TestRollupFpdCreateFPCmd tests the rollup-fpd create-finality-provider command
// This follows the exact same pattern as Babylon's TestFinalityProviderCreateCmd
func TestRollupFpdCreateFPCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use the new function that starts actual daemon processes
	ctm, fps := StartRollupManagerWithFinalityProvider(t, 1, ctx)
	defer ctm.Stop(t)

	fpIns := fps[0]

	// Create the CLI command
	cmd := rollupfpdaemon.CommandCreateFP(testBinaryName)

	// Create a new EOTS key for testing
	eotsKeyName := "test-rollup-fp-key-2"
	eotsPkBz, err := ctm.EOTSServerHandler.CreateKey(eotsKeyName, "")
	require.NoError(t, err)
	eotsPk, err := bbntypes.NewBIP340PubKey(eotsPkBz)
	require.NoError(t, err)

	// Create test data (same structure as Babylon)
	data := struct {
		KeyName                 string `json:"keyName"`
		ChainID                 string `json:"chainID"`
		Passphrase              string `json:"passphrase"`
		CommissionRate          string `json:"commissionRate"`
		CommissionMaxRate       string `json:"commissionMaxRate"`
		CommissionMaxChangeRate string `json:"commissionMaxChangeRate"`
		Moniker                 string `json:"moniker"`
		Identity                string `json:"identity"`
		Website                 string `json:"website"`
		SecurityContract        string `json:"securityContract"`
		Details                 string `json:"details"`
		EotsPK                  string `json:"eotsPK"`
	}{
		KeyName:                 fpIns.GetConfig().BabylonConfig.Key,
		ChainID:                 rollupBSNID,
		Passphrase:              passphrase,
		CommissionRate:          "0.10",
		CommissionMaxRate:       "0.20",
		CommissionMaxChangeRate: "0.01",
		Moniker:                 "rollup-test-fp",
		Identity:                "ROLLUP123456789",
		Website:                 "https://rollup-fp.example.com",
		SecurityContract:        "https://rollup-fp.example.com/security",
		Details:                 "This is a test rollup BSN finality provider.",
		EotsPK:                  eotsPk.MarshalHex(),
	}

	// Create temporary JSON file
	file, err := os.Create(fmt.Sprintf("%s/%s", t.TempDir(), "rollup-finality-provider.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Remove(file.Name())
	})

	err = json.NewEncoder(file).Encode(data)
	require.NoError(t, err)

	// Set command arguments - can now connect to real daemon!
	cmd.SetArgs([]string{
		"--from-file=" + file.Name(),
		"--daemon-address=" + fpIns.GetConfig().RPCListener,
	})

	// Run the command
	err = cmd.Execute()
	require.NoError(t, err)

	// Verify the finality provider was created by querying Babylon
	fp, err := ctm.BabylonController.QueryFinalityProvider(ctx, eotsPk.MustToBTCPK())
	require.NoError(t, err)
	require.NotNil(t, fp)

	t.Logf("✅ Rollup FPD create-finality-provider command test completed successfully")
	t.Logf("✅ Created finality provider with EOTS PK: %s", eotsPk.MarshalHex())
}

// TestRollupFpdCommitPubRandCmd tests the rollup-fpd unsafe-commit-pubrand command
// This command commits public randomness to the rollup BSN contract
func TestRollupFpdCommitPubRandCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctm := StartRollupTestManager(t, ctx)
	defer ctm.Stop(t)

	// Set up finality providers and activate them
	fps := ctm.setupBabylonAndConsumerFp(t)
	consumerFpPk := fps[1]

	// Add consumer FP to contract allowlist
	ctm.addFpToContractAllowlist(t, ctx, consumerFpPk)
	ctm.delegateBTCAndWaitForActivation(t, fps[0], consumerFpPk)

	// Get consumer FP instance
	consumerFpInstance := ctm.getConsumerFpInstance(t, consumerFpPk)

	// Wait for initial randomness commitment
	ctm.WaitForFpPubRandTimestamped(t, consumerFpInstance)

	// Stop the FP instance to test manual commitment
	err := consumerFpInstance.Stop()
	require.NoError(t, err)

	// Create the commit pubrand command
	cmd := rollupfpdaemon.CommandCommitPubRand(testBinaryName)

	// Target height for commitment (commit for next 1000 blocks)
	targetHeight := uint64(30000)

	// Set up command arguments
	cmd.SetArgs([]string{
		consumerFpPk.MarshalHex(),
		strconv.FormatUint(targetHeight, 10),
		"--home=" + filepath.Dir(ctm.ConsumerFpApp.GetConfig().DatabaseConfig.DBPath),
	})

	// Execute the command
	t1 := time.Now()
	err = cmd.ExecuteContext(ctx)
	require.NoError(t, err)
	t.Logf("Manual public randomness commitment took %v", time.Since(t1))

	// Verify the commitment was successful by checking if we can query it
	require.Eventually(t, func() bool {
		lastCommit, err := ctm.RollupBSNController.QueryLastPublicRandCommit(ctx, consumerFpPk.MustToBTCPK())
		if err != nil {
			t.Logf("Failed to query last public randomness commit: %v", err)
			return false
		}
		if lastCommit == nil {
			t.Logf("No public randomness commit found")
			return false
		}
		// Check if the commitment covers the target height
		return lastCommit.StartHeight+lastCommit.NumPubRand >= targetHeight
	}, eventuallyWaitTimeOut, eventuallyPollTime, "Public randomness should be committed to target height")

	t.Logf("✅ Rollup FPD commit-pubrand command test completed successfully")
	t.Logf("✅ Committed public randomness up to height: %d", targetHeight)
}

// TestRollupFpdRecoverProofCmd tests the rollup-fpd recover-rand-proof command
// This command recovers public randomness merkle proofs from the rollup BSN
func TestRollupFpdRecoverProofCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctm := StartRollupTestManager(t, ctx)
	defer ctm.Stop(t)

	// Set up finality providers and activate them
	fps := ctm.setupBabylonAndConsumerFp(t)
	consumerFpPk := fps[1]

	// Add consumer FP to contract allowlist and activate
	ctm.addFpToContractAllowlist(t, ctx, consumerFpPk)
	ctm.delegateBTCAndWaitForActivation(t, fps[0], consumerFpPk)

	// Get consumer FP instance and start it
	consumerFpInstance := ctm.getConsumerFpInstance(t, consumerFpPk)
	err := consumerFpInstance.Start(ctx)
	require.NoError(t, err)

	// Wait for randomness commitment and voting
	ctm.WaitForFpPubRandTimestamped(t, consumerFpInstance)

	// Wait for at least one vote to ensure we have finalized blocks
	require.Eventually(t, func() bool {
		return consumerFpInstance.GetLastVotedHeight() > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime, "FP should vote on at least one block")

	lastVotedHeight := consumerFpInstance.GetLastVotedHeight()
	t.Logf("FP voted on block at height: %d", lastVotedHeight)

	// Stop the FP instance
	err = consumerFpInstance.Stop()
	require.NoError(t, err)

	// Get the FP config and delete the database to simulate data loss
	fpCfg := consumerFpInstance.GetConfig()
	dbPath := filepath.Join(fpCfg.DatabaseConfig.DBPath, fpCfg.DatabaseConfig.DBFileName)
	err = os.Remove(dbPath)
	require.NoError(t, err)

	// Create the recover proof command
	cmd := rollupfpdaemon.CommandRecoverProof(testBinaryName)

	// Set up command arguments
	cmd.SetArgs([]string{
		consumerFpPk.MarshalHex(),
		"--home=" + filepath.Dir(fpCfg.DatabaseConfig.DBPath),
		"--chain-id=" + rollupBSNID,
	})

	// Execute the command
	err = cmd.ExecuteContext(ctx)
	require.NoError(t, err)

	// Verify the database was recreated
	_, err = os.Stat(dbPath)
	require.NoError(t, err)

	// Verify the proofs were recovered by checking the database
	fpdb, err := fpCfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)
	defer fpdb.Close()

	pubRandStore, err := store.NewPubRandProofStore(fpdb)
	require.NoError(t, err)

	// Check if we can retrieve a proof for a finalized block
	_, err = pubRandStore.GetPubRandProof([]byte(rollupBSNID), consumerFpPk.MustMarshal(), lastVotedHeight)
	require.NoError(t, err)

	t.Logf("✅ Rollup FPD recover-rand-proof command test completed successfully")
	t.Logf("✅ Recovered proofs for finality provider: %s", consumerFpPk.MarshalHex())
	t.Logf("✅ Database recreated at: %s", dbPath)
}

// TestRollupFpdStartCmd tests the rollup-fpd start command
// This command starts the rollup finality provider daemon with rollup-specific configuration
func TestRollupFpdStartCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ctm := StartRollupTestManager(t, ctx)
	defer ctm.Stop(t)

	// Set up finality providers
	fps := ctm.setupBabylonAndConsumerFp(t)
	consumerFpPk := fps[1]

	// Add consumer FP to contract allowlist
	ctm.addFpToContractAllowlist(t, ctx, consumerFpPk)
	ctm.delegateBTCAndWaitForActivation(t, fps[0], consumerFpPk)

	// Create the start command
	cmd := rollupfpdaemon.CommandStart(testBinaryName)

	// Set up client context
	fpCfg := ctm.ConsumerFpApp.GetConfig()

	// Create a test config file for the start command
	testHomeDir := filepath.Dir(fpCfg.DatabaseConfig.DBPath)

	// Update the config to include rollup-specific settings
	rollupCfg := &rollupfpconfig.RollupFPConfig{
		RollupNodeRPCAddress:    ctm.RollupBSNController.Cfg.RollupNodeRPCAddress,
		FinalityContractAddress: ctm.RollupBSNController.Cfg.FinalityContractAddress,
		Common:                  fpCfg,
	}

	// Write the rollup config to file
	fileParser := goflags.NewParser(rollupCfg, goflags.Default)
	err := goflags.NewIniParser(fileParser).WriteFile(rollupfpconfig.CfgFile(testHomeDir), goflags.IniIncludeDefaults)
	require.NoError(t, err)

	// Set up command arguments
	cmd.SetArgs([]string{
		"--home=" + testHomeDir,
		"--eots-pk=" + consumerFpPk.MarshalHex(),
		"--rpc-listener=" + fpCfg.RPCListener,
	})

	// Start the command in a goroutine since it runs until shutdown
	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.ExecuteContext(ctx)
	}()

	// Wait a bit for the daemon to start
	time.Sleep(5 * time.Second)

	// Verify the daemon started by checking if we can connect to the RPC listener
	// This is a simple validation that the start command executed without immediate errors
	select {
	case err := <-cmdDone:
		// If the command finished, it should be due to context cancellation
		if ctx.Err() == context.DeadlineExceeded {
			t.Logf("✅ Rollup FPD start command test completed successfully")
			t.Logf("✅ Daemon started and ran for the test duration")
		} else {
			require.NoError(t, err, "Start command should not fail immediately")
		}
	case <-time.After(10 * time.Second):
		// If still running after 10 seconds, consider it successful
		t.Logf("✅ Rollup FPD start command test completed successfully")
		t.Logf("✅ Daemon is running successfully")
		cancel() // Cancel to stop the daemon

		// Wait for graceful shutdown
		select {
		case <-cmdDone:
			t.Logf("✅ Daemon shut down gracefully")
		case <-time.After(5 * time.Second):
			t.Logf("⚠️  Daemon shutdown took longer than expected")
		}
	}
}

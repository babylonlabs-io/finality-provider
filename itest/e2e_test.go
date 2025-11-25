//go:build e2e
// +build e2e

package e2etest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	"github.com/babylonlabs-io/finality-provider/testutil"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	goflags "github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"

	eotscmd "github.com/babylonlabs-io/finality-provider/eotsmanager/cmd/eotsd/daemon"
	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	cfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/types"
)

var (
	stakingTime   = uint16(1000)
	stakingAmount = int64(500000)
)

// TestFinalityProviderLifeCycle tests the whole life cycle of a finality-provider
// creation -> registration -> randomness commitment ->
// activation with BTC delegation and Covenant sig ->
// vote submission -> block finalization
// The test runs 2 finality providers connecting to
// a single EOTS manager
func TestFinalityProviderLifeCycle(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n := 2
	tm, fps := StartManagerWithFinalityProvider(t, n, ctx)
	defer tm.Stop(t)

	// check the public randomness is committed
	tm.WaitForFpPubRandTimestamped(t, fps[0])

	// send a BTC delegation
	for _, fp := range fps {
		_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fp.GetBtcPk()}, stakingTime, stakingAmount)
	}

	// check the BTC delegation is pending
	delsResp := tm.WaitForNPendingDels(t, n)
	var dels []*bstypes.BTCDelegation
	for _, delResp := range delsResp {
		del, err := ParseRespBTCDelToBTCDel(delResp)
		require.NoError(t, err)
		dels = append(dels, del)
		// send covenant sigs
		tm.InsertCovenantSigForDelegation(t, del)
	}

	// check the BTC delegation is active
	_ = tm.WaitForNActiveDels(t, n)

	// check the last voted block is finalized
	lastVotedHeight := tm.WaitForFpVoteCast(t, fps[0])

	tm.CheckBlockFinalization(t, lastVotedHeight, 1)
	t.Logf("the block at height %v is finalized", lastVotedHeight)
}

// TestDoubleSigning tests the attack scenario where the finality-provider
// sends a finality vote over a conflicting block
// in this case, the BTC private key should be extracted by Babylon
func TestDoubleSigning(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer tm.Stop(t)

	fpIns := fps[0]

	// check the public randomness is committed
	tm.WaitForFpPubRandTimestamped(t, fpIns)

	// send a BTC delegation
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.GetBtcPk()}, stakingTime, stakingAmount)

	// check the BTC delegation is pending
	delsResp := tm.WaitForNPendingDels(t, 1)
	del, err := ParseRespBTCDelToBTCDel(delsResp[0])
	require.NoError(t, err)

	// send covenant sigs
	tm.InsertCovenantSigForDelegation(t, del)

	// check the BTC delegation is active
	_ = tm.WaitForNActiveDels(t, 1)

	// check the last voted block is finalized
	lastVotedHeight := tm.WaitForFpVoteCast(t, fpIns)
	tm.CheckBlockFinalization(t, lastVotedHeight, 1)
	t.Logf("the block at height %v is finalized", lastVotedHeight)

	finalizedBlocks := tm.WaitForNFinalizedBlocks(t, 1)

	// test duplicate vote which should be ignored
	res, extractedKey, err := fpIns.TestSubmitBatchFinalitySignaturesAndExtractPrivKey([]*types.BlockInfo{finalizedBlocks[0]}, false)
	require.NoError(t, err)
	require.Nil(t, extractedKey)
	require.Empty(t, res)
	t.Logf("duplicate vote for %d is sent", finalizedBlocks[0].Height)

	// attack: manually submit a finality vote over a conflicting block
	// to trigger the extraction of finality-provider's private key
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := &types.BlockInfo{
		Height: finalizedBlocks[0].Height,
		Hash:   datagen.GenRandomByteArray(r, 32),
	}

	// confirm we have double sign protection
	_, _, err = fpIns.TestSubmitBatchFinalitySignaturesAndExtractPrivKey([]*types.BlockInfo{b}, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "double sign")

	_, extractedKey, err = fpIns.TestSubmitBatchFinalitySignaturesAndExtractPrivKey([]*types.BlockInfo{b}, false)
	require.NoError(t, err)
	require.NotNil(t, extractedKey)
	localKey := tm.EOTSServerHandler.GetFPPrivKey(t, fpIns.GetBtcPkBIP340().MustMarshal())
	require.True(t, localKey.Key.Equals(&extractedKey.Key) || localKey.Key.Negate().Equals(&extractedKey.Key))

	t.Logf("the equivocation attack is successful")
}

// TestCatchingUp tests if a fp can catch up after restarted
func TestCatchingUp(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer tm.Stop(t)

	fpIns := fps[0]

	// check the public randomness is committed
	tm.WaitForFpPubRandTimestamped(t, fpIns)

	// send a BTC delegation
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.GetBtcPk()}, stakingTime, stakingAmount)

	// check the BTC delegation is pending
	delsResp := tm.WaitForNPendingDels(t, 1)
	del, err := ParseRespBTCDelToBTCDel(delsResp[0])
	require.NoError(t, err)

	// send covenant sigs
	tm.InsertCovenantSigForDelegation(t, del)

	// check the BTC delegation is active
	_ = tm.WaitForNActiveDels(t, 1)

	// check the last voted block is finalized
	lastVotedHeight := tm.WaitForFpVoteCast(t, fpIns)
	tm.CheckBlockFinalization(t, lastVotedHeight, 1)

	t.Logf("the block at height %v is finalized", lastVotedHeight)

	var finalizedBlocks []*types.BlockInfo
	finalizedBlocks = tm.WaitForNFinalizedBlocks(t, 1)

	n := 3
	// stop the finality-provider for a few blocks then restart to trigger the fast sync
	tm.StopAndRestartFpAfterNBlocks(t, n, fpIns)

	// check there are n+1 blocks finalized
	finalizedBlocks = tm.WaitForNFinalizedBlocks(t, n+1)
	finalizedHeight := finalizedBlocks[0].Height
	t.Logf("the latest finalized block is at %v", finalizedHeight)

	// check if the fast sync works by checking if the gap is not more than 1
	currentHeaderRes, err := tm.BBNClient.QueryBestBlock()
	currentHeight := currentHeaderRes.Height
	t.Logf("the current block is at %v", currentHeight)
	require.NoError(t, err)
	require.True(t, currentHeight < finalizedHeight+uint64(n))
}

func TestFinalityProviderEditCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer tm.Stop(t)

	fpIns := fps[0]

	cmd := daemon.CommandEditFinalityDescription()

	const (
		monikerFlag          = "moniker"
		identityFlag         = "identity"
		websiteFlag          = "website"
		securityContactFlag  = "security-contact"
		detailsFlag          = "details"
		fpdDaemonAddressFlag = "daemon-address"
	)

	moniker := "test-moniker"
	website := "https://test.com"
	securityContact := "test@test.com"
	details := "Test details"
	identity := "test-identity"

	// don't try to edit commission because need to wait
	// 24hs after creation to edit the commission rate
	args := []string{
		fpIns.GetBtcPkHex(),
		"--" + fpdDaemonAddressFlag, fpIns.GetConfig().RPCListener,
		"--" + monikerFlag, moniker,
		"--" + websiteFlag, website,
		"--" + securityContactFlag, securityContact,
		"--" + detailsFlag, details,
		"--" + identityFlag, identity,
	}

	cmd.SetArgs(args)

	// Run the command
	err := cmd.Execute()
	require.NoError(t, err)

	gotFp, err := tm.BBNClient.QueryFinalityProvider(fpIns.GetBtcPk())
	require.NoError(t, err)

	require.Equal(t, gotFp.FinalityProvider.Description.Moniker, moniker)
	require.Equal(t, gotFp.FinalityProvider.Description.Website, website)
	require.Equal(t, gotFp.FinalityProvider.Description.Identity, identity)
	require.Equal(t, gotFp.FinalityProvider.Description.Details, details)
	require.Equal(t, gotFp.FinalityProvider.Description.SecurityContact, securityContact)

	moniker = "test2-moniker"
	args = []string{
		fpIns.GetBtcPkHex(),
		"--" + fpdDaemonAddressFlag, fpIns.GetConfig().RPCListener,
		"--" + monikerFlag, moniker,
	}

	cmd.SetArgs(args)

	// Run the command
	err = cmd.Execute()
	require.NoError(t, err)

	updatedFp, err := tm.BBNClient.QueryFinalityProvider(fpIns.GetBtcPk())
	require.NoError(t, err)

	updateFpDesc := updatedFp.FinalityProvider.Description
	oldDesc := gotFp.FinalityProvider.Description

	require.Equal(t, updateFpDesc.Moniker, moniker)
	require.Equal(t, updateFpDesc.Website, oldDesc.Website)
	require.Equal(t, updateFpDesc.Identity, oldDesc.Identity)
	require.Equal(t, updateFpDesc.Details, oldDesc.Details)
	require.Equal(t, updateFpDesc.SecurityContact, oldDesc.SecurityContact)
}

func TestFinalityProviderCreateCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer tm.Stop(t)

	fpIns := fps[0]

	cmd := daemon.CommandCreateFP()

	eotsKeyName := "eots-key-2"
	eotsPkBz, err := tm.EOTSServerHandler.CreateKey(eotsKeyName)
	require.NoError(t, err)
	eotsPk, err := bbntypes.NewBIP340PubKey(eotsPkBz)
	require.NoError(t, err)

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
		ChainID:                 testChainID,
		Passphrase:              passphrase,
		CommissionRate:          "0.10",
		CommissionMaxRate:       "0.20",
		CommissionMaxChangeRate: "0.01",
		Moniker:                 "some moniker",
		Identity:                "F123456789ABCDEF",
		Website:                 "https://fp.example.com",
		SecurityContract:        "https://fp.example.com/security",
		Details:                 "This is a highly secure and reliable fp.",
		EotsPK:                  eotsPk.MarshalHex(),
	}

	file, err := os.Create(fmt.Sprintf("%s/%s", t.TempDir(), "finality-provider.json"))
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(file.Name())
	})

	if err := json.NewEncoder(file).Encode(data); err != nil {
		log.Fatalf("Failed to write JSON to file: %v", err)
	}

	cmd.SetArgs([]string{
		"--from-file=" + file.Name(),
		"--daemon-address=" + fpIns.GetConfig().RPCListener,
	})

	// Run the command
	err = cmd.Execute()
	require.NoError(t, err)

	fp, err := tm.BBNClient.QueryFinalityProvider(eotsPk.MustToBTCPK())
	require.NoError(t, err)
	require.NotNil(t, fp)
}

func TestRemoveMerkleProofsCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer tm.Stop(t)

	fpIns := fps[0]

	tm.WaitForFpPubRandTimestamped(t, fps[0])
	cmd := daemon.CommandUnsafePruneMerkleProof()

	cmd.SetArgs([]string{
		fpIns.GetBtcPkHex(),
		"--daemon-address=" + fpIns.GetConfig().RPCListener,
		"--up-to-height=100",
		"--chain-id=" + testChainID,
	})

	err := cmd.Execute()
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := tm.Fps[0].GetPubRandProofStore().
			GetPubRandProof(fpIns.GetChainID(), fpIns.GetBtcPkBIP340().MustMarshal(), 99)

		return errors.Is(err, store.ErrPubRandProofNotFound)
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func TestPrintEotsCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tm := StartManager(t, ctx)
	r := rand.New(rand.NewSource(time.Now().Unix()))
	defer tm.Stop(t)

	expected := make(map[string]string)
	for i := 0; i < r.Intn(10); i++ {
		eotsKeyName := fmt.Sprintf("eots-key-%s", datagen.GenRandomHexStr(r, 4))
		ekey, err := tm.EOTSServerHandler.CreateKey(eotsKeyName)
		require.NoError(t, err)
		pk, err := schnorr.ParsePubKey(ekey)
		require.NoError(t, err)
		expected[eotsKeyName] = bbntypes.NewBIP340PubKeyFromBTCPK(pk).MarshalHex()
	}

	cancel()

	cmd := eotscmd.CommandPrintAllKeys()

	defaultConfig := eotscfg.DefaultConfigWithHomePath(tm.EOTSHomeDir)
	fileParser := goflags.NewParser(defaultConfig, goflags.Default)
	err := goflags.NewIniParser(fileParser).WriteFile(eotscfg.CfgFile(tm.EOTSHomeDir), goflags.IniIncludeDefaults)
	require.NoError(t, err)

	cmd.SetArgs([]string{
		"--home=" + tm.EOTSHomeDir,
	})

	var outputBuffer bytes.Buffer
	cmd.SetOut(&outputBuffer)
	cmd.SetErr(&outputBuffer)

	err = cmd.Execute()
	require.NoError(t, err)

	output := outputBuffer.String()
	t.Logf("Captured output: %s", output)

	for keyName, eotsPK := range expected {
		require.Contains(t, output, keyName)
		require.Contains(t, output, eotsPK)
	}
}

func TestRecoverRandProofCmd(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer tm.Stop(t)

	fpIns := fps[0]

	// check the public randomness is committed
	tm.WaitForFpPubRandTimestamped(t, fpIns)

	// send a BTC delegation
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.GetBtcPk()}, stakingTime, stakingAmount)

	// check the BTC delegation is pending
	delsResp := tm.WaitForNPendingDels(t, 1)
	del, err := ParseRespBTCDelToBTCDel(delsResp[0])
	require.NoError(t, err)

	// send covenant sigs
	tm.InsertCovenantSigForDelegation(t, del)

	// check the BTC delegation is active
	_ = tm.WaitForNActiveDels(t, 1)

	// check the last voted block is finalized
	lastVotedHeight := tm.WaitForFpVoteCast(t, fpIns)
	tm.CheckBlockFinalization(t, lastVotedHeight, 1)
	t.Logf("the block at height %v is finalized", lastVotedHeight)

	finalizedBlock := tm.WaitForNFinalizedBlocks(t, 1)
	fpCfg := fpIns.GetConfig()

	// delete the db file
	dbPath := filepath.Join(fpCfg.DatabaseConfig.DBPath, fpCfg.DatabaseConfig.DBFileName)
	err = os.Remove(dbPath)
	require.NoError(t, err)

	fpCfg.EOTSManagerAddress = tm.EOTSServerHandler.Config().RPCListener
	fpHomePath := filepath.Dir(fpCfg.DatabaseConfig.DBPath)
	fileParser := goflags.NewParser(fpCfg, goflags.Default)
	err = goflags.NewIniParser(fileParser).WriteFile(cfg.CfgFile(fpHomePath), goflags.IniIncludeDefaults)
	require.NoError(t, err)

	// run the cmd
	cmd := daemon.CommandRecoverProof()
	cmd.SetArgs([]string{
		fpIns.GetBtcPkHex(),
		"--home=" + fpHomePath,
		"--chain-id=" + testChainID,
	})
	err = cmd.Execute()
	require.NoError(t, err)

	// assert db exists
	_, err = os.Stat(dbPath)
	require.NoError(t, err)

	fpdb, err := fpCfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)

	pubRandStore, err := store.NewPubRandProofStore(fpdb)
	require.NoError(t, err)
	_, err = pubRandStore.GetPubRandProof([]byte(testChainID), fpIns.GetBtcPkBIP340().MustMarshal(), finalizedBlock[0].Height)
	require.NoError(t, err)
}

func TestEotsdRollbackCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testDir := t.TempDir()

	eotsHomeDir := filepath.Join(testDir, "eots-home")
	eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
	eotsCfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eotsCfg.Metrics.Port = testutil.AllocateUniquePort(t)

	eh := NewEOTSServerHandler(t, eotsCfg, eotsHomeDir)
	eh.Start(ctx)

	eotsCli, err := client.NewEOTSManagerGRpcClient(eotsCfg.RPCListener, "")
	require.NoError(t, err)

	key, err := eh.CreateKey("eots-key-1")
	require.NoError(t, err)

	err = eotsCli.Ping()
	require.NoError(t, err)

	const numRecords = 100
	const rollbackHeight = 10

	for i := 0; i < numRecords; i++ {
		_, err = eotsCli.SignEOTS(
			key,
			[]byte(testChainID),
			[]byte("test"),
			uint64(i),
		)
		require.NoError(t, err)
	}

	cmd := eotscmd.NewSignStoreRollbackCmd()
	require.NoError(t, err)

	err = eh.Stop()
	require.NoError(t, err)

	eotsPK, err := bbntypes.NewBIP340PubKey(key)
	require.NoError(t, err)

	cmd.SetArgs([]string{
		"--home=" + eotsHomeDir,
		"--rollback-until-height=" + strconv.Itoa(rollbackHeight),
		"--chain-id=" + testChainID,
		"--eots-pk=" + eotsPK.MarshalHex(),
	})

	err = cmd.Execute()
	require.NoError(t, err)

	eotsCfg.Metrics.Port = testutil.AllocateUniquePort(t)
	eotsCfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eh = NewEOTSServerHandler(t, eotsCfg, eotsHomeDir)
	eh.Start(ctx)

	for i := rollbackHeight; i <= numRecords; i++ {
		exists, err := eh.IsRecordInDb(key, []byte(testChainID), uint64(i))
		require.NoError(t, err)
		require.False(t, exists)
	}

	for i := 0; i < rollbackHeight; i++ {
		exists, err := eh.IsRecordInDb(key, []byte(testChainID), uint64(i))
		require.NoError(t, err)
		require.True(t, exists)
	}
}

// TestBatchSubmissionWithDuplicates tests that when submitting a batch
// of finality signatures where some are duplicates, the duplicates are
// removed and the batch is retried with only the valid signatures
func TestBatchSubmissionWithDuplicates(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer tm.Stop(t)

	fpIns := fps[0]

	// 1. Setup: get FP activated
	tm.WaitForFpPubRandTimestamped(t, fpIns)
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.GetBtcPk()}, stakingTime, stakingAmount)
	delsResp := tm.WaitForNPendingDels(t, 1)
	del, err := ParseRespBTCDelToBTCDel(delsResp[0])
	require.NoError(t, err)
	tm.InsertCovenantSigForDelegation(t, del)
	_ = tm.WaitForNActiveDels(t, 1)

	// 2. Let FP vote on a few blocks, then stop it
	lastVotedHeight := tm.WaitForFpVoteCast(t, fpIns)
	t.Logf("FP cast first vote at height %d", lastVotedHeight)

	// Wait a bit for the FP to vote on more blocks
	time.Sleep(3 * time.Second)

	// Stop the FP to prevent more voting
	err = fpIns.Stop()
	require.NoError(t, err)
	t.Logf("Stopped FP")

	// Wait for pending operations to complete
	time.Sleep(2 * time.Second)

	// 3. Query the chain to find which blocks actually have votes from this FP
	// We'll scan recent blocks to find which ones have votes
	currentHeight, err := tm.BBNClient.QueryBestBlock()
	require.NoError(t, err)

	votedBlocks := make([]*types.BlockInfo, 0)
	unvotedBlocks := make([]*types.BlockInfo, 0)

	// Scan blocks starting from first voted height
	for h := lastVotedHeight; h <= currentHeight.Height; h++ {
		votes, err := tm.BBNClient.QueryVotesAtHeight(h)
		require.NoError(t, err)

		block, err := tm.BBNClient.QueryBlock(h)
		require.NoError(t, err)

		if len(votes) > 0 {
			votedBlocks = append(votedBlocks, block)
			t.Logf("Block %d has %d vote(s)", h, len(votes))
		} else {
			unvotedBlocks = append(unvotedBlocks, block)
			t.Logf("Block %d has no votes", h)
		}

		// We need at least 2 voted blocks and 2 unvoted blocks
		if len(votedBlocks) >= 2 && len(unvotedBlocks) >= 2 {
			break
		}
	}

	// If we don't have enough unvoted blocks, wait for more blocks to be produced
	for len(unvotedBlocks) < 2 {
		t.Logf("Waiting for more blocks to be produced (need %d more unvoted blocks)", 2-len(unvotedBlocks))
		time.Sleep(1 * time.Second)

		currentHeight, err = tm.BBNClient.QueryBestBlock()
		require.NoError(t, err)

		// Check new blocks
		startHeight := uint64(1)
		if len(votedBlocks) > 0 {
			startHeight = votedBlocks[len(votedBlocks)-1].Height + 1
		}
		if len(unvotedBlocks) > 0 {
			startHeight = unvotedBlocks[len(unvotedBlocks)-1].Height + 1
		}

		for h := startHeight; h <= currentHeight.Height; h++ {
			votes, err := tm.BBNClient.QueryVotesAtHeight(h)
			require.NoError(t, err)

			if len(votes) == 0 {
				block, err := tm.BBNClient.QueryBlock(h)
				require.NoError(t, err)
				unvotedBlocks = append(unvotedBlocks, block)
				t.Logf("Found unvoted block at height %d", h)

				if len(unvotedBlocks) >= 2 {
					break
				}
			}
		}
	}

	require.GreaterOrEqual(t, len(votedBlocks), 2, "need at least 2 voted blocks")
	require.GreaterOrEqual(t, len(unvotedBlocks), 2, "need at least 2 unvoted blocks")

	t.Logf("Found %d voted blocks and %d unvoted blocks", len(votedBlocks), len(unvotedBlocks))
	t.Logf("Will create mixed batch with: duplicate heights [%d, %d], new heights [%d, %d]",
		votedBlocks[0].Height, votedBlocks[1].Height,
		unvotedBlocks[0].Height, unvotedBlocks[1].Height)

	// 4. Submit a mixed batch with duplicates and new votes
	// This tests the reliablySendMsgsResendingOnMsgErr function's ability to handle
	// a batch where some messages fail with expected errors (duplicates)
	mixedBatch := []*types.BlockInfo{
		votedBlocks[0],   // duplicate
		votedBlocks[1],   // duplicate
		unvotedBlocks[0], // new vote
		unvotedBlocks[1], // new vote
	}

	t.Logf("Submitting mixed batch with duplicate heights [%d, %d] and new heights [%d, %d]",
		votedBlocks[0].Height, votedBlocks[1].Height,
		unvotedBlocks[0].Height, unvotedBlocks[1].Height)

	// Use the batch submission method with useSafeEOTSFunc=false to allow duplicates
	res, _, err := fpIns.TestSubmitBatchFinalitySignaturesAndExtractPrivKey(mixedBatch, false)
	require.NoError(t, err)
	require.NotNil(t, res, "batch submission should succeed")
	require.NotEmpty(t, res.TxHash, "batch submission should return a tx hash")

	t.Logf("Successfully submitted mixed batch, tx_hash: %s", res.TxHash)

	// 5. Verify that votes exist for the new blocks
	// This confirms the submissions were successful
	require.Eventually(t, func() bool {
		votes, err := tm.BBNClient.QueryVotesAtHeight(unvotedBlocks[0].Height)
		if err != nil {
			t.Logf("Error querying votes for height %d: %v", unvotedBlocks[0].Height, err)
			return false
		}
		if len(votes) > 0 {
			t.Logf("✅ Found %d vote(s) for new block at height %d", len(votes), unvotedBlocks[0].Height)
			return true
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		votes, err := tm.BBNClient.QueryVotesAtHeight(unvotedBlocks[1].Height)
		if err != nil {
			t.Logf("Error querying votes for height %d: %v", unvotedBlocks[1].Height, err)
			return false
		}
		if len(votes) > 0 {
			t.Logf("✅ Found %d vote(s) for new block at height %d", len(votes), unvotedBlocks[1].Height)
			return true
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("✅ Batch submission with duplicates test passed: " +
		"duplicates were handled gracefully and new votes were submitted successfully")
}

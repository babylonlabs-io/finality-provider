//go:build e2e_babylon

package e2etest_babylon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"

	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/testutil"

	"github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/v4/types"
	eotscmd "github.com/babylonlabs-io/finality-provider/eotsmanager/cmd/eotsd/daemon"
	commoncmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/common"
	"github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	cfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	goflags "github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"
)

const (
	BinaryName = "fpd"
)

// TestFinalityProviderLifeCycle tests the whole life cycle of a finality-provider
// creation -> registration -> randomness commitment ->
// activation with BTC delegation and Covenant sig ->
// vote submission -> block finalization
func TestFinalityProviderLifeCycle(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	n := 2
	tm, fps := StartManagerWithFinalityProvider(t, n, ctx)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	// check the public randomness is committed
	tm.WaitForFpPubRandTimestamped(t, fps[0])

	// send a BTC delegation
	for _, fp := range fps {
		_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fp.GetBtcPk()}, e2eutils.StakingTime, e2eutils.StakingAmount)
	}

	// check the BTC delegation is pending
	delsResp := tm.WaitForNPendingDels(t, n)
	for _, delResp := range delsResp {
		del, err := e2eutils.ParseRespBTCDelToBTCDel(delResp)
		require.NoError(t, err)
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

// TestSkippingDoubleSignError tests the scenario where the finality-provider
// should skip the block when encountering a double sign request from the
// eots manager
func TestSkippingDoubleSignError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	fpIns := fps[0]

	// check the public randomness is committed
	tm.WaitForFpPubRandTimestamped(t, fpIns)

	// send a BTC delegation
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.GetBtcPk()}, e2eutils.StakingTime, e2eutils.StakingAmount)

	// check the BTC delegation is pending
	delsResp := tm.WaitForNPendingDels(t, 1)
	del, err := e2eutils.ParseRespBTCDelToBTCDel(delsResp[0])
	require.NoError(t, err)

	// send covenant sigs
	tm.InsertCovenantSigForDelegation(t, del)

	// check the BTC delegation is active
	_ = tm.WaitForNActiveDels(t, 1)

	_ = tm.WaitForFpVoteCast(t, fpIns)

	// stop the fp and manually submits a finality sig for a future height
	err = fpIns.Stop()
	require.NoError(t, err)
	currentHeight := tm.WaitForNBlocks(t, 1)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := types.NewBlockInfo(currentHeight, datagen.GenRandomByteArray(r, 32), false)
	t.Logf("manually sending a vote for height %d", currentHeight)

	_, _, err = fpIns.NewTestHelper().SubmitFinalitySignatureAndExtractPrivKey(ctx, b, true)
	require.NoError(t, err)

	// restart the fp to see if it will skip sending the height
	err = fpIns.Start(ctx)
	require.NoError(t, err)

	// assert that the fp voting continues
	tm.WaitForFpVoteCastAtHeight(t, fpIns, currentHeight+1)
}

// TestDoubleSigning tests the attack scenario where the finality-provider
// sends a finality vote over a conflicting block
// in this case, the BTC private key should be extracted by Babylon
func TestDoubleSigning(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	fpIns := fps[0]

	// check the public randomness is committed
	tm.WaitForFpPubRandTimestamped(t, fpIns)

	// send a BTC delegation
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.GetBtcPk()}, e2eutils.StakingTime, e2eutils.StakingAmount)

	// check the BTC delegation is pending
	delsResp := tm.WaitForNPendingDels(t, 1)
	del, err := e2eutils.ParseRespBTCDelToBTCDel(delsResp[0])
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

	fpTestHelper := fpIns.NewTestHelper()

	// test duplicate vote which should be ignored
	res, extractedKey, err := fpTestHelper.SubmitFinalitySignatureAndExtractPrivKey(ctx, finalizedBlock, false)
	require.NoError(t, err)
	require.Nil(t, extractedKey)
	require.Empty(t, res)
	t.Logf("duplicate vote for %d is sent", finalizedBlock.GetHeight())

	// attack: manually submit a finality vote over a conflicting block
	// to trigger the extraction of finality-provider's private key
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := types.NewBlockInfo(finalizedBlock.GetHeight(), datagen.GenRandomByteArray(r, 32), false)

	// confirm we have double sign protection
	_, _, err = fpTestHelper.SubmitFinalitySignatureAndExtractPrivKey(ctx, b, true)
	require.Contains(t, err.Error(), "FailedPrecondition")

	_, extractedKey, err = fpTestHelper.SubmitFinalitySignatureAndExtractPrivKey(ctx, b, false)
	require.NoError(t, err)
	require.NotNil(t, extractedKey)
	localKey := tm.EOTSServerHandler.GetFPPrivKey(t, fpIns.GetBtcPkBIP340().MustMarshal())
	require.True(t, localKey.Key.Equals(&extractedKey.Key) || localKey.Key.Negate().Equals(&extractedKey.Key))

	t.Logf("the equivocation attack is successful")
}

// TestCatchingUp tests if a fp can catch up after restarted
func TestCatchingUp(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	fpIns := fps[0]

	// check the public randomness is committed
	tm.WaitForFpPubRandTimestamped(t, fpIns)

	// send a BTC delegation
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.GetBtcPk()}, e2eutils.StakingTime, e2eutils.StakingAmount)

	// check the BTC delegation is pending
	delsResp := tm.WaitForNPendingDels(t, 1)
	del, err := e2eutils.ParseRespBTCDelToBTCDel(delsResp[0])
	require.NoError(t, err)

	// send covenant sigs
	tm.InsertCovenantSigForDelegation(t, del)

	// check the BTC delegation is active
	_ = tm.WaitForNActiveDels(t, 1)

	// check the last voted block is finalized
	lastVotedHeight := tm.WaitForFpVoteCast(t, fpIns)
	tm.CheckBlockFinalization(t, lastVotedHeight, 1)

	t.Logf("the block at height %v is finalized", lastVotedHeight)

	tm.WaitForNFinalizedBlocks(t, 1)

	var n uint = 3
	// stop the finality-provider for a few blocks then restart to trigger the fast sync
	tm.StopAndRestartFpAfterNBlocks(ctx, t, int(n), fpIns)

	// check there are n+1 blocks finalized
	finalizedBlock := tm.WaitForNFinalizedBlocks(t, n+1)
	t.Logf("the latest finalized block is at %v", finalizedBlock.GetHeight())

	// check if the fast sync works by checking if the gap is not more than 1
	currentBlock, err := tm.BBNConsumerClient.QueryLatestBlock(ctx)
	require.NoError(t, err)
	currentHeight := currentBlock.GetHeight()
	t.Logf("the current block is at %v", currentHeight)
	require.NoError(t, err)
	require.True(t, currentHeight < finalizedBlock.GetHeight()+uint64(n))
}

func TestFinalityProviderEditCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	fpIns := fps[0]

	cmd := commoncmd.CommandEditFinalityDescription(BinaryName)

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

	gotFp, err := tm.BabylonController.QueryFinalityProvider(ctx, fpIns.GetBtcPk())
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

	updatedFp, err := tm.BabylonController.QueryFinalityProvider(ctx, fpIns.GetBtcPk())
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
	// todo(lazar955): breaking change on bbn does not allow to create multiple fps with the same key, skipping for now
	t.Skip()
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	fpIns := fps[0]

	cmd := daemon.CommandCreateFP(BinaryName)

	eotsKeyName := "eots-key-2"
	eotsPkBz, err := tm.EOTSServerHandler.CreateKey(eotsKeyName, "")
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
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Remove(file.Name())
	})

	err = json.NewEncoder(file).Encode(data)
	require.NoError(t, err)

	cmd.SetArgs([]string{
		"--from-file=" + file.Name(),
		"--daemon-address=" + fpIns.GetConfig().RPCListener,
	})

	// Run the command
	err = cmd.Execute()
	require.NoError(t, err)

	fp, err := tm.BabylonController.QueryFinalityProvider(ctx, eotsPk.MustToBTCPK())
	require.NoError(t, err)
	require.NotNil(t, fp)
}

func TestRemoveMerkleProofsCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	fpIns := fps[0]

	tm.WaitForFpPubRandTimestamped(t, fpIns)
	cmd := commoncmd.CommandUnsafePruneMerkleProof(BinaryName)

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
	ctx, cancel := context.WithCancel(t.Context())
	tm := StartManager(t, ctx, "", "")
	r := rand.New(rand.NewSource(time.Now().Unix()))
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	expected := make(map[string]string)
	for i := 0; i < r.Intn(10); i++ {
		eotsKeyName := fmt.Sprintf("eots-key-%s", datagen.GenRandomHexStr(r, 4))
		ekey, err := tm.EOTSServerHandler.CreateKey(eotsKeyName, "")
		require.NoError(t, err)
		pk, err := schnorr.ParsePubKey(ekey)
		require.NoError(t, err)
		expected[eotsKeyName] = bbntypes.NewBIP340PubKeyFromBTCPK(pk).MarshalHex()
	}

	cancel()

	cmd := eotscmd.NewKeysCmd()
	cmd.SetArgs([]string{
		"list",
		"--home=" + tm.EOTSHomeDir,
		"--keyring-backend=test",
	})

	var outputBuffer bytes.Buffer
	cmd.SetOut(&outputBuffer)
	cmd.SetErr(&outputBuffer)

	err := cmd.Execute()
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
	ctx, cancel := context.WithCancel(t.Context())
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	fpIns := fps[0]

	// check the public randomness is committed
	tm.WaitForFpPubRandTimestamped(t, fpIns)

	// send a BTC delegation
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.GetBtcPk()}, e2eutils.StakingTime, e2eutils.StakingAmount)

	// check the BTC delegation is pending
	delsResp := tm.WaitForNPendingDels(t, 1)
	del, err := e2eutils.ParseRespBTCDelToBTCDel(delsResp[0])
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
	cmd := daemon.CommandRecoverProof(BinaryName)
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
	_, err = pubRandStore.GetPubRandProof([]byte(testChainID), fpIns.GetBtcPkBIP340().MustMarshal(), finalizedBlock.GetHeight())
	require.NoError(t, err)
}

func TestSigHeightOutdated(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	fpIns := fps[0]

	tm.WaitForFpPubRandTimestamped(t, fpIns)

	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.GetBtcPk()}, e2eutils.StakingTime, e2eutils.StakingAmount)

	delsResp := tm.WaitForNPendingDels(t, 1)
	del, err := e2eutils.ParseRespBTCDelToBTCDel(delsResp[0])
	require.NoError(t, err)

	tm.InsertCovenantSigForDelegation(t, del)

	_ = tm.WaitForNActiveDels(t, 1)

	lastVotedHeight := tm.WaitForFpVoteCast(t, fpIns)
	tm.CheckBlockFinalization(t, lastVotedHeight, 1)
	t.Logf("the block at height %v is finalized", lastVotedHeight)

	finalizedBlock := tm.WaitForNFinalizedBlocks(t, 1)

	res, extractedKey, err := fpIns.NewTestHelper().SubmitFinalitySignatureAndExtractPrivKey(ctx, finalizedBlock, false)

	require.NoError(t, err)
	require.Nil(t, extractedKey)
	require.Empty(t, res)

	t.Logf("Finality signature for already finalized block at height %d was properly handled", finalizedBlock.GetHeight())
}

func TestEotsdRollbackCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	testDir := t.TempDir()

	eotsHomeDir := filepath.Join(testDir, "eots-home")
	eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
	eotsCfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eotsCfg.Metrics.Port = testutil.AllocateUniquePort(t)

	eh := e2eutils.NewEOTSServerHandler(t, eotsCfg, eotsHomeDir)
	eh.Start(ctx)

	eotsCli := NewEOTSManagerGrpcClientWithRetry(t, eotsCfg)

	key, err := eh.CreateKey("eots-key-1", "")
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
	eh = e2eutils.NewEOTSServerHandler(t, eotsCfg, eotsHomeDir)
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

func TestEotsdBackupCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	testDir := t.TempDir()

	eotsHomeDir := filepath.Join(testDir, "eots-home")
	eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
	eotsCfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eotsCfg.Metrics.Port = testutil.AllocateUniquePort(t)

	eh := e2eutils.NewEOTSServerHandler(t, eotsCfg, eotsHomeDir)
	eh.Start(ctx)

	eotsCli := NewEOTSManagerGrpcClientWithRetry(t, eotsCfg)

	key, err := eh.CreateKey("eots-key-1", "")
	require.NoError(t, err)

	err = eotsCli.Ping()
	require.NoError(t, err)

	cmd := eotscmd.NewBackupCmd()
	require.NoError(t, err)

	backupHome := t.TempDir()
	backupPath := fmt.Sprintf("%s/data", backupHome)
	dbPath := fmt.Sprintf("%s/data/eots.db", eotsHomeDir)

	cmd.SetArgs([]string{
		"--db-path=" + dbPath,
		"--rpc-client=" + eotsCfg.RPCListener,
		"--backup-dir=" + backupPath,
	})

	const numRecords = 10
	for i := 0; i < numRecords; i++ {
		_, err = eotsCli.SignEOTS(
			key,
			[]byte(testChainID),
			[]byte("test"),
			uint64(i),
		)
		require.NoError(t, err)
	}

	var outputBuffer bytes.Buffer
	cmd.SetOut(&outputBuffer)
	cmd.SetErr(&outputBuffer)

	err = cmd.Execute()
	require.NoError(t, err)

	// confirm the records are in the original db
	for i := 0; i < numRecords; i++ {
		exists, err := eh.IsRecordInDb(key, []byte(testChainID), uint64(i))
		require.NoError(t, err)
		require.True(t, exists)
	}

	output := outputBuffer.String()
	t.Logf("Captured output: %s", output)

	splitOutput := strings.Split(output, ":")
	if len(splitOutput) != 2 {
		t.Fatalf("Invalid output format: %s", output)
	}

	bkpDBPath := strings.TrimSpace(splitOutput[1])
	bkpDBPathSplit := strings.Split(bkpDBPath, "/")
	bkpDBName := bkpDBPathSplit[len(bkpDBPathSplit)-1]

	// initialize a new eotsd instance with the backup db
	eotsCfgBkp := eotscfg.DefaultConfigWithHomePath(backupHome)
	eotsCfgBkp.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eotsCfgBkp.Metrics.Port = testutil.AllocateUniquePort(t)
	eotsCfgBkp.DatabaseConfig.DBPath = backupPath
	eotsCfgBkp.DatabaseConfig.DBFileName = bkpDBName
	ehBkp := e2eutils.NewEOTSServerHandler(t, eotsCfgBkp, backupHome)
	ehBkp.Start(ctx)

	// confirm the records are in the backup db
	for i := 0; i < numRecords; i++ {
		exists, err := ehBkp.IsRecordInDb(key, []byte(testChainID), uint64(i))
		require.NoError(t, err)
		require.True(t, exists)
	}
}

// TestEotsdUnlockCmd tests the EOTS manager unlock command, demonstrating file backend keyring
func TestEotsdUnlockCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	testDir := t.TempDir()

	eotsHomeDir := filepath.Join(testDir, "eots-home")
	eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHomeDir)
	eotsCfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eotsCfg.Metrics.Port = testutil.AllocateUniquePort(t)
	eotsCfg.KeyringBackend = keyring.BackendFile
	eotsCfg.HMACKey = "some-hmac-key"

	// antipattern to set env in parallel tests but we only do it here
	err := os.Setenv("HMAC_KEY", eotsCfg.HMACKey)
	require.NoError(t, err)

	eh := e2eutils.NewEOTSServerHandler(t, eotsCfg, eotsHomeDir)
	eh.Start(ctx)

	eotsCli := NewEOTSManagerGrpcClientWithRetry(t, eotsCfg)

	const passphrase = "test-passphrase"
	key, err := eh.CreateKey("eots-key-1", passphrase)
	require.NoError(t, err)

	err = eotsCli.Ping()
	require.NoError(t, err)

	// stop the eotsd to simulate a restart otherwise keyring password will already be set in the file keyring
	// (set during the creation of the key)
	eh.Stop()
	eotsCfg.Metrics.Port = testutil.AllocateUniquePort(t)
	eotsCfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", testutil.AllocateUniquePort(t))
	eh = e2eutils.NewEOTSServerHandler(t, eotsCfg, eotsHomeDir)
	eh.Start(ctx)
	eotsCli = NewEOTSManagerGrpcClientWithRetry(t, eotsCfg)

	cmd := eotscmd.NewUnlockKeyringCmd()
	require.NoError(t, err)

	eotsPK, err := bbntypes.NewBIP340PubKey(key)
	require.NoError(t, err)

	const wrongPassphrase = "wrong-passphrase"
	eotscmd.UnlockCmdPasswordReader = func(_ *cobra.Command) (string, error) {
		return wrongPassphrase, nil
	}
	cmd.SetArgs([]string{
		"--eots-pk=" + eotsPK.MarshalHex(),
		"--rpc-client=" + eotsCfg.RPCListener,
	})
	err = cmd.Execute()
	require.Error(t, err)

	cmd.SetArgs([]string{
		"--eots-pk=" + eotsPK.MarshalHex(),
		"--rpc-client=" + eotsCfg.RPCListener,
	})

	eotscmd.UnlockCmdPasswordReader = func(_ *cobra.Command) (string, error) {
		return passphrase, nil
	}

	err = cmd.Execute()
	require.NoError(t, err)

	const numRecords = 10

	for i := 0; i < numRecords; i++ {
		_, err = eotsCli.SignEOTS(
			key,
			[]byte(testChainID),
			[]byte("test"),
			uint64(i),
		)
		require.NoError(t, err)
	}

	for i := 0; i < numRecords; i++ {
		exists, err := eh.IsRecordInDb(key, []byte(testChainID), uint64(i))
		require.NoError(t, err)
		require.True(t, exists)
	}
}

func TestUnsafeCommitPubRandCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	fpIns := fps[0]

	// check the public randomness is committed
	tm.WaitForFpPubRandTimestamped(t, fpIns)

	// send a BTC delegation
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.GetBtcPk()}, e2eutils.StakingTime, e2eutils.StakingAmount)

	// check the BTC delegation is pending
	delsResp := tm.WaitForNPendingDels(t, 1)
	del, err := e2eutils.ParseRespBTCDelToBTCDel(delsResp[0])
	require.NoError(t, err)

	// send covenant sigs
	tm.InsertCovenantSigForDelegation(t, del)

	// check the BTC delegation is active
	_ = tm.WaitForNActiveDels(t, 1)

	// check the last voted block is finalized
	lastVotedHeight := tm.WaitForFpVoteCast(t, fpIns)
	tm.CheckBlockFinalization(t, lastVotedHeight, 1)
	t.Logf("the block at height %v is finalized", lastVotedHeight)

	_ = tm.WaitForNFinalizedBlocks(t, 1)
	fpCfg := fpIns.GetConfig()

	fpCfg.EOTSManagerAddress = tm.EOTSServerHandler.Config().RPCListener
	fpHomePath := filepath.Dir(fpCfg.DatabaseConfig.DBPath)
	fileParser := goflags.NewParser(fpCfg, goflags.Default)
	err = goflags.NewIniParser(fileParser).WriteFile(cfg.CfgFile(fpHomePath), goflags.IniIncludeDefaults)
	require.NoError(t, err)

	err = fpIns.Shutdown()
	require.NoError(t, err)

	// run the cmd
	cmd := daemon.CommandCommitPubRand(BinaryName)
	cmd.SetArgs([]string{
		fpIns.GetBtcPkBIP340().MarshalHex(),
		"20000",
		"--home=" + fpHomePath,
	})
	t1 := time.Now()
	err = cmd.Execute()
	require.NoError(t, err)
	t.Logf("Committing public randomness took %v", time.Since(t1))
}

func TestFpdBackupCmd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	fpIns := fps[0]

	// check the public randomness is committed
	tm.WaitForFpPubRandTimestamped(t, fpIns)
	cmd := commoncmd.NewBackupCmd("fpd")

	backupHome := t.TempDir()
	backupPath := fmt.Sprintf("%s/data", backupHome)

	cmd.SetArgs([]string{
		"--db-path=" + fpIns.GetConfig().DatabaseConfig.DBPath,
		"--daemon-address=" + fpIns.GetConfig().RPCListener,
		"--backup-dir=" + backupPath,
	})

	var outputBuffer bytes.Buffer
	cmd.SetOut(&outputBuffer)
	cmd.SetErr(&outputBuffer)

	err := cmd.Execute()
	require.NoError(t, err)

	output := outputBuffer.String()
	t.Logf("Captured output: %s", output)

	splitOutput := strings.Split(output, ":")
	if len(splitOutput) != 2 {
		t.Fatalf("Invalid output format: %s", output)
	}

	bkpDBPath := strings.TrimSpace(splitOutput[1])
	bkpDBPathSplit := strings.Split(bkpDBPath, "/")
	bkpDBName := bkpDBPathSplit[len(bkpDBPathSplit)-1]

	fpIns.GetConfig().DatabaseConfig.DBPath = backupPath
	fpIns.GetConfig().DatabaseConfig.DBFileName = bkpDBName
	fpdb, err := fpIns.GetConfig().DatabaseConfig.GetDBBackend()
	require.NoError(t, err)

	fpStore, err := store.NewFinalityProviderStore(fpdb)
	require.NoError(t, err)

	storeFp, err := fpStore.GetFinalityProvider(fpIns.GetBtcPk())
	require.NoError(t, err)
	require.NotNil(t, storeFp)
}

// TestBatchSubmissionWithDuplicates tests that when submitting a batch
// of finality signatures where some are duplicates, the duplicates are
// removed and the batch is retried with only the valid signatures
func TestBatchSubmissionWithDuplicates(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tm, fps := StartManagerWithFinalityProvider(t, 1, ctx)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	fpIns := fps[0]

	// 1. Setup: get FP activated
	tm.WaitForFpPubRandTimestamped(t, fpIns)
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.GetBtcPk()}, e2eutils.StakingTime, e2eutils.StakingAmount)
	delsResp := tm.WaitForNPendingDels(t, 1)
	del, err := e2eutils.ParseRespBTCDelToBTCDel(delsResp[0])
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
	currentHeight, err := tm.BBNConsumerClient.QueryLatestBlock(t.Context())
	require.NoError(t, err)

	votedBlocks := make([]types.BlockDescription, 0)
	unvotedBlocks := make([]types.BlockDescription, 0)

	// Scan blocks starting from first voted height
	for h := lastVotedHeight; h <= currentHeight.GetHeight(); h++ {
		votes, err := tm.BBNConsumerClient.Client().VotesAtHeight(h)
		require.NoError(t, err)

		block, err := tm.BBNConsumerClient.QueryBlock(t.Context(), h)
		require.NoError(t, err)

		if len(votes.BtcPks) > 0 {
			votedBlocks = append(votedBlocks, block)
			t.Logf("Block %d has %d vote(s)", h, len(votes.BtcPks))
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

		currentHeight, err = tm.BBNConsumerClient.QueryLatestBlock(t.Context())
		require.NoError(t, err)

		// Check new blocks
		startHeight := uint64(1)
		if len(votedBlocks) > 0 {
			startHeight = votedBlocks[len(votedBlocks)-1].GetHeight() + 1
		}
		if len(unvotedBlocks) > 0 {
			startHeight = unvotedBlocks[len(unvotedBlocks)-1].GetHeight() + 1
		}

		for h := startHeight; h <= currentHeight.GetHeight(); h++ {
			votes, err := tm.BBNConsumerClient.Client().VotesAtHeight(h)
			require.NoError(t, err)

			if len(votes.BtcPks) == 0 {
				block, err := tm.BBNConsumerClient.QueryBlock(t.Context(), h)
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
		votedBlocks[0].GetHeight(), votedBlocks[1].GetHeight(),
		unvotedBlocks[0].GetHeight(), unvotedBlocks[1].GetHeight())

	// 4. Submit a mixed batch with duplicates and new votes
	// This tests the reliablySendMsgsResendingOnMsgErr function's ability to handle
	// a batch where some messages fail with expected errors (duplicates)
	mixedBatch := []types.BlockDescription{
		votedBlocks[0],   // duplicate
		votedBlocks[1],   // duplicate
		unvotedBlocks[0], // new vote
		unvotedBlocks[1], // new vote
	}

	t.Logf("Submitting mixed batch with duplicate heights [%d, %d] and new heights [%d, %d]",
		votedBlocks[0].GetHeight(), votedBlocks[1].GetHeight(),
		unvotedBlocks[0].GetHeight(), unvotedBlocks[1].GetHeight())

	// Use the batch submission method with useSafeEOTSFunc=false to allow duplicates
	res, _, err := fpIns.NewTestHelper().SubmitBatchFinalitySignaturesAndExtractPrivKey(ctx, mixedBatch, false)
	require.NoError(t, err)
	require.NotNil(t, res, "batch submission should succeed")
	require.NotEmpty(t, res.TxHash, "batch submission should return a tx hash")

	t.Logf("Successfully submitted mixed batch, tx_hash: %s", res.TxHash)

	// 5. Verify that votes exist for the new blocks
	// This confirms the submissions were successful
	require.Eventually(t, func() bool {
		votes, err := tm.BBNConsumerClient.Client().VotesAtHeight(unvotedBlocks[0].GetHeight())
		if err != nil {
			t.Logf("Error querying votes for height %d: %v", unvotedBlocks[0].GetHeight(), err)
			return false
		}
		if len(votes.BtcPks) > 0 {
			t.Logf("✅ Found %d vote(s) for new block at height %d", len(votes.BtcPks), unvotedBlocks[0].GetHeight())
			return true
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		votes, err := tm.BBNConsumerClient.Client().VotesAtHeight(unvotedBlocks[1].GetHeight())
		if err != nil {
			t.Logf("Error querying votes for height %d: %v", unvotedBlocks[1].GetHeight(), err)
			return false
		}
		if len(votes.BtcPks) > 0 {
			t.Logf("✅ Found %d vote(s) for new block at height %d", len(votes.BtcPks), unvotedBlocks[1].GetHeight())
			return true
		}
		return false
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("✅ Batch submission with duplicates test passed: " +
		"duplicates were handled gracefully and new votes were submitted successfully")
}

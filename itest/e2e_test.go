//go:build e2e
// +build e2e

package e2etest

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"testing"
	"time"

	bbntypes "github.com/babylonlabs-io/babylon/types"

	sdkmath "cosmossdk.io/math"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
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
func TestFinalityProviderLifeCycle(t *testing.T) {
	tm, fpIns := StartManagerWithFinalityProvider(t)
	defer tm.Stop(t)

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
}

// TestDoubleSigning tests the attack scenario where the finality-provider
// sends a finality vote over a conflicting block
// in this case, the BTC private key should be extracted by Babylon
func TestDoubleSigning(t *testing.T) {
	tm, fpIns := StartManagerWithFinalityProvider(t)
	defer tm.Stop(t)

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
	res, extractedKey, err := fpIns.TestSubmitFinalitySignatureAndExtractPrivKey(finalizedBlocks[0], false)
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
	_, _, err = fpIns.TestSubmitFinalitySignatureAndExtractPrivKey(b, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "double sign")

	_, extractedKey, err = fpIns.TestSubmitFinalitySignatureAndExtractPrivKey(b, false)
	require.NoError(t, err)
	require.NotNil(t, extractedKey)
	localKey := tm.GetFpPrivKey(t, fpIns.GetBtcPkBIP340().MustMarshal())
	require.True(t, localKey.Key.Equals(&extractedKey.Key) || localKey.Key.Negate().Equals(&extractedKey.Key))

	t.Logf("the equivocation attack is successful")
}

// TestCatchingUp tests if a fp can catch up after restarted
func TestCatchingUp(t *testing.T) {
	tm, fpIns := StartManagerWithFinalityProvider(t)
	defer tm.Stop(t)

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
	tm, fpIns := StartManagerWithFinalityProvider(t)
	defer tm.Stop(t)

	cmd := daemon.CommandEditFinalityDescription()

	const (
		monikerFlag          = "moniker"
		identityFlag         = "identity"
		websiteFlag          = "website"
		securityContactFlag  = "security-contact"
		detailsFlag          = "details"
		fpdDaemonAddressFlag = "daemon-address"
		commissionRateFlag   = "commission-rate"
	)

	moniker := "test-moniker"
	website := "https://test.com"
	securityContact := "test@test.com"
	details := "Test details"
	identity := "test-identity"
	commissionRateStr := "0.3"

	args := []string{
		fpIns.GetBtcPkHex(),
		"--" + fpdDaemonAddressFlag, tm.FpConfig.RPCListener,
		"--" + monikerFlag, moniker,
		"--" + websiteFlag, website,
		"--" + securityContactFlag, securityContact,
		"--" + detailsFlag, details,
		"--" + identityFlag, identity,
		"--" + commissionRateFlag, commissionRateStr,
	}

	cmd.SetArgs(args)

	// Run the command
	err := cmd.Execute()
	require.NoError(t, err)

	gotFp, err := tm.BBNClient.QueryFinalityProvider(fpIns.GetBtcPk())
	require.NoError(t, err)

	rate, err := sdkmath.LegacyNewDecFromStr(commissionRateStr)
	require.NoError(t, err)

	require.Equal(t, gotFp.FinalityProvider.Description.Moniker, moniker)
	require.Equal(t, gotFp.FinalityProvider.Description.Website, website)
	require.Equal(t, gotFp.FinalityProvider.Description.Identity, identity)
	require.Equal(t, gotFp.FinalityProvider.Description.Details, details)
	require.Equal(t, gotFp.FinalityProvider.Description.SecurityContact, securityContact)
	require.Equal(t, gotFp.FinalityProvider.Commission, &rate)

	moniker = "test2-moniker"
	args = []string{
		fpIns.GetBtcPkHex(),
		"--" + fpdDaemonAddressFlag, tm.FpConfig.RPCListener,
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
	require.Equal(t, updatedFp.FinalityProvider.Commission, &rate)
}

func TestFinalityProviderCreateCmd(t *testing.T) {
	tm, _ := StartManagerWithFinalityProvider(t)
	defer tm.Stop(t)

	cmd := daemon.CommandCreateFP()

	eotsKeyName := "eots-key-2"
	eotsPkBz, err := tm.EOTSClient.CreateKey(eotsKeyName, passphrase, hdPath)
	require.NoError(t, err)
	eotsPk, err := bbntypes.NewBIP340PubKey(eotsPkBz)
	require.NoError(t, err)

	data := struct {
		KeyName          string `json:"keyName"`
		ChainID          string `json:"chainID"`
		Passphrase       string `json:"passphrase"`
		CommissionRate   string `json:"commissionRate"`
		Moniker          string `json:"moniker"`
		Identity         string `json:"identity"`
		Website          string `json:"website"`
		SecurityContract string `json:"securityContract"`
		Details          string `json:"details"`
		EotsPK           string `json:"eotsPK"`
	}{
		KeyName:          tm.FpConfig.BabylonConfig.Key,
		ChainID:          testChainID,
		Passphrase:       passphrase,
		CommissionRate:   "0.10",
		Moniker:          "some moniker",
		Identity:         "F123456789ABCDEF",
		Website:          "https://fp.example.com",
		SecurityContract: "https://fp.example.com/security",
		Details:          "This is a highly secure and reliable fp.",
		EotsPK:           eotsPk.MarshalHex(),
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
		"--daemon-address=" + tm.FpConfig.RPCListener,
	})

	// Run the command
	err = cmd.Execute()
	require.NoError(t, err)

	fp, err := tm.BBNClient.QueryFinalityProvider(eotsPk.MustToBTCPK())
	require.NoError(t, err)
	require.NotNil(t, fp)
}

//go:build e2e_babylon
// +build e2e_babylon

package e2etest_babylon

import (
	"math/rand"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/types"
)

// TestFinalityProviderLifeCycle tests the whole life cycle of a finality-provider
// creation -> registration -> randomness commitment ->
// activation with BTC delegation and Covenant sig ->
// vote submission -> block finalization
func TestFinalityProviderLifeCycle(t *testing.T) {
	tm, fpInsList := StartManagerWithFinalityProvider(t, 1)
	defer tm.Stop(t)

	fpIns := fpInsList[0]

	// check the public randomness is committed
	e2eutils.WaitForFpPubRandCommitted(t, fpIns)

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
}

// TestDoubleSigning tests the attack scenario where the finality-provider
// sends a finality vote over a conflicting block
// in this case, the BTC private key should be extracted by Babylon
func TestDoubleSigning(t *testing.T) {
	tm, fpInsList := StartManagerWithFinalityProvider(t, 1)
	defer tm.Stop(t)

	fpIns := fpInsList[0]

	// check the public randomness is committed
	e2eutils.WaitForFpPubRandCommitted(t, fpIns)

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

	finalizedBlockHeight := tm.WaitForNFinalizedBlocksAndReturnTipHeight(t, 1)

	// attack: manually submit a finality vote over a conflicting block
	// to trigger the extraction of finality-provider's private key
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := &types.BlockInfo{
		Height: finalizedBlockHeight,
		Hash:   datagen.GenRandomByteArray(r, 32),
	}
	_, extractedKey, err := fpIns.TestSubmitFinalitySignatureAndExtractPrivKey(b)
	require.NoError(t, err)
	require.NotNil(t, extractedKey)
	localKey := tm.GetFpPrivKey(t, fpIns.GetBtcPkBIP340().MustMarshal())
	require.True(t, localKey.Key.Equals(&extractedKey.Key) || localKey.Key.Negate().Equals(&extractedKey.Key))

	t.Logf("the equivocation attack is successful")

	tm.WaitForFpShutDown(t, fpIns.GetBtcPkBIP340())

	// try to start all the finality providers and the slashed one should not be restarted
	err = tm.Fpa.StartHandlingAll()
	require.NoError(t, err)
	fps, err := tm.Fpa.ListAllFinalityProvidersInfo()
	require.NoError(t, err)
	require.Equal(t, 1, len(fps))
	require.Equal(t, proto.FinalityProviderStatus_name[4], fps[0].Status)
	require.Equal(t, false, fps[0].IsRunning)
}

// TestMultipleFinalityProviders tests starting with multiple finality providers
func TestMultipleFinalityProviders(t *testing.T) {
	n := 3
	tm, fpInstances := StartManagerWithFinalityProvider(t, n)
	defer tm.Stop(t)

	// submit BTC delegations for each finality-provider
	for _, fpi := range fpInstances {
		// check the public randomness is committed
		e2eutils.WaitForFpPubRandCommitted(t, fpi)
		// send a BTC delegation
		_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpi.GetBtcPk()}, e2eutils.StakingTime, e2eutils.StakingAmount)
	}

	// check the BTC delegations are pending
	delsResp := tm.WaitForNPendingDels(t, n)
	require.Equal(t, n, len(delsResp))

	// send covenant sigs to each of the delegations
	for _, delResp := range delsResp {
		d, err := e2eutils.ParseRespBTCDelToBTCDel(delResp)
		require.NoError(t, err)
		// send covenant sigs
		tm.InsertCovenantSigForDelegation(t, d)
	}

	// check the BTC delegations are active
	_ = tm.WaitForNActiveDels(t, n)

	// check there's a block finalized
	_ = tm.WaitForNFinalizedBlocksAndReturnTipHeight(t, 1)
}

// TestFastSync tests the fast sync process where the finality-provider is terminated and restarted with fast sync
func TestFastSync(t *testing.T) {
	tm, fpInsList := StartManagerWithFinalityProvider(t, 1)
	defer tm.Stop(t)

	fpIns := fpInsList[0]

	// check the public randomness is committed
	e2eutils.WaitForFpPubRandCommitted(t, fpIns)

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

	tm.WaitForNFinalizedBlocksAndReturnTipHeight(t, 1)

	var n uint = 3
	// stop the finality-provider for a few blocks then restart to trigger the fast sync
	tm.FpConfig.FastSyncGap = uint64(n)
	tm.StopAndRestartFpAfterNBlocks(t, n, fpIns)

	// check there are n+1 blocks finalized
	finalizedHeight := tm.WaitForNFinalizedBlocksAndReturnTipHeight(t, n+1)
	t.Logf("the latest finalized block is at %v", finalizedHeight)

	// check if the fast sync works by checking if the gap is not more than 1
	currentHeight, err := tm.BBNConsumerClient.QueryLatestBlockHeight()
	t.Logf("the current block is at %v", currentHeight)
	require.NoError(t, err)
	require.True(t, currentHeight < finalizedHeight+uint64(n))
}

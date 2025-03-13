//go:build e2e_babylon
// +build e2e_babylon

package e2etest_babylon

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"testing"

	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	btcec "github.com/btcsuite/btcd/btcec/v2"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
)

// NewDescription creates a new validator description
func NewDescription(moniker string) *stakingtypes.Description {
	return &stakingtypes.Description{
		Moniker:         moniker,
		Identity:        "",
		Website:         "",
		SecurityContact: "",
		Details:         "",
	}
}

// generateHMACKey generates a random HMAC key for testing
func generateHMACKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := cryptorand.Read(bytes); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// startManagerWithHMAC starts a test manager with finality providers configured with HMAC
func startManagerWithHMAC(t *testing.T, n int, ctx context.Context) (*TestManager, []*service.FinalityProviderInstance, func()) {
	defaultHmacKey, err := generateDefaultHMACKey()
	require.NoError(t, err)

	tm := StartManager(t, ctx, defaultHmacKey, defaultHmacKey)

	cleanup := func() {
		tm.Stop(t)
	}

	var runningFps []*service.FinalityProviderInstance
	for i := 0; i < n; i++ {
		fpIns := tm.AddFinalityProvider(t, ctx, defaultHmacKey)
		runningFps = append(runningFps, fpIns)
	}

	require.Eventually(t, func() bool {
		fps, err := tm.BBNClient.QueryFinalityProviders()
		if err != nil {
			t.Logf("failed to query finality providers from Babylon %s", err.Error())
			return false
		}
		return len(fps) == n
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	return tm, runningFps, cleanup
}

// TestHMACFinalityProviderLifeCycle tests the whole life cycle of a finality-provider with HMAC enabled
func TestHMACFinalityProviderLifeCycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	n := 2

	tm, fps, cleanup := startManagerWithHMAC(t, n, ctx)
	defer cleanup()

	tm.WaitForFpPubRandTimestamped(t, fps[0])

	for _, fp := range fps {
		_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fp.GetBtcPk()}, e2eutils.StakingTime, e2eutils.StakingAmount)
	}

	delsResp := tm.WaitForNPendingDels(t, n)
	var dels []*bstypes.BTCDelegation
	for _, delResp := range delsResp {
		del, err := e2eutils.ParseRespBTCDelToBTCDel(delResp)
		require.NoError(t, err)
		dels = append(dels, del)
		tm.InsertCovenantSigForDelegation(t, del)
	}

	_ = tm.WaitForNActiveDels(t, n)

	lastVotedHeight := tm.WaitForFpVoteCast(t, fps[0])

	tm.CheckBlockFinalization(t, lastVotedHeight, 1)
	t.Logf("the block at height %v is finalized using HMAC-authenticated finality provider", lastVotedHeight)
}

// TestHMACMismatch tests that using mismatched HMAC keys between EOTS server and client
// results in authentication failures and prevents operations
func TestHMACMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	eotsHmacKey := "server-hmac-key-for-testing"
	fpHmacKey := "client-hmac-key-for-testing-different"

	tm := StartManager(t, ctx, eotsHmacKey, fpHmacKey)
	defer tm.Stop(t)

	require.Equal(t, eotsHmacKey, tm.EOTSServerHandler.Config().HMACKey, "HMAC key should be set in the server config")
	require.Equal(t, fpHmacKey, tm.FpConfig.HMACKey, "HMAC key should be set in the FP config")

	eotsKeyName := "test-key-hmac-mismatch"
	eotsPkBytes, err := tm.EOTSServerHandler.CreateKey(eotsKeyName)
	require.NoError(t, err)

	msgToSign := []byte("test message for signing that is")
	_, err = tm.EOTSClient.SignSchnorrSig(eotsPkBytes, msgToSign)
	require.Error(t, err, "SignSchnorrSig should fail with mismatched HMAC keys")
	require.Contains(t, err.Error(), "Unauthenticated", "Expected HMAC authentication error during SignSchnorrSig")

	t.Logf("Successfully verified HMAC authentication: operation failed with authentication error: %v", err)
}

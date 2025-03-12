//go:build e2e_babylon
// +build e2e_babylon

package e2etest_babylon

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	btcec "github.com/btcsuite/btcd/btcec/v2"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
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
func startManagerWithHMAC(t *testing.T, n int, ctx context.Context) (*TestManager, []*service.FinalityProviderInstance) {
	hmacKey, err := generateHMACKey()
	require.NoError(t, err)
	t.Logf("Using HMAC key: %s", hmacKey)

	tm := StartManager(t, ctx)
	err = tm.EOTSServerHandler.SetHMACKey(hmacKey)
	require.NoError(t, err)

	var runningFps []*service.FinalityProviderInstance
	for i := 0; i < n; i++ {
		fpIns := tm.AddFinalityProvider(t, ctx, hmacKey)
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

	t.Logf("the test manager is running with finality providers using HMAC authentication")

	return tm, runningFps
}

// TestHMACFinalityProviderLifeCycle tests the whole life cycle of a finality-provider with HMAC enabled
func TestHMACFinalityProviderLifeCycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	n := 2
	tm, fps := startManagerWithHMAC(t, n, ctx)
	defer tm.Stop(t)

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

	// Generate two different HMAC keys
	serverHmacKey, err := generateHMACKey()
	require.NoError(t, err)
	clientHmacKey, err := generateHMACKey()
	require.NoError(t, err)
	t.Logf("Using server HMAC key: %s", serverHmacKey)
	t.Logf("Using client HMAC key: %s", clientHmacKey)

	os.Setenv(client.HMACKeyEnvVar, serverHmacKey)

	tm := StartManager(t, ctx)
	defer tm.Stop(t)

	err = tm.EOTSClient.Ping()
	require.NoError(t, err, "Ping should always work since authentication is disabled for it")

	r := rand.New(rand.NewSource(time.Now().Unix()))
	eotsKeyName := fmt.Sprintf("eots-key-%s", datagen.GenRandomHexStr(r, 4))

	os.Setenv(client.HMACKeyEnvVar, clientHmacKey)

	altClient, err := client.NewEOTSManagerGRpcClient(tm.EOTSServerHandler.Config().RPCListener)
	require.NoError(t, err, "Creating client should succeed since Ping is used for initial connection and doesn't require auth")
	defer altClient.Close()

	err = altClient.Ping()
	require.NoError(t, err, "Ping should work with any HMAC key")

	msgToSign := []byte("test message for signing")
	_, err = altClient.SignSchnorrSig([]byte(eotsKeyName), msgToSign)
	require.Error(t, err, "SignSchnorrSig should fail with mismatched HMAC keys")
	require.Contains(t, err.Error(), "invalid HMAC", "Expected HMAC authentication error during SignSchnorrSig")

	// Switch back to the correct HMAC key to verify the operation works properly
	os.Setenv(client.HMACKeyEnvVar, serverHmacKey)
	correctClient, err := client.NewEOTSManagerGRpcClient(tm.EOTSServerHandler.Config().RPCListener)
	require.NoError(t, err)
	defer correctClient.Close()

	_, err = correctClient.SignSchnorrSig([]byte(eotsKeyName), msgToSign)
	require.NotContains(t, err.Error(), "invalid HMAC", "Should not get HMAC authentication error with correct key")

	t.Logf("Successfully verified HMAC authentication: operations fail with wrong key but work with correct key")
}

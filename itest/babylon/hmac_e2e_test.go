package e2etest_babylon

import (
	"context"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
	"testing"
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

// TestHMACMismatch tests that using mismatched HMAC keys between EOTS server and client
// results in authentication failures and prevents operations
func TestHMACMismatch(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())

	eotsHmacKey := "server-hmac-key-for-testing"
	fpHmacKey := "client-hmac-key-for-testing-different"

	tm := StartManager(t, ctx, eotsHmacKey, fpHmacKey)
	defer func() {
		cancel()
		tm.Stop(t)
	}()

	require.Equal(t, eotsHmacKey, tm.EOTSServerHandler.Config().HMACKey, "HMAC key should be set in the server config")
	require.Equal(t, fpHmacKey, tm.FpConfig.HMACKey, "HMAC key should be set in the FP config")

	eotsKeyName := "test-key-hmac-mismatch"
	eotsPkBytes, err := tm.EOTSServerHandler.CreateKey(eotsKeyName, "")
	require.NoError(t, err)

	msgToSign := []byte("test message for signing that is")
	tm.EOTSServerHandler.Config().HMACKey = fpHmacKey // use wrong key

	eotsClient := NewEOTSManagerGrpcClientWithRetry(t, tm.EOTSServerHandler.Config())
	_, err = eotsClient.SignSchnorrSig(eotsPkBytes, msgToSign)
	require.Error(t, err, "SignSchnorrSig should fail with mismatched HMAC keys")
	require.Contains(t, err.Error(), "Unauthenticated", "Expected HMAC authentication error during SignSchnorrSig")

	t.Logf("Successfully verified HMAC authentication: operation failed with authentication error: %v", err)
}

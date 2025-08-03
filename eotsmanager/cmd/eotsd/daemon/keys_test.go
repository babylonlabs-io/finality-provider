package daemon

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	fplog "github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/testutil"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func FuzzNewKeysCmd(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		root := NewRootCmd()

		tDir := t.TempDir()
		tempHome := filepath.Join(tDir, "homeeots")
		homeFlagFilled := fmt.Sprintf("--%s=%s", sdkflags.FlagHome, tempHome)
		keyringBackendFlagFilled := fmt.Sprintf("--%s=%s", sdkflags.FlagKeyringBackend, keyring.BackendTest)

		// Create separate buffers for stdout and stderr
		stdoutBuf := new(bytes.Buffer)
		stderrBuf := new(bytes.Buffer)

		root.SetOut(stdoutBuf)
		root.SetErr(stderrBuf)

		defer func() {
			err := os.RemoveAll(tempHome)
			require.NoError(t, err)
		}()

		// Initialize the EOTS manager
		_, _ = exec(t, root, "init", homeFlagFilled)

		// Generate a random key name
		keyName := testutil.GenRandomHexStr(r, 5)

		// Execute the keys add command with keyring backend
		_, _ = exec(t, root, "keys", "add", keyName, homeFlagFilled, keyringBackendFlagFilled)

		// Execute the keys list command with keyring backend
		_, _ = exec(t, root, "keys", "list", homeFlagFilled, keyringBackendFlagFilled)

		// Log output for debugging
		t.Logf("Stdout: %q", stdoutBuf.String())
		t.Logf("Stderr: %q", stderrBuf.String())

		// Basic check - key name should be in output
		require.Contains(t, stdoutBuf.String(), keyName, "List output should contain the key name")

		// Load the EOTS manager and verify the key existence
		cfg := config.DefaultConfigWithHomePath(tempHome)
		dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)
		defer func() {
			err := dbBackend.Close()
			require.NoError(t, err)
		}()

		logger, err := fplog.NewDevLogger()
		require.NoError(t, err)
		eotsManager, err := eotsmanager.NewLocalEOTSManager(tempHome, "test", dbBackend, logger)
		require.NoError(t, err, "Should be able to create EOTS manager")

		// Verify the key exists and has correct BIP340 public key
		pubKey, err := eotsManager.LoadBIP340PubKeyFromKeyName(keyName)
		require.NoError(t, err, "Should be able to load public key")
		require.NotNil(t, pubKey, "Public key should not be nil")

		// Verify the public key is in the output
		hexPk := pubKey.MarshalHex()
		require.Contains(t, stdoutBuf.String(), hexPk, "List output should contain the BIP340 public key hex")
	})
}

// exec executes a command based on the cmd passed
func exec(t *testing.T, root *cobra.Command, args ...string) (*cobra.Command, error) {
	root.SetArgs(args)
	c, err := root.ExecuteC()
	require.NoError(t, err)

	return c, nil
}

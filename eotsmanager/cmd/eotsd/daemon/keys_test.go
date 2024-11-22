package daemon

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/testutil"
)

func FuzzNewKeysCmd(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		root := NewRootCmd()

		tDir := t.TempDir()
		tempHome := filepath.Join(tDir, "homeeots")
		homeFlagFilled := fmt.Sprintf("--%s=%s", sdkflags.FlagHome, tempHome)
		rootCmdBuff := new(bytes.Buffer)
		defer func() {
			err := os.RemoveAll(tempHome)
			require.NoError(t, err)
		}()

		// Initialize the EOTS manager
		_, _ = exec(t, root, rootCmdBuff, "init", homeFlagFilled)

		// Generate a random key name
		keyName := testutil.GenRandomHexStr(r, 5)

		// Execute the keys add command
		keyringBackendFlagFilled := fmt.Sprintf("--%s=%s", sdkflags.FlagKeyringBackend, keyring.BackendTest)
		_, _ = exec(t, root, rootCmdBuff, "keys", "add", keyName, homeFlagFilled, keyringBackendFlagFilled)

		// Execute the keys list command
		_, listOutput := exec(t, root, rootCmdBuff, "keys", "list", homeFlagFilled)

		// Check if the added key is in the list
		require.Contains(t, listOutput, keyName)

		// Load the EOTS manager and verify the key existence
		eotsCfg := eotscfg.DefaultConfigWithHomePath(tempHome)
		dbBackend, err := eotsCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)
		defer func() {
			err := dbBackend.Close()
			require.NoError(t, err)
		}()

		eotsManager, err := eotsmanager.NewLocalEOTSManager(tempHome, "test", dbBackend, zap.NewNop())
		require.NoError(t, err, "Should be able to create EOTS manager")

		pubKey, err := eotsManager.LoadBIP340PubKeyFromKeyName(keyName)
		require.NoError(t, err, "Should be able to load public key")
		require.NotNil(t, pubKey, "Public key should not be nil")
	})
}

// exec executes a command based on the cmd passed, the args should only search for subcommands, not parent commands
func exec(t *testing.T, root *cobra.Command, rootCmdBuf *bytes.Buffer, args ...string) (c *cobra.Command, output string) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	c, err := root.ExecuteC()
	require.NoError(t, err)

	outStr := buf.String()
	if len(outStr) > 0 {
		return c, outStr
	}

	_, err = buf.Write(rootCmdBuf.Bytes())
	require.NoError(t, err)

	return c, buf.String()
}

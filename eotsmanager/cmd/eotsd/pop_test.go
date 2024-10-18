package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/types"
	btcstktypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/testutil"
)

type KeyOutput struct {
	Name      string `json:"name" yaml:"name"`
	PubKeyHex string `json:"pub_key_hex" yaml:"pub_key_hex"`
	Mnemonic  string `json:"mnemonic,omitempty" yaml:"mnemonic"`
}

func FuzzPoPExport(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		tempDir := t.TempDir()
		homeDir := filepath.Join(tempDir, "eots-home")

		// init config in home folder
		initCmd := NewInitCmd()
		initCmd.Flags().AddFlagSet(NewRootCmd().PersistentFlags())
		initCmd.SetArgs(
			[]string{
				fmt.Sprintf("--%s=%s", homeFlag, homeDir),
			},
		)
		err := initCmd.Execute()
		require.NoError(t, err)

		keyName := testutil.GenRandomHexStr(r, 10)
		addKeyCmd := findSubCommand(NewKeysCmd(), "add")
		addKeyCmd.Flags().AddFlagSet(NewRootCmd().PersistentFlags())
		addKeyCmd.SetArgs([]string{
			keyName,
			fmt.Sprintf("--%s=%s", homeFlag, homeDir),
		})
		outputKeysAdd := captureOutput(func() {
			err := addKeyCmd.Execute()
			require.NoError(t, err)
		})
		keyOutJson := searchInTxt(outputKeysAdd, "for recovery):")

		var keyOut KeyOutput
		err = json.Unmarshal([]byte(keyOutJson), &keyOut)
		require.NoError(t, err)

		bbnAddr := datagen.GenRandomAccount().GetAddress()

		exportPoPCmd := NewExportPoPCmd()
		exportPoPCmd.SetArgs([]string{
			bbnAddr.String(),
			"--home", homeDir,
			"--eots-pk", keyOut.PubKeyHex,
		})

		exportedPoP := captureOutput(func() {
			err := exportPoPCmd.Execute()
			require.NoError(t, err)
		})

		var popExport PoPExport
		err = json.Unmarshal([]byte(exportedPoP), &popExport)
		require.NoError(t, err)

		pop, err := btcstktypes.NewPoPBTCFromHex(popExport.PoPHex)
		require.NoError(t, err)

		require.NotNil(t, popExport)
		require.NoError(t, pop.ValidateBasic())

		btcPubKey, err := bbn.NewBIP340PubKeyFromHex(popExport.PubKeyHex)
		require.NoError(t, err)
		require.NoError(t, pop.Verify(bbnAddr, btcPubKey, &chaincfg.SigNetParams))
	})
}

// Helper function to capture command output
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

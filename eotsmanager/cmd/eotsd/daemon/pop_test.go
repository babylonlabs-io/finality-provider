package daemon_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"

	dcli "github.com/babylonchain/finality-provider/eotsmanager/cmd/eotsd/daemon"
	"github.com/babylonchain/finality-provider/testutil"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/types"
	btcstktypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func FuzzPoPExport(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		tempDir := t.TempDir()
		homeDir := filepath.Join(tempDir, "eots-home")
		app := testApp()

		// init config in home folder
		hFlag := fmt.Sprintf("--home=%s", homeDir)
		err := app.Run([]string{"eotsd", "init", hFlag})
		require.NoError(t, err)

		keyName := testutil.GenRandomHexStr(r, 10)
		keyNameFlag := fmt.Sprintf("--key-name=%s", keyName)

		outputKeysAdd := appRunWithOutput(r, t, app, []string{"eotsd", "keys", "add", hFlag, keyNameFlag})
		keyOutJson := searchInTxt(outputKeysAdd, "for recovery):")

		var keyOut dcli.KeyOutput
		err = json.Unmarshal([]byte(keyOutJson), &keyOut)
		require.NoError(t, err)

		bbnAddr := datagen.GenRandomAccount().GetAddress()

		btcPkFlag := fmt.Sprintf("--btc-pk=%s", keyOut.PubKeyHex)
		exportedPoP := appRunPoPExport(r, t, app, []string{bbnAddr.String(), hFlag, btcPkFlag})
		pop, err := btcstktypes.NewPoPBTCFromHex(exportedPoP.PoPHex)
		require.NoError(t, err)

		require.NotNil(t, exportedPoP)
		require.NoError(t, pop.ValidateBasic())

		btcPubKey, err := bbn.NewBIP340PubKeyFromHex(exportedPoP.PubKeyHex)
		require.NoError(t, err)
		require.NoError(t, pop.Verify(bbnAddr, btcPubKey, &chaincfg.SigNetParams))
	})
}

func appRunPoPExport(r *rand.Rand, t *testing.T, app *cli.App, arguments []string) dcli.PoPExport {
	args := []string{"eotsd", "pop-export"}
	args = append(args, arguments...)
	outputSign := appRunWithOutput(r, t, app, args)
	signatureStr := searchInTxt(outputSign, "")

	var dataSigned dcli.PoPExport
	err := json.Unmarshal([]byte(signatureStr), &dataSigned)
	require.NoError(t, err)

	return dataSigned
}

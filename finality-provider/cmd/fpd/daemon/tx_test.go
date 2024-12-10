package daemon_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/spf13/cobra"

	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/babylon/app"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/types"

	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"

	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	"github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/testutil"
)

func FuzzNewValidateSignedFinalityProviderCmd(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	tempApp := app.NewTmpBabylonApp()

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		rootCmdBuff := new(bytes.Buffer)
		root := rootCmd(rootCmdBuff)

		tDir := t.TempDir()
		tempHome := filepath.Join(tDir, "homefpnew")
		homeFlag := fmt.Sprintf("--home=%s", tempHome)

		exec(t, root, rootCmdBuff, "init", homeFlag)

		keyName := datagen.GenRandomHexStr(r, 5)
		kbt := "--keyring-backend=test"

		keyOut := execUnmarshal[keys.KeyOutput](t, root, rootCmdBuff, "keys", "add", keyName, homeFlag, kbt)
		require.Equal(t, keyName, keyOut.Name)

		fpAddr, err := sdk.AccAddressFromBech32(keyOut.Address)
		require.NoError(t, err)

		btcSK, btcPK, err := datagen.GenRandomBTCKeyPair(r)
		require.NoError(t, err)

		pop, err := btcstakingtypes.NewPoPBTC(fpAddr, btcSK)
		require.NoError(t, err)

		popHex, err := pop.ToHexStr()
		require.NoError(t, err)

		bip340PK := bbn.NewBIP340PubKeyFromBTCPK(btcPK)

		_, unsignedMsgStr := exec(
			t, root, rootCmdBuff, "tx", "create-finality-provider", bip340PK.MarshalHex(), popHex, homeFlag, kbt,
			fmt.Sprintf("--from=%s", keyName), "--generate-only", "--gas-prices=10ubbn",
			"--commission-rate=0.05", "--moniker='niceFP'", "--identity=x", "--website=test.com",
			"--security-contact=niceEmail", "--details='no Details'",
		)

		tx, err := tempApp.TxConfig().TxJSONDecoder()([]byte(unsignedMsgStr))
		require.NoError(t, err)

		txMsgs := tx.GetMsgs()
		require.Len(t, txMsgs, 1)
		sdkMsg := txMsgs[0]

		msg, ok := sdkMsg.(*btcstakingtypes.MsgCreateFinalityProvider)
		require.True(t, ok)
		require.Equal(t, msg.Addr, fpAddr.String())

		// sends the unsigned msg to a file to be signed.
		unsignedMsgFilePath := writeToTempFile(t, r, unsignedMsgStr)

		_, signedMsgStr := exec(t, root, rootCmdBuff, "tx", "sign", unsignedMsgFilePath, homeFlag, kbt,
			fmt.Sprintf("--from=%s", keyName), "--offline", "--account-number=0", "--sequence=0",
		)
		// sends the signed msg to a file.
		signedMsgFilePath := writeToTempFile(t, r, signedMsgStr)

		tx, err = tempApp.TxConfig().TxJSONDecoder()([]byte(signedMsgStr))
		require.NoError(t, err)

		msgSigned, ok := tx.GetMsgs()[0].(*btcstakingtypes.MsgCreateFinalityProvider)
		require.True(t, ok)
		require.Equal(t, msgSigned.Addr, fpAddr.String())

		// executes the verification.
		_, outputValidate := exec(t, root, rootCmdBuff, "tx", "validate-signed-finality-provider", signedMsgFilePath, homeFlag)
		require.Equal(t, "The signed MsgCreateFinalityProvider is valid", outputValidate)
	})
}

func rootCmd(outputBuff *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "fpd",
		PersistentPreRunE: fpcmd.PersistClientCtx(client.Context{}.WithOutput(outputBuff)),
	}
	cmd.PersistentFlags().String(flags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")

	cmd.AddCommand(
		daemon.CommandInit(), daemon.CommandStart(), daemon.CommandKeys(),
		daemon.CommandGetDaemonInfo(), daemon.CommandCreateFP(), daemon.CommandLsFP(),
		daemon.CommandInfoFP(), daemon.CommandAddFinalitySig(),
		daemon.CommandTxs(),
	)

	return cmd
}

// exec executes a command based on the cmd passed, the args should only search for subcommands, not parent commands
// it also receives rootCmdBuf as a parameter, which is the buff set in the cmd context standard output for the commands
// that do print to the context stdout instead of the root stdout.
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

func execUnmarshal[structure any](t *testing.T, root *cobra.Command, rootCmdBuf *bytes.Buffer, args ...string) (output structure) {
	var typed structure
	_, outStr := exec(t, root, rootCmdBuf, args...)

	// helpful for debug
	// fmt.Printf("\nargs %s", strings.Join(args, " "))
	// fmt.Printf("\nout %s", outStr)

	err := json.Unmarshal([]byte(outStr), &typed)
	require.NoError(t, err)

	return typed
}

func writeToTempFile(t *testing.T, r *rand.Rand, str string) (outputFilePath string) {
	outputFilePath = filepath.Join(t.TempDir(), datagen.GenRandomHexStr(r, 5))

	outPutFile, err := os.Create(outputFilePath)
	require.NoError(t, err)
	defer outPutFile.Close()

	_, err = outPutFile.WriteString(str)
	require.NoError(t, err)

	return outputFilePath
}

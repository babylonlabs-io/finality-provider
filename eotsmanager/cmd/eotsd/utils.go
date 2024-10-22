package main

import (
	"os"
	"path/filepath"

	"github.com/babylonlabs-io/babylon/app"
	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

	"github.com/babylonlabs-io/finality-provider/util"
)

func getHomePath(cmd *cobra.Command) (string, error) {
	rawHomePath, err := cmd.Flags().GetString(sdkflags.FlagHome)
	if err != nil {
		return "", err
	}

	homePath, err := filepath.Abs(rawHomePath)
	if err != nil {
		return "", err
	}
	// Create home directory
	homePath = util.CleanAndExpandPath(homePath)

	return homePath, nil
}

// PersistClientCtx persist some vars from the cmd or config to the client context.
// It gives preferences to flags over the values in the config. If the flag is not set
// and exists a value in the config that could be used, it will be set in the ctx.
func PersistClientCtx(ctx client.Context) func(cmd *cobra.Command, _ []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		// TODO(verify): if it uses the default encoding config it fails to list keys! output:
		// "xx" is not a valid name or address: unable to unmarshal item.Data:
		// Bytes left over in UnmarshalBinaryLengthPrefixed, should read 10 more bytes but have 154
		// [cosmos/cosmos-sdk@v0.50.6/crypto/keyring/keyring.go:973
		tempApp := app.NewTmpBabylonApp()

		ctx = ctx.
			WithCodec(tempApp.AppCodec()).
			WithInterfaceRegistry(tempApp.InterfaceRegistry()).
			WithTxConfig(tempApp.TxConfig()).
			WithLegacyAmino(tempApp.LegacyAmino()).
			WithInput(os.Stdin)

		// set the default command outputs
		cmd.SetOut(cmd.OutOrStdout())
		cmd.SetErr(cmd.ErrOrStderr())

		ctx = ctx.WithCmdContext(cmd.Context())

		// updates the ctx in the cmd in case something was modified bt the config
		return client.SetCmdClientContextHandler(ctx, cmd)
	}
}

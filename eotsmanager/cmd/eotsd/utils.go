package main

import (
	"os"
	"path/filepath"

	"github.com/babylonlabs-io/babylon/app/params"
	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/std"
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
		encCfg := params.DefaultEncodingConfig()
		std.RegisterInterfaces(encCfg.InterfaceRegistry)

		ctx = ctx.
			WithCodec(encCfg.Codec).
			WithInterfaceRegistry(encCfg.InterfaceRegistry).
			WithTxConfig(encCfg.TxConfig).
			WithLegacyAmino(encCfg.Amino).
			WithInput(os.Stdin)

		// set the default command outputs
		cmd.SetOut(cmd.OutOrStdout())
		cmd.SetErr(cmd.ErrOrStderr())

		ctx = ctx.WithCmdContext(cmd.Context())

		// updates the ctx in the cmd in case something was modified bt the config
		return client.SetCmdClientContextHandler(ctx, cmd)
	}
}

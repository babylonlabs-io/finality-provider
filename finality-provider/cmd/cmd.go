package cmd

import (
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/babylonlabs-io/babylon/app"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
)

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

		if err := client.SetCmdClientContextHandler(ctx, cmd); err != nil {
			return err
		}

		ctx = client.GetClientContextFromCmd(cmd)
		// check the config file exists
		cfg, err := fpcfg.LoadConfig(ctx.HomeDir)
		if err != nil {
			return nil // if no conifg is found just stop.
		}

		// config was found, load the defaults if not set by flag
		// flags have preference over config.
		ctx, err = FillContextFromBabylonConfig(ctx, cmd.Flags(), cfg.BabylonConfig)
		if err != nil {
			return err
		}

		// updates the ctx in the cmd in case something was modified bt the config
		return client.SetCmdClientContext(cmd, ctx)
	}
}

// FillContextFromBabylonConfig loads the bbn config to the context if values were not set by flag.
// Preference is FlagSet values over the config.
func FillContextFromBabylonConfig(ctx client.Context, flagSet *pflag.FlagSet, bbnConf *fpcfg.BBNConfig) (client.Context, error) {
	if !flagSet.Changed(flags.FlagFrom) {
		ctx = ctx.WithFrom(bbnConf.Key)
	}
	if !flagSet.Changed(flags.FlagChainID) {
		ctx = ctx.WithChainID(bbnConf.ChainID)
	}
	if !flagSet.Changed(flags.FlagKeyringBackend) {
		kr, err := client.NewKeyringFromBackend(ctx, bbnConf.KeyringBackend)
		if err != nil {
			return ctx, err
		}

		ctx = ctx.WithKeyring(kr)
	}
	if !flagSet.Changed(flags.FlagKeyringDir) {
		ctx = ctx.WithKeyringDir(bbnConf.KeyDirectory)
	}
	if !flagSet.Changed(flags.FlagOutput) {
		ctx = ctx.WithOutputFormat(bbnConf.OutputFormat)
	}
	if !flagSet.Changed(flags.FlagSignMode) {
		ctx = ctx.WithSignModeStr(bbnConf.SignModeStr)
	}

	return ctx, nil
}

// RunEWithClientCtx runs cmd with client context and returns an error.
func RunEWithClientCtx(
	fRunWithCtx func(ctx client.Context, cmd *cobra.Command, args []string) error,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		clientCtx, err := client.GetClientQueryContext(cmd)
		if err != nil {
			return err
		}

		return fRunWithCtx(clientCtx, cmd, args)
	}
}

package daemon

import (
	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	goflags "github.com/jessevdk/go-flags"
	"github.com/spf13/cobra"

	helper "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/util"
)

// CommandKeys returns the keys group command and updates the add command to do a
// post run action to update the config if exists.
func CommandKeys() *cobra.Command {
	keysCmd := keys.Commands()
	keyAddCmd := util.GetSubCommand(keysCmd, "add")
	if keyAddCmd == nil {
		panic("failed to find keys add command")
	}

	keyAddCmd.PostRunE = helper.RunEWithClientCtx(func(ctx client.Context, cmd *cobra.Command, args []string) error {
		// check the config file exists
		cfg, err := fpcfg.LoadConfig(ctx.HomeDir)
		if err != nil {
			//nolint:nilerr
			return nil // config does not exist, so does not update it
		}

		keyringBackend, err := cmd.Flags().GetString(sdkflags.FlagKeyringBackend)
		if err != nil {
			return err
		}

		// write the updated config into the config file
		cfg.BabylonConfig.Key = args[0]
		cfg.BabylonConfig.KeyringBackend = keyringBackend
		fileParser := goflags.NewParser(cfg, goflags.Default)

		return goflags.NewIniParser(fileParser).WriteFile(fpcfg.CfgFile(ctx.HomeDir), goflags.IniIncludeComments|goflags.IniIncludeDefaults)
	})

	return keysCmd
}

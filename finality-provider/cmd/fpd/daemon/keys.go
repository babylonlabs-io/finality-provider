package daemon

import (
	"strings"

	helper "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	goflags "github.com/jessevdk/go-flags"
	"github.com/spf13/cobra"
)

// CommandKeys returns the keys group command and updates the add command to do a
// post run action to update the config if exists.
func CommandKeys() *cobra.Command {
	keysCmd := keys.Commands()
	keyAddCmd := GetSubCommand(keysCmd, "add")
	if keyAddCmd == nil {
		panic("failed to find keys add command")
	}

	keyAddCmd.PostRunE = helper.RunEWithClientCtx(func(ctx client.Context, cmd *cobra.Command, args []string) error {
		// check the config file exists
		cfg, err := fpcfg.LoadConfig(ctx.HomeDir)
		if err != nil {
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

		return goflags.NewIniParser(fileParser).WriteFile(fpcfg.ConfigFile(ctx.HomeDir), goflags.IniIncludeComments|goflags.IniIncludeDefaults)
	})

	return keysCmd
}

// GetSubCommand retuns the command if it finds, otherwise it returns nil
func GetSubCommand(cmd *cobra.Command, commandName string) *cobra.Command {
	for _, c := range cmd.Commands() {
		if !strings.EqualFold(c.Name(), commandName) {
			continue
		}
		return c
	}
	return nil
}

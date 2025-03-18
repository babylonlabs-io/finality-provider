package daemon

import (
	"fmt"

	"github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/spf13/cobra"
)

// CommandKeys returns the keys group command and updates the add command to do a
// post run action to update the config if exists.
func CommandKeys() *cobra.Command {
	keysCmd := keys.Commands()
	keyAddCmd := util.GetSubCommand(keysCmd, "add")
	if keyAddCmd == nil {
		panic("failed to find keys add command")
	}

	keyAddCmd.Long += "\nIf this key is needed to run as the default for the finality-provider daemon, remind to update the fpd.conf"

	keyAddCmd.Flags().String(keyringBackendFlag, "", "The keyring backend to use")

	originalRunE := keyAddCmd.RunE

	keyAddCmd.RunE = func(cmd *cobra.Command, args []string) error {
		keyringBackend, err := cmd.Flags().GetString(keyringBackendFlag)
		if err != nil {
			return fmt.Errorf("failed to get keyring-backend flag: %w", err)
		}
		clientCtx, err := client.GetClientQueryContext(cmd)
		if err != nil {
			return fmt.Errorf("failed to get context: %w", err)
		}

		var cfg *config.Config
		if keyringBackend == "" {
			cfg, err = config.LoadConfig(clientCtx.HomeDir)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.BabylonConfig.KeyringBackend != "test" {
				return fmt.Errorf(`the keyring backend should be "test"`)
			}
		} else {
			fmt.Printf("flag KeyringBackend: %s\n", keyringBackend)
			if keyringBackend != "test" {
				return fmt.Errorf(`the keyring backend should be "test"`)
			}
			cfg, err = config.LoadConfig(clientCtx.HomeDir)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			cfg.BabylonConfig.KeyringBackend = keyringBackend
			if err := config.SaveConfig(cfg, clientCtx.HomeDir); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
		}

		if originalRunE != nil {
			return originalRunE(cmd, args)
		}

		return nil
	}

	return keysCmd
}

package daemon

import (
	"strings"

	"github.com/cosmos/cosmos-sdk/client/keys"
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

	keyAddCmd.Long += "\nIf this key is needed to run as the default for the finality-provider daemon, remind to update the fpd.conf"
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

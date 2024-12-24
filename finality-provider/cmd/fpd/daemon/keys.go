package daemon

import (
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/spf13/cobra"

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

	keyAddCmd.Long += "\nIf this key is needed to run as the default for the finality-provider daemon, remind to update the fpd.conf"

	return keysCmd
}

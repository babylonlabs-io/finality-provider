package daemon

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	btcstakingcli "github.com/babylonlabs-io/babylon/x/btcstaking/client/cli"
	authcli "github.com/cosmos/cosmos-sdk/x/auth/client/cli"
)

// CommandTxs returns the transaction commands for finality provider related msgs.
func CommandTxs() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "tx",
		Short:                      "transactions subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		authcli.GetSignCommand(),
		btcstakingcli.NewCreateFinalityProviderCmd(),
	)

	return cmd
}

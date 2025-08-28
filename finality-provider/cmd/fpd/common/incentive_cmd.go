//nolint:revive
package common

import (
	incentivecli "github.com/babylonlabs-io/babylon/v3/x/incentive/client/cli"
	"github.com/spf13/cobra"
)

// AddIncentiveCommands adds all the incentive-related commands to the provided command.
// The incentive commands are generic to {Babylon, Cosmos BSN, rollup BSN} finality providers
func AddIncentiveCommands(cmd *cobra.Command) {
	cmd.AddCommand(
		incentivecli.NewWithdrawRewardCmd(),
		incentivecli.NewSetWithdrawAddressCmd(),
		incentivecli.CmdQueryRewardGauges(),
	)
}

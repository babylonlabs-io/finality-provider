package incentive

import (
	incentivecli "github.com/babylonlabs-io/babylon/v3/x/incentive/client/cli"
	"github.com/spf13/cobra"
)

func AddIncentiveCommands(rootCmd *cobra.Command) {
	rootCmd.AddCommand(
		incentivecli.NewWithdrawRewardCmd(),
		incentivecli.NewSetWithdrawAddressCmd(),
		incentivecli.CmdQueryRewardGauges(),
	)
}

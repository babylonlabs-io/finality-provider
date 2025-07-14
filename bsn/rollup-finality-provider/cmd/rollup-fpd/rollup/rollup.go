package rollup

import "github.com/spf13/cobra"

func AddRollupBSNCommands(rootCmd *cobra.Command) {
	rootCmd.AddCommand(
		CommandInit(),
		CommandStart(),
		CommandCreateFP(),
		CommandCommitPubRand(),
		CommandRecoverProof(),
	)
}

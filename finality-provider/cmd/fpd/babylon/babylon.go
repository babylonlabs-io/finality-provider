package babylon

import "github.com/spf13/cobra"

func AddBabylonCommands(rootCmd *cobra.Command) {
	rootCmd.AddCommand(
		CommandInit(),
		CommandStart(),
		CommandCreateFP(),
		CommandCommitPubRand(),
		CommandRecoverProof(),
	)
}

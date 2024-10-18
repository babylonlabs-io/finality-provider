package main

import (
	"github.com/spf13/cobra"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
)

// NewRootCmd creates a new root command for fpd. It is called once in the main function.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "eotsd",
		Short:         "A daemon program from managing Extractable One Time Signatures (eotsd).",
		SilenceErrors: false,
	}

	rootCmd.PersistentFlags().String(homeFlag, config.DefaultEOTSDir, "The application home directory")

	rootCmd.AddCommand(
		NewInitCmd(),
		NewKeysCmd(),
		NewStartCmd(),
		NewExportPoPCmd(),
		NewSignSchnorrCmd(),
	)

	return rootCmd
}

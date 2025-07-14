package daemon

import (
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/version"
	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
)

// NewRootCmd creates a new root command for fpd. It is called once in the main function.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:               "eotsd",
		Short:             "A daemon program from managing Extractable One Time Signatures (eotsd).",
		SilenceErrors:     false,
		PersistentPreRunE: PersistClientCtx(client.Context{}),
	}

	rootCmd.PersistentFlags().String(sdkflags.FlagHome, config.DefaultEOTSDir, "The application home directory")

	rootCmd.AddCommand(
		NewInitCmd(),
		NewKeysCmd(),
		NewStartCmd(),
		version.CommandVersion("eotsd"),
		NewPopCmd(),
		NewSignStoreRollbackCmd(),
		NewBackupCmd(),
		NewUnlockKeyringCmd(),
	)

	return rootCmd
}

package main

import (
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	"github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
)

// NewRootCmd creates a new root command for fpd. It is called once in the main function.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:               "fpd",
		Short:             "fpd - Finality Provider Daemon (fpd).",
		Long:              `fpd is the daemon to create and manage finality providers.`,
		SilenceErrors:     false,
		PersistentPreRunE: fpcmd.PersistClientCtx(client.Context{}),
	}
	rootCmd.PersistentFlags().String(flags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")

	return rootCmd
}

func main() {
	cmd := NewRootCmd()
	cmd.AddCommand(
		daemon.CommandInit(), daemon.CommandStart(), daemon.CommandKeys(),
		daemon.CommandGetDaemonInfo(), daemon.CommandCreateFP(), daemon.CommandLsFP(),
		daemon.CommandInfoFP(), daemon.CommandRegisterFP(), daemon.CommandAddFinalitySig(),
		daemon.CommandExportFP(), daemon.CommandTxs(), daemon.CommandUnjailFP(),
		daemon.CommandEditFinalityDescription(), daemon.CommandVersion(),
	)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Whoops. There was an error while executing your fpd CLI '%s'", err)
		os.Exit(1)
	}
}

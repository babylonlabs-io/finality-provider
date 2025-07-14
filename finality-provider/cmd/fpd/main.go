package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	"github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/babylon"
	"github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	"github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/incentive"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/version"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
)

const BinaryName = "fpd"

// NewRootCmd creates a new root command for fpd. It is called once in the main function.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:               BinaryName,
		Short:             fmt.Sprintf("%s - Finality Provider Daemon.", BinaryName),
		Long:              fmt.Sprintf(`%s is the daemon to create and manage finality providers.`, BinaryName),
		SilenceErrors:     false,
		PersistentPreRunE: fpcmd.PersistClientCtx(client.Context{}),
	}
	rootCmd.PersistentFlags().String(flags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")

	return rootCmd
}

func main() {
	cmd := NewRootCmd()

	// add daemon commands
	daemon.AddDaemonCommands(cmd)
	// add incentive commands
	incentive.AddIncentiveCommands(cmd)
	// add version command
	version.AddVersionCommand(cmd, BinaryName)
	// Babylon-specific commands
	babylon.AddBabylonCommands(cmd)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := cmd.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Whoops. There was an error while executing your fpd CLI '%s'", err)
		os.Exit(1) //nolint:gocritic
	}
}

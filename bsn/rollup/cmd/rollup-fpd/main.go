package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	rollupfpdaemon "github.com/babylonlabs-io/finality-provider/bsn/rollup/cmd/rollup-fpd/daemon"
	clientctx "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	commoncmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/common"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/version"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
)

const BinaryName = "rollup-fpd"

// NewRootCmd creates a new root command for fpd. It is called once in the main function.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:               BinaryName,
		Short:             fmt.Sprintf("%s - Finality Provider Daemon for rollup BSNs.", BinaryName),
		Long:              fmt.Sprintf(`%s is the daemon to create and manage finality providers for rollup BSNs.`, BinaryName),
		SilenceErrors:     false,
		PersistentPreRunE: clientctx.PersistClientCtx(client.Context{}),
	}
	rootCmd.PersistentFlags().String(flags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")

	return rootCmd
}

func main() {
	cmd := NewRootCmd()

	// add all common commands
	commoncmd.AddCommonCommands(cmd, BinaryName)
	// add all keys commands
	commoncmd.AddKeysCommands(cmd)
	// add all incentive commands
	commoncmd.AddIncentiveCommands(cmd)
	// add version commands
	version.AddVersionCommands(cmd, BinaryName)

	// add the rest of commands that are specific to rollup finality provider
	cmd.AddCommand(
		rollupfpdaemon.CommandInit(BinaryName),
		rollupfpdaemon.CommandStart(BinaryName),
		rollupfpdaemon.CommandCreateFP(BinaryName),
		rollupfpdaemon.CommandCommitPubRand(BinaryName),
		rollupfpdaemon.CommandRecoverProof(BinaryName),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := cmd.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Whoops. There was an error while executing your fpd CLI '%s'", err)
		os.Exit(1) //nolint:gocritic
	}
}

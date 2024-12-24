package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	incentivecli "github.com/babylonlabs-io/babylon/x/incentive/client/cli"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	"github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/version"
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
		daemon.CommandInfoFP(), daemon.CommandAddFinalitySig(), daemon.CommandUnjailFP(),
		daemon.CommandEditFinalityDescription(), daemon.CommandCommitPubRand(),
		incentivecli.NewWithdrawRewardCmd(), incentivecli.NewSetWithdrawAddressCmd(),
		version.CommandVersion("fpd"), daemon.CommandUnsafePruneMerkleProof(),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := cmd.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Whoops. There was an error while executing your fpd CLI '%s'", err)
		os.Exit(1) //nolint:gocritic
	}
}

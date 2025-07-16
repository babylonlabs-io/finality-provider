package daemon

import (
	"fmt"
	"path/filepath"

	rollupfpcfg "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/config"
	clientctx "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	fpdaemon "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
)

// CommandCommitPubRand returns the commit-pubrand command by connecting to the fpd daemon.
func CommandCommitPubRand(binaryName string) *cobra.Command {
	cmd := fpdaemon.CommandCommitPubRandTemplate(binaryName)
	cmd.RunE = clientctx.RunEWithClientCtx(runCommandCommitPubRand)

	return cmd
}

func runCommandCommitPubRand(ctx client.Context, cmd *cobra.Command, args []string) error {
	// Get homePath from context like in start.go
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return err
	}
	homePath = util.CleanAndExpandPath(homePath)

	cfg, err := rollupfpcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	return fpdaemon.RunCommandCommitPubRandWithConfig(ctx, cmd, homePath, cfg.Common, args)
}

package rollup

import (
	"fmt"
	"math"
	"path/filepath"

	rollupcfg "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/config"
	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	babyloncmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/babylon"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
)

// CommandCommitPubRand returns the commit-pubrand command by connecting to the fpd daemon.
func CommandCommitPubRand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "unsafe-commit-pubrand [fp-eots-pk-hex] [target-height]",
		Aliases: []string{"unsafe-cpr"},
		Short:   "[UNSAFE] Manually trigger public randomness commitment for a finality provider",
		Long: `[UNSAFE] Manually trigger public randomness commitment for a finality provider.
WARNING: this can drain the finality provider's balance if the target height is too high.`,
		Example: `fpd unsafe-commit-pubrand --home /home/user/.fpd [fp-eots-pk-hex] [target-height]`,
		Args:    cobra.ExactArgs(2),
		RunE:    fpcmd.RunEWithClientCtx(runCommandCommitPubRand),
	}
	cmd.Flags().Uint64("start-height", math.MaxUint64, "The block height to start committing pubrand from (optional)")
	cmd.Flags().String(flags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")

	return cmd
}

func runCommandCommitPubRand(ctx client.Context, cmd *cobra.Command, args []string) error {
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return err
	}
	homePath = util.CleanAndExpandPath(homePath)
	cfg, err := rollupcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	return babyloncmd.RunCommandCommitPubRandWithCfg(ctx, cmd, cfg.Common, args)
}

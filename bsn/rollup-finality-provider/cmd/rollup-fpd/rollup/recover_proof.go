package rollup

import (
	"fmt"
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

func CommandRecoverProof() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "recover-rand-proof [fp-eots-pk-hex]",
		Aliases: []string{"rrp"},
		Short:   "Recover the public randomness' merkle proof for a finality provider",
		Long:    "Recover the public randomness' merkle proof for a finality provider. Currently only Babylon consumer chain is supported.",
		Example: `fpd recover-rand-proof --home /home/user/.fpd [fp-eots-pk-hex]`,
		Args:    cobra.ExactArgs(1),
		RunE:    fpcmd.RunEWithClientCtx(runCommandRecoverProof),
	}
	cmd.Flags().Uint64("start-height", 1, "The block height from which the proof is recovered from (optional)")
	cmd.Flags().String(flags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")
	cmd.Flags().String(flags.FlagChainID, "", "The consumer identifier")

	return cmd
}

func runCommandRecoverProof(ctx client.Context, cmd *cobra.Command, args []string) error {
	chainID, err := cmd.Flags().GetString(flags.FlagChainID)
	if err != nil {
		return fmt.Errorf("failed to read chain id flag: %w", err)
	}

	if chainID == "" {
		return fmt.Errorf("please specify chain-id")
	}

	startHeight, err := cmd.Flags().GetUint64("start-height")
	if err != nil {
		return err
	}

	// Get homePath from context like in start.go
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return err
	}
	homePath = util.CleanAndExpandPath(homePath)

	cfg, err := rollupcfg.LoadRollupFPConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	return babyloncmd.RunCommandRecoverProofWithCfg(cfg.Common, args[0], chainID, startHeight, homePath)
}

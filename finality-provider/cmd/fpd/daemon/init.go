package daemon

import (
	"fmt"
	"path/filepath"

	clientctx "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	commoncmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/common"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/jessevdk/go-flags"
	"github.com/spf13/cobra"
)

// CommandInit returns the init command of fpd daemon that starts the config dir.
func CommandInit(binaryName string) *cobra.Command {
	cmd := CommandInitTemplate(binaryName)
	cmd.RunE = clientctx.RunEWithClientCtx(runInitCmd)

	return cmd
}

func CommandInitTemplate(binaryName string) *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "init",
		Short:   "Initialize a finality-provider home directory.",
		Long:    `Creates a new finality-provider home directory with default config`,
		Example: fmt.Sprintf(`%s init --home /home/user/.fpd --force`, binaryName),
		Args:    cobra.NoArgs,
	}
	cmd.Flags().Bool(commoncmd.ForceFlag, false, "Override existing configuration")

	return cmd
}

func runInitCmd(ctx client.Context, cmd *cobra.Command, _ []string) error {
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return err
	}
	homePath = util.CleanAndExpandPath(homePath)

	force, err := cmd.Flags().GetBool(commoncmd.ForceFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", commoncmd.ForceFlag, err)
	}

	if util.FileExists(homePath) && !force {
		return fmt.Errorf("home path %s already exists", homePath)
	}

	if err := util.MakeDirectory(homePath); err != nil {
		return err
	}
	// Create log directory
	logDir := fpcfg.LogDir(homePath)
	if err := util.MakeDirectory(logDir); err != nil {
		return err
	}

	defaultConfig := fpcfg.DefaultConfigWithHome(homePath)
	fileParser := flags.NewParser(&defaultConfig, flags.Default)

	return flags.NewIniParser(fileParser).WriteFile(fpcfg.CfgFile(homePath), flags.IniIncludeComments|flags.IniIncludeDefaults)
}

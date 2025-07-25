package daemon

import (
	"fmt"
	"github.com/babylonlabs-io/finality-provider/bsn/cosmos/config"
	"path/filepath"

	"github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	commoncmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/common"
	fpdaemon "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/jessevdk/go-flags"
	"github.com/spf13/cobra"
)

// CommandInit returns the init command of fpd daemon that starts the config dir.
func CommandInit(binaryName string) *cobra.Command {
	cmd := fpdaemon.CommandInitTemplate(binaryName)
	cmd.RunE = clientctx.RunEWithClientCtx(runInitCmd)

	return cmd
}

func runInitCmd(ctx client.Context, cmd *cobra.Command, _ []string) error {
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
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
		return fmt.Errorf("failed to create home directory: %w", err)
	}
	// Create log directory
	logDir := config.LogDir(homePath)
	if err := util.MakeDirectory(logDir); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	defaultConfig := config.DefaultConfigWithHome(homePath)
	fileParser := flags.NewParser(&defaultConfig, flags.Default)

	if err := flags.NewIniParser(fileParser).WriteFile(config.CfgFile(homePath), flags.IniIncludeComments|flags.IniIncludeDefaults); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

package daemon

import (
	"fmt"

	"github.com/jessevdk/go-flags"
	"github.com/spf13/cobra"

	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/util"
)

func NewInitCmd() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init <path to executable>",
		Short: "Initialize the eotsd home directory.",
		RunE:  initHome,
	}

	initCmd.Flags().Bool(forceFlag, false, "Override existing configuration")

	return initCmd
}

func initHome(cmd *cobra.Command, _ []string) error {
	homePath, err := getHomePath(cmd)
	if err != nil {
		return fmt.Errorf("failed to get home path: %w", err)
	}
	force, err := cmd.Flags().GetBool(forceFlag)
	if err != nil {
		return fmt.Errorf("failed to get force flag: %w", err)
	}

	if util.FileExists(homePath) && !force {
		return fmt.Errorf("home path %s already exists", homePath)
	}

	if err := util.MakeDirectory(homePath); err != nil {
		return fmt.Errorf("failed to create home directory: %w", err)
	}
	// Create log directory
	logDir := eotscfg.LogDir(homePath)
	if err := util.MakeDirectory(logDir); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}
	// Create data directory
	dataDir := eotscfg.DataDir(homePath)
	if err := util.MakeDirectory(dataDir); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	defaultConfig := eotscfg.DefaultConfig()
	defaultConfig.DatabaseConfig.DBPath = dataDir
	fileParser := flags.NewParser(defaultConfig, flags.Default)

	if err := flags.NewIniParser(fileParser).WriteFile(eotscfg.CfgFile(homePath), flags.IniIncludeComments|flags.IniIncludeDefaults); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

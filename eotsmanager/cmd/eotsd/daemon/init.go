package daemon

import (
	"fmt"
	"path/filepath"

	"github.com/jessevdk/go-flags"
	"github.com/urfave/cli"

	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/util"
)

var InitCommand = cli.Command{
	Name:  "init",
	Usage: "Initialize the eotsd home directory.",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "Path to where the home directory will be initialized",
			Value: eotscfg.DefaultEOTSDir,
		},
		cli.BoolFlag{
			Name:     forceFlag,
			Usage:    "Override existing configuration",
			Required: false,
		},
	},
	Action: initHome,
}

func initHome(c *cli.Context) error {
	homePath, err := filepath.Abs(c.String(homeFlag))
	if err != nil {
		return err
	}
	// Create home directory
	homePath = util.CleanAndExpandPath(homePath)
	force := c.Bool(forceFlag)

	if util.FileExists(homePath) && !force {
		return fmt.Errorf("home path %s already exists", homePath)
	}

	if err := util.MakeDirectory(homePath); err != nil {
		return err
	}
	// Create log directory
	logDir := eotscfg.LogDir(homePath)
	if err := util.MakeDirectory(logDir); err != nil {
		return err
	}
	// Create data directory
	dataDir := eotscfg.DataDir(homePath)
	if err := util.MakeDirectory(dataDir); err != nil {
		return err
	}

	defaultConfig := eotscfg.DefaultConfig()
	defaultConfig.DatabaseConfig.DBPath = dataDir
	fileParser := flags.NewParser(defaultConfig, flags.Default)

	return flags.NewIniParser(fileParser).WriteFile(eotscfg.ConfigFile(homePath), flags.IniIncludeComments|flags.IniIncludeDefaults)
}

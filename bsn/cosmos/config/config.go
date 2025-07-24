package config

import (
	"fmt"
	"path/filepath"

	"github.com/babylonlabs-io/babylon/v3/client/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/jessevdk/go-flags"
)

const (
	defaultConfigFileName = "fpd.conf"
	defaultLogDirname     = "logs"
	defaultLogFilename    = "fpd.log"
)

type CosmosFPConfig struct {
	Cosmwasm *CosmwasmConfig `group:"wasm" namespace:"wasm"`
	// Below configurations are needed for the Babylon client
	Common *fpcfg.Config
}

func (cfg *CosmosFPConfig) Validate() error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("rollup-node-rpc-address is required")
	}

	if cfg.Common == nil {
		return fmt.Errorf("babylon config is required")
	}
	if err := cfg.Common.Validate(); err != nil {
		return fmt.Errorf("babylon config is invalid: %w", err)
	}

	return nil
}

func (cfg *CosmosFPConfig) GetBabylonConfig() config.BabylonConfig {
	return cfg.Common.BabylonConfig.ToBabylonConfig()
}

// LoadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
//  1. Start with a default config with sane settings
//  2. Pre-parse the command line to check for an alternative config file
//  3. Load configuration file overwriting defaults with any specified options
//  4. Parse CLI options and overwrite/add any specified options
func LoadConfig(homePath string) (*CosmosFPConfig, error) {
	// The home directory is required to have a configuration file with a specific name
	// under it.
	cfgFile := fpcfg.CfgFile(homePath)
	if !util.FileExists(cfgFile) {
		return nil, fmt.Errorf("specified config file does "+
			"not exist in %s", cfgFile)
	}

	// Next, load any additional configuration options from the file.
	var cfg CosmosFPConfig
	fileParser := flags.NewParser(&cfg, flags.Default)
	err := flags.NewIniParser(fileParser).ParseFile(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Make sure everything we just loaded makes sense.
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func DefaultConfigWithHome(homePath string) CosmosFPConfig {
	cfg := fpcfg.DefaultConfigWithHome(homePath)

	return CosmosFPConfig{
		Common: &cfg,
		// TODO: default values for the rollup-fpd config
	}
}

func CfgFile(homePath string) string {
	return filepath.Join(homePath, defaultConfigFileName)
}

func LogDir(homePath string) string {
	return filepath.Join(homePath, defaultLogDirname)
}
func LogFile(homePath string) string {
	return filepath.Join(LogDir(homePath), defaultLogFilename)
}

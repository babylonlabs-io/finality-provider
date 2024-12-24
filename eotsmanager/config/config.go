package config

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/jessevdk/go-flags"

	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/util"
)

const (
	defaultLogLevel       = "debug"
	defaultDataDirname    = "data"
	defaultLogDirname     = "logs"
	defaultLogFilename    = "eotsd.log"
	defaultConfigFileName = "eotsd.conf"
	DefaultRPCPort        = 12582
	defaultKeyringBackend = keyring.BackendTest
)

var (
	// DefaultEOTSDir the default EOTS home directory:
	//   C:\Users\<username>\AppData\Local\ on Windows
	//   ~/.eotsd on Linux
	//   ~/Library/Application Support/Eotsd on MacOS
	DefaultEOTSDir = btcutil.AppDataDir("eotsd", false)

	defaultRPCListener = "127.0.0.1:" + strconv.Itoa(DefaultRPCPort)
)

type Config struct {
	LogLevel       string          `long:"loglevel" description:"Logging level for all subsystems" choice:"trace" choice:"debug" choice:"info" choice:"warn" choice:"error" choice:"fatal"`
	KeyringBackend string          `long:"keyring-type" description:"Type of keyring to use"`
	RPCListener    string          `long:"rpclistener" description:"the listener for RPC connections, e.g., 127.0.0.1:1234"`
	Metrics        *metrics.Config `group:"metrics" namespace:"metrics"`

	DatabaseConfig *DBConfig `group:"dbconfig" namespace:"dbconfig"`
}

// LoadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
//  1. Start with a default config with sane settings
//  2. Pre-parse the command line to check for an alternative config file
//  3. Load configuration file overwriting defaults with any specified options
//  4. Parse CLI options and overwrite/add any specified options
func LoadConfig(homePath string) (*Config, error) {
	// The home directory is required to have a configuration file with a specific name
	// under it.
	cfgFile := CfgFile(homePath)
	if !util.FileExists(cfgFile) {
		return nil, fmt.Errorf("specified config file does "+
			"not exist in %s", cfgFile)
	}

	// Next, load any additional configuration options from the file.
	var cfg Config
	fileParser := flags.NewParser(&cfg, flags.Default)
	err := flags.NewIniParser(fileParser).ParseFile(cfgFile)
	if err != nil {
		return nil, err
	}

	// Make sure everything we just loaded makes sense.
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate check the given configuration to be sane. This makes sure no
// illegal values or combination of values are set. All file system paths are
// normalized. The cleaned up config is returned on success.
func (cfg *Config) Validate() error {
	_, err := net.ResolveTCPAddr("tcp", cfg.RPCListener)
	if err != nil {
		return fmt.Errorf("invalid RPC listener address %s, %w", cfg.RPCListener, err)
	}

	if cfg.KeyringBackend == "" {
		return fmt.Errorf("the keyring backend should not be empty")
	}

	if cfg.Metrics == nil {
		return fmt.Errorf("empty metrics config")
	}

	if err := cfg.Metrics.Validate(); err != nil {
		return fmt.Errorf("invalid metrics config")
	}

	return nil
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

func DataDir(homePath string) string {
	return filepath.Join(homePath, defaultDataDirname)
}

func DefaultConfig() *Config {
	return DefaultConfigWithHomePath(DefaultEOTSDir)
}

func DefaultConfigWithHomePath(homePath string) *Config {
	cfg := &Config{
		LogLevel:       defaultLogLevel,
		KeyringBackend: defaultKeyringBackend,
		DatabaseConfig: DefaultDBConfigWithHomePath(homePath),
		RPCListener:    defaultRPCListener,
		Metrics:        metrics.DefaultEotsConfig(),
	}
	if err := cfg.Validate(); err != nil {
		panic(err)
	}

	return cfg
}

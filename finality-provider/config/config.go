package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/jessevdk/go-flags"
	"go.uber.org/zap/zapcore"

	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/util"
)

// Constants for config default values
const (
	defaultChainType                   = "babylon"
	defaultLogLevel                    = zapcore.DebugLevel
	defaultLogDirname                  = "logs"
	defaultLogFilename                 = "fpd.log"
	defaultFinalityProviderKeyName     = "finality-provider"
	DefaultRPCPort                     = 12581
	defaultConfigFileName              = "fpd.conf"
	defaultNumPubRand                  = 50000 // support running of roughly 5 days with block production time as 10s
	defaultBatchSubmissionSize         = 100
	defaultSubmitRetryInterval         = 1 * time.Second
	defaultSignatureSubmissionInterval = 1 * time.Second
	defaultMaxSubmissionRetries        = 20
	defaultDataDirname                 = "data"
	// TimestampingDelayBlocks is an estimation on the delay
	// for a commit to become available due to BTC timestamping protocol,
	// counted as the number of Babylon Genesis' blocks.
	// It is calculated by converting the time it takes for the public randomness
	// to be BTC timestamped counted as Bitcoin blocks to Babylon Genesis blocks.
	// As this is an estimation, we make the following assumptions:
	//    - Babylon Genesis' block time: 10s
	//    - Bitcoin's block time: 10m => 60 Babylon Genesis blocks per Bitcoin block.
	//    - Babylon Genesis' finalization delta: 300 blocks
	// The above leads to a recommended default value of 300 * 60 = 18000 Babylon blocks.
	// You should set this parameter depending on the network you connect to.
	defaultTimestampingDelayBlocks = 18000
)

// Constants for system parameters validation limits
const (
	// MaxBatchSize is the maximum allowed batch submission size
	// This constant limits BatchSubmissionSize as larger values could cause an error due to insufficient fees.
	MaxBatchSize = 100
	// MaxPubRand is the maximum allowed number of public randomness in each public randomness commit.
	// This constant limits NumPubRand as larger values could take more than 10min to generate/store
	// the randomness proofs.
	MaxPubRand = 500000 //
	// MinPubRand is the minimum allowed number of public randomness in each public randomness commit.
	// This constant limits NumPubRand to be generally large to cover inaccurate estimation of BTC block time.
	// For example, Bitcoin is running slow and by the time when the commit is timestamped, the randomness is run out.
	MinPubRand = 18000
	// RandCommitInterval is the interval between each check of whether a new commit needs to be made,
	// technically this could be a large value depending on NumPubRand, but hardcode a small value for safety
	RandCommitInterval = 30 * time.Second
)

var (
	//   C:\Users\<username>\AppData\Local\ on Windows
	//   ~/.fpd on Linux
	//   ~/Users/<username>/Library/Application Support/Fpd on MacOS
	DefaultFpdDir = btcutil.AppDataDir("fpd", false)

	defaultEOTSManagerAddress = "127.0.0.1:" + strconv.Itoa(eotscfg.DefaultRPCPort)
	DefaultRPCListener        = "127.0.0.1:" + strconv.Itoa(DefaultRPCPort)
	DefaultDataDir            = DataDir(DefaultFpdDir)
)

// Config is the main config for the fpd cli command
type Config struct {
	LogLevel string `long:"loglevel" description:"Logging level for all subsystems" choice:"trace" choice:"debug" choice:"info" choice:"warn" choice:"error" choice:"fatal"`
	// ChainType and ChainID (if any) of the chain config identify a consumer chain
	ChainType                   string        `long:"chaintype" description:"the type of the consumer chain" choice:"babylon"`
	NumPubRand                  uint32        `long:"numPubRand" description:"The number of Schnorr public randomness for each commitment"`
	TimestampingDelayBlocks     uint32        `long:"timestampingdelayblocks" description:"The delay, measured in blocks, between a randomness commit submission and the randomness is BTC-timestamped"`
	MaxSubmissionRetries        uint32        `long:"maxsubmissionretries" description:"The maximum number of retries to submit finality signature or public randomness"`
	EOTSManagerAddress          string        `long:"eotsmanageraddress" description:"The address of the remote EOTS manager; Empty if the EOTS manager is running locally"`
	HMACKey                     string        `long:"hmackey" description:"The HMAC key for authentication with EOTSD. If not provided, will use HMAC_KEY environment variable."`
	BatchSubmissionSize         uint32        `long:"batchsubmissionsize" description:"The size of a batch in one submission"`
	SubmissionRetryInterval     time.Duration `long:"submissionretryinterval" description:"The interval between each attempt to submit finality signature or public randomness after a failure"`
	SignatureSubmissionInterval time.Duration `long:"signaturesubmissioninterval" description:"The interval between each finality signature(s) submission"`

	PollerConfig *ChainPollerConfig `group:"chainpollerconfig" namespace:"chainpollerconfig"`

	DatabaseConfig *DBConfig `group:"dbconfig" namespace:"dbconfig"`

	BabylonConfig *BBNConfig `group:"babylon" namespace:"babylon"`

	RPCListener string `long:"rpclistener" description:"the listener for RPC connections, e.g., 127.0.0.1:1234"`

	Metrics *metrics.Config `group:"metrics" namespace:"metrics"`
}

func DefaultConfigWithHome(homePath string) Config {
	bbnCfg := DefaultBBNConfig()
	bbnCfg.Key = defaultFinalityProviderKeyName
	bbnCfg.KeyDirectory = homePath
	pollerCfg := DefaultChainPollerConfig()
	cfg := Config{
		ChainType:                   defaultChainType,
		LogLevel:                    defaultLogLevel.String(),
		DatabaseConfig:              DefaultDBConfigWithHomePath(homePath),
		BabylonConfig:               &bbnCfg,
		PollerConfig:                &pollerCfg,
		TimestampingDelayBlocks:     defaultTimestampingDelayBlocks,
		NumPubRand:                  defaultNumPubRand,
		BatchSubmissionSize:         defaultBatchSubmissionSize,
		SubmissionRetryInterval:     defaultSubmitRetryInterval,
		SignatureSubmissionInterval: defaultSignatureSubmissionInterval,
		MaxSubmissionRetries:        defaultMaxSubmissionRetries,
		EOTSManagerAddress:          defaultEOTSManagerAddress,
		RPCListener:                 DefaultRPCListener,
		Metrics:                     metrics.DefaultFpConfig(),
	}

	if err := cfg.Validate(); err != nil {
		panic(err)
	}

	return cfg
}

func DefaultConfig() Config {
	return DefaultConfigWithHome(DefaultFpdDir)
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

// Validate checks the given configuration to be sane. This makes sure no
// illegal values or a combination of values are set. All file system paths are
// normalized. The cleaned up config is returned on success.
func (cfg *Config) Validate() error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Validate timing configurations
	if err := cfg.validateTimingConfigs(); err != nil {
		return fmt.Errorf("timing configuration validation failed: %w", err)
	}

	// Validate batch and retry configurations
	if err := cfg.validateBatchAndRetryConfigs(); err != nil {
		return fmt.Errorf("batch and retry configuration validation failed: %w", err)
	}

	// Validate poller configuration
	if cfg.PollerConfig == nil {
		return fmt.Errorf("poller config cannot be empty")
	}

	if err := cfg.PollerConfig.Validate(); err != nil {
		return fmt.Errorf("poller configuration validation failed: %w", err)
	}

	// Validate metrics configuration
	if cfg.Metrics == nil {
		return fmt.Errorf("metrics configuration cannot be empty")
	}
	if err := cfg.Metrics.Validate(); err != nil {
		return fmt.Errorf("metrics configuration validation failed: %w", err)
	}

	return nil
}

func (cfg *Config) validateTimingConfigs() error {
	// Validate SignatureSubmissionInterval
	if cfg.SignatureSubmissionInterval <= 0 {
		return fmt.Errorf("signature submission interval must be positive, got %v", cfg.SignatureSubmissionInterval)
	}

	// Validate SubmissionRetryInterval
	if cfg.SubmissionRetryInterval <= 0 {
		return fmt.Errorf("submission retry interval must be positive, got %v", cfg.SubmissionRetryInterval)
	}

	return nil
}

func (cfg *Config) validateBatchAndRetryConfigs() error {
	// Validate BatchSubmissionSize
	if cfg.BatchSubmissionSize <= 0 {
		return fmt.Errorf("batch submission size must be positive, got %d", cfg.BatchSubmissionSize)
	}
	if cfg.BatchSubmissionSize > MaxBatchSize {
		return fmt.Errorf("batch submission size must not exceed %d, got %d", MaxBatchSize, cfg.BatchSubmissionSize)
	}

	// Validate MaxSubmissionRetries
	if cfg.MaxSubmissionRetries <= 0 {
		return fmt.Errorf("max submission retries must be positive, got %d", cfg.MaxSubmissionRetries)
	}

	// Validate NumPubRand
	if cfg.NumPubRand < MinPubRand {
		return fmt.Errorf("number of public randomness must not be less than %d, got %d", MinPubRand, cfg.NumPubRand)
	}
	if cfg.NumPubRand > MaxPubRand {
		return fmt.Errorf("number of public randomness must not exceed %d, got %d", MaxPubRand, cfg.NumPubRand)
	}
	if cfg.TimestampingDelayBlocks <= 0 {
		return fmt.Errorf("timestamping delay blocks should be positive")
	}

	return nil
}

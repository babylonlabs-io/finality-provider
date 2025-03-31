package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
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
	defaultBitcoinNetwork              = "signet"
	defaultDataDirname                 = "data"
)

// Constants for system parameters validation limits
const (
	// MaxBatchSize is the maximum allowed batch submission size,
	// this limits BatchSubmissionSize as larger values could cause error due to insufficient fees
	MaxBatchSize = 100
	// MaxPubRand is the maximum allowed number of public randomness in each commit,
	// this limits NumPubRand as larger values could take more than 10min to generate/store
	// rand proofs
	MaxPubRand = 500000 //
	// MinPubRand is the minimum allowed number of public randomness in each commit
	// a commit with less than this value will be rejected by Babylon Genesis
	MinPubRand = 8192
	// TimestampingDelayBlocks is the delay for a commit to become available due to BTC timestamping protocol
	// it is calculated by converting BTC block time (parameter w=300) to Babylon Genesis block time
	// 300 BTC blocks * 600s / 10s where 300 BTC blocks is the system parameter of BTC block time required
	// by the btc timestamping protocol. Adding 12,000 as a buffer in case 300 BTC blocks come with more than
	// 10min of average time
	TimestampingDelayBlocks = 18000 + 12000
	// RandCommitInterval is the interval between each check of whether a new commit needs to be made,
	// technically this could be a large value depending on NumPubRand, but hardcode a small value for safety
	RandCommitInterval = 30 * time.Second // Interval between check of randomness commit
)

var (
	//   C:\Users\<username>\AppData\Local\ on Windows
	//   ~/.fpd on Linux
	//   ~/Users/<username>/Library/Application Support/Fpd on MacOS
	DefaultFpdDir = btcutil.AppDataDir("fpd", false)

	defaultBTCNetParams       = chaincfg.SigNetParams
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
	MaxSubmissionRetries        uint32        `long:"maxsubmissionretries" description:"The maximum number of retries to submit finality signature or public randomness"`
	EOTSManagerAddress          string        `long:"eotsmanageraddress" description:"The address of the remote EOTS manager; Empty if the EOTS manager is running locally"`
	HMACKey                     string        `long:"hmackey" description:"The HMAC key for authentication with EOTSD. If not provided, will use HMAC_KEY environment variable."`
	BatchSubmissionSize         uint32        `long:"batchsubmissionsize" description:"The size of a batch in one submission"`
	SubmissionRetryInterval     time.Duration `long:"submissionretryinterval" description:"The interval between each attempt to submit finality signature or public randomness after a failure"`
	SignatureSubmissionInterval time.Duration `long:"signaturesubmissioninterval" description:"The interval between each finality signature(s) submission"`

	// not configurable in config file but still keep it here to allow inserting value for e2e tests
	TimestampingDelayBlocks uint32

	BitcoinNetwork string `long:"bitcoinnetwork" description:"Bitcoin network to run on" choice:"mainnet" choice:"regtest" choice:"testnet" choice:"simnet" choice:"signet"`

	BTCNetParams chaincfg.Params

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
		TimestampingDelayBlocks:     TimestampingDelayBlocks,
		NumPubRand:                  defaultNumPubRand,
		BatchSubmissionSize:         defaultBatchSubmissionSize,
		SubmissionRetryInterval:     defaultSubmitRetryInterval,
		SignatureSubmissionInterval: defaultSignatureSubmissionInterval,
		MaxSubmissionRetries:        defaultMaxSubmissionRetries,
		BitcoinNetwork:              defaultBitcoinNetwork,
		BTCNetParams:                defaultBTCNetParams,
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

	cfg.TimestampingDelayBlocks = TimestampingDelayBlocks

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

	btcNetParams, err := NetParamsBTC(cfg.BitcoinNetwork)
	if err != nil {
		return fmt.Errorf("invalid BTC network: %w", err)
	}

	cfg.BTCNetParams = btcNetParams

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

	return nil
}

// NetParamsBTC parses the BTC net params from config.
func NetParamsBTC(btcNet string) (chaincfg.Params, error) {
	switch btcNet {
	case "mainnet":
		return chaincfg.MainNetParams, nil
	case "testnet":
		return chaincfg.TestNet3Params, nil
	case "regtest":
		return chaincfg.RegressionNetParams, nil
	case "simnet":
		return chaincfg.SimNetParams, nil
	case "signet":
		return chaincfg.SigNetParams, nil
	default:
		return chaincfg.Params{}, fmt.Errorf("invalid network: %v", btcNet)
	}
}

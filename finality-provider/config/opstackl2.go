package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	cwcfg "github.com/babylonlabs-io/finality-provider/cosmwasmclient/config"

	"github.com/cosmos/btcutil/bech32"
)

type OPStackL2Config struct {
	OPStackL2RPCAddress      string `long:"opstackl2-rpc-address" description:"the rpc address of the op-stack-l2 node to connect to"`
	OPFinalityGadgetAddress  string `long:"op-finality-gadget" description:"the contract address of the op-finality-gadget"`
	BabylonFinalityGadgetRpc string `long:"babylon-finality-gadget-rpc" description:"the rpc address of babylon op finality gadget"`
	// Below configurations are needed for the Babylon client
	Key            string        `long:"key" description:"name of the babylon key to sign transactions with"`
	ChainID        string        `long:"chain-id" description:"chain id of the babylon chain to connect to"`
	RPCAddr        string        `long:"rpc-address" description:"address of the babylon rpc server to connect to"`
	GRPCAddr       string        `long:"grpc-address" description:"address of the babylon grpc server to connect to"`
	AccountPrefix  string        `long:"acc-prefix" description:"babylon account prefix to use for addresses"`
	KeyringBackend string        `long:"keyring-type" description:"type of keyring to use"`
	GasAdjustment  float64       `long:"gas-adjustment" description:"adjustment factor when using babylon gas estimation"`
	GasPrices      string        `long:"gas-prices" description:"comma separated minimum babylon gas prices to accept for transactions"`
	KeyDirectory   string        `long:"key-dir" description:"directory to store babylon keys in"`
	Debug          bool          `long:"debug" description:"flag to print debug output"`
	Timeout        time.Duration `long:"timeout" description:"client timeout when doing queries"`
	BlockTimeout   time.Duration `long:"block-timeout" description:"block timeout when waiting for block events"`
	OutputFormat   string        `long:"output-format" description:"default output when printint responses"`
	SignModeStr    string        `long:"sign-mode" description:"sign mode to use"`
}

func (cfg *OPStackL2Config) Validate() error {
	if cfg.OPStackL2RPCAddress == "" {
		return fmt.Errorf("opstackl2-rpc-address is required")
	}
	_, _, err := bech32.Decode(cfg.OPFinalityGadgetAddress, len(cfg.OPFinalityGadgetAddress))
	if err != nil {
		return fmt.Errorf("op-finality-gadget: invalid bech32 address: %w", err)
	}
	if !strings.HasPrefix(cfg.OPFinalityGadgetAddress, cfg.AccountPrefix) {
		return fmt.Errorf("op-finality-gadget: invalid address prefix: %w", err)
	}
	if cfg.BabylonFinalityGadgetRpc == "" {
		return fmt.Errorf("babylon-finality-gadget-rpc is required")
	}
	if _, err := url.Parse(cfg.RPCAddr); err != nil {
		return fmt.Errorf("rpc-addr is not correctly formatted: %w", err)
	}
	if cfg.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if cfg.BlockTimeout < 0 {
		return fmt.Errorf("block-timeout can't be negative")
	}
	return nil
}

func (cfg *OPStackL2Config) ToCosmwasmConfig() cwcfg.CosmwasmConfig {
	return cwcfg.CosmwasmConfig{
		Key:              cfg.Key,
		ChainID:          cfg.ChainID,
		RPCAddr:          cfg.RPCAddr,
		AccountPrefix:    cfg.AccountPrefix,
		KeyringBackend:   cfg.KeyringBackend,
		GasAdjustment:    cfg.GasAdjustment,
		GasPrices:        cfg.GasPrices,
		KeyDirectory:     cfg.KeyDirectory,
		Debug:            cfg.Debug,
		Timeout:          cfg.Timeout,
		BlockTimeout:     cfg.BlockTimeout,
		OutputFormat:     cfg.OutputFormat,
		SignModeStr:      cfg.SignModeStr,
		SubmitterAddress: "",
	}
}

func (cfg *OPStackL2Config) ToBBNConfig() BBNConfig {
	return BBNConfig{
		Key:            cfg.Key,
		ChainID:        cfg.ChainID,
		RPCAddr:        cfg.RPCAddr,
		AccountPrefix:  cfg.AccountPrefix,
		KeyringBackend: cfg.KeyringBackend,
		GasAdjustment:  cfg.GasAdjustment,
		GasPrices:      cfg.GasPrices,
		KeyDirectory:   cfg.KeyDirectory,
		Debug:          cfg.Debug,
		Timeout:        cfg.Timeout,
		BlockTimeout:   cfg.BlockTimeout,
		OutputFormat:   cfg.OutputFormat,
		SignModeStr:    cfg.SignModeStr,
	}
}

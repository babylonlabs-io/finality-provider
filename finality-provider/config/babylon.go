package config

import (
	"time"

	bbncfg "github.com/babylonlabs-io/babylon/client/config"
)

type BBNConfig struct {
	Key            string        `long:"key" description:"name of the key to sign transactions with"`
	ChainID        string        `long:"chain-id" description:"chain id of the chain to connect to"`
	RPCAddr        string        `long:"rpc-address" description:"address of the rpc server to connect to"`
	GRPCAddr       string        `long:"grpc-address" description:"address of the grpc server to connect to"`
	AccountPrefix  string        `long:"acc-prefix" description:"account prefix to use for addresses"`
	KeyringBackend string        `long:"keyring-type" description:"type of keyring to use"`
	GasAdjustment  float64       `long:"gas-adjustment" description:"adjustment factor when using gas estimation"`
	GasPrices      string        `long:"gas-prices" description:"comma separated minimum gas prices to accept for transactions"`
	KeyDirectory   string        `long:"key-dir" description:"directory to store keys in"`
	Debug          bool          `long:"debug" description:"flag to print debug output"`
	Timeout        time.Duration `long:"timeout" description:"client timeout when doing queries"`
	BlockTimeout   time.Duration `long:"block-timeout" description:"block timeout when waiting for block events"`
	OutputFormat   string        `long:"output-format" description:"default output when printint responses"`
	SignModeStr    string        `long:"sign-mode" description:"sign mode to use"`
}

func DefaultBBNConfig() BBNConfig {
	dc := bbncfg.DefaultBabylonConfig()
	// fill up the config from dc config
	return BBNConfig{
		Key:            dc.Key,
		ChainID:        dc.ChainID,
		RPCAddr:        dc.RPCAddr,
		GRPCAddr:       dc.GRPCAddr,
		AccountPrefix:  dc.AccountPrefix,
		KeyringBackend: dc.KeyringBackend,
		GasAdjustment:  1.5,
		GasPrices:      "0.002ubbn",
		Debug:          dc.Debug,
		Timeout:        dc.Timeout,
		// Setting this to relatively low value, out current babylon client (lens) will
		// block for this amout of time to wait for transaction inclusion in block
		BlockTimeout: 1 * time.Minute,
		OutputFormat: dc.OutputFormat,
		SignModeStr:  dc.SignModeStr,
	}
}

func BBNConfigToBabylonConfig(bc *BBNConfig) bbncfg.BabylonConfig {
	return bbncfg.BabylonConfig{
		Key:              bc.Key,
		ChainID:          bc.ChainID,
		RPCAddr:          bc.RPCAddr,
		AccountPrefix:    bc.AccountPrefix,
		KeyringBackend:   bc.KeyringBackend,
		GasAdjustment:    bc.GasAdjustment,
		GasPrices:        bc.GasPrices,
		KeyDirectory:     bc.KeyDirectory,
		Debug:            bc.Debug,
		Timeout:          bc.Timeout,
		BlockTimeout:     bc.BlockTimeout,
		OutputFormat:     bc.OutputFormat,
		SignModeStr:      bc.SignModeStr,
		SubmitterAddress: "",
	}
}

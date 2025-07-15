package config

import (
	"fmt"

	"github.com/babylonlabs-io/babylon/v3/client/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/cosmos/btcutil/bech32"
)

type RollupFPConfig struct {
	RollupNodeRPCAddress     string `long:"rollup-node-rpc-address" description:"the rpc address of the rollup node to connect to"`
	BabylonFinalityGadgetRpc string `long:"babylon-finality-gadget-rpc" description:"the rpc address of rollup finality gadget"` //nolint:stylecheck,revive
	FinalityContractAddress  string `long:"finality-contract-address" description:"the contract address of the rollup finality contract"`

	// Below configurations are needed for the Babylon client
	Common *fpcfg.Config
}

func (cfg *RollupFPConfig) Validate() error {
	if cfg.RollupNodeRPCAddress == "" {
		return fmt.Errorf("rollup-node-rpc-address is required")
	}
	_, _, err := bech32.Decode(cfg.FinalityContractAddress, len(cfg.FinalityContractAddress))
	if err != nil {
		return fmt.Errorf("finality-contract-address: invalid bech32 address: %w", err)
	}
	if cfg.Common == nil {
		return fmt.Errorf("babylon config is required")
	}
	if err := cfg.Common.Validate(); err != nil {
		return fmt.Errorf("babylon config is invalid: %w", err)
	}

	return nil
}

func (cfg *RollupFPConfig) GetBabylonConfig() config.BabylonConfig {
	return cfg.Common.BabylonConfig.ToBabylonConfig()
}

package config

import (
	"fmt"

	"github.com/babylonlabs-io/babylon/v3/client/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/cosmos/btcutil/bech32"
)

type OPStackL2Config struct {
	OPStackL2RPCAddress      string `long:"opstackl2-rpc-address" description:"the rpc address of the op-stack-l2 node to connect to"`
	OPFinalityGadgetAddress  string `long:"op-finality-gadget" description:"the contract address of the op-finality-gadget"`
	BabylonFinalityGadgetRpc string `long:"babylon-finality-gadget-rpc" description:"the rpc address of babylon op finality gadget"` //nolint:stylecheck,revive

	// Below configurations are needed for the Babylon client
	Common *fpcfg.Config
}

func (cfg *OPStackL2Config) Validate() error {
	if cfg.OPStackL2RPCAddress == "" {
		return fmt.Errorf("opstackl2-rpc-address is required")
	}
	_, _, err := bech32.Decode(cfg.OPFinalityGadgetAddress, len(cfg.OPFinalityGadgetAddress))
	if err != nil {
		return fmt.Errorf("op-finality-gadget: invalid bech32 address: %w", err)
	}
	if cfg.Common == nil {
		return fmt.Errorf("babylon config is required")
	}
	if err := cfg.Common.Validate(); err != nil {
		return fmt.Errorf("babylon config is invalid: %w", err)
	}

	return nil
}

func (cfg *OPStackL2Config) GetBabylonConfig() config.BabylonConfig {
	return cfg.Common.BabylonConfig.ToBabylonConfig()
}

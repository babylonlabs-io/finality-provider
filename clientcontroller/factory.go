package clientcontroller

import (
	"fmt"

	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/cosmwasm"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/opstackl2"
	cosmwasmcfg "github.com/babylonlabs-io/finality-provider/cosmwasmclient/config"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"go.uber.org/zap"
)

const (
	BabylonConsumerChainType   = "babylon"
	OPStackL2ConsumerChainType = "OPStackL2"
	WasmConsumerChainType      = "wasm"
)

func NewBabylonController(config *fpcfg.Config, logger *zap.Logger) (api.ClientController, error) {
	bbnConfig := fpcfg.BBNConfigToBabylonConfig(config.BabylonConfig)
	bbnClient, err := bbnclient.New(
		&bbnConfig,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Babylon rpc client: %w", err)
	}
	cc, err := babylon.NewBabylonController(bbnClient, config.BabylonConfig, &config.BTCNetParams, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Babylon rpc client: %w", err)
	}

	return cc, err
}

func NewConsumerController(config *fpcfg.Config, logger *zap.Logger) (api.ConsumerController, error) {
	var (
		ccc api.ConsumerController
		err error
	)

	switch config.ChainType {
	case BabylonConsumerChainType:
		ccc, err = babylon.NewBabylonConsumerController(config.BabylonConfig, &config.BTCNetParams, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create Babylon rpc client: %w", err)
		}
	case OPStackL2ConsumerChainType:
		ccc, err = opstackl2.NewOPStackL2ConsumerController(config.OPStackL2Config, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create OPStack L2 consumer client: %w", err)
		}
	case WasmConsumerChainType:
		wasmEncodingCfg := cosmwasmcfg.GetWasmdEncodingConfig()
		ccc, err = cosmwasm.NewCosmwasmConsumerController(config.CosmwasmConfig, wasmEncodingCfg, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create Wasm rpc client: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported consumer chain")
	}

	return ccc, err
}

package clientcontroller

import (
	"fmt"
	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"go.uber.org/zap"
)

const (
	BabylonConsumerChainType   = "babylon"
	OPStackL2ConsumerChainType = "OPStackL2"
	WasmConsumerChainType      = "wasm"
)

func NewBabylonController(bbnConfig *fpcfg.BBNConfig, logger *zap.Logger) (api.BabylonController, error) {
	bbnCfg := bbnConfig.ToBabylonConfig()
	bbnClient, err := bbnclient.New(
		&bbnCfg,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Babylon rpc client: %w", err)
	}
	cc, err := babylon.NewBabylonController(bbnClient, bbnConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Babylon rpc client: %w", err)
	}

	return cc, nil
}

func NewConsumerController(config *fpcfg.Config, logger *zap.Logger) (api.ConsumerController, error) {
	var (
		ccc api.ConsumerController
		err error
	)

	switch config.ChainType {
	case BabylonConsumerChainType:
		ccc, err = babylon.NewBabylonConsumerController(config.BabylonConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create Babylon rpc client: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported consumer chain")
	}

	return ccc, nil
}

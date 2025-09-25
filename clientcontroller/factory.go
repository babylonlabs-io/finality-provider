package clientcontroller

import (
	"fmt"
	bbnclient "github.com/babylonlabs-io/babylon/v4/client/client"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"go.uber.org/zap"
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

//go:build e2e_op
// +build e2e_op

package e2etest_op

import (
	"encoding/json"
	"testing"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	opcc "github.com/babylonlabs-io/finality-provider/clientcontroller/opstackl2"
	e2eutils "github.com/babylonlabs-io/finality-provider/itest"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// This test case will be removed by the final PR
func TestOpTestManagerSetup(t *testing.T) {
	ctm := StartOpL2ConsumerManager(t)
	defer ctm.Stop(t)

	// setup logger
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.Level(zap.DebugLevel))
	logger, err := config.Build()
	require.NoError(t, err)

	// create cosmwasm client
	cwConfig := ctm.OpConsumerController.Cfg.ToCosmwasmConfig()
	cwClient, err := opcc.NewCwClient(&cwConfig, logger)
	require.NoError(t, err)

	// query cw contract config
	queryMsg := map[string]interface{}{
		"config": struct{}{},
	}
	queryMsgBytes, err := json.Marshal(queryMsg)
	require.NoError(t, err)

	var queryConfigResponse *wasmtypes.QuerySmartContractStateResponse
	require.Eventually(t, func() bool {
		queryConfigResponse, err = cwClient.QuerySmartContractState(
			ctm.OpConsumerController.Cfg.OPFinalityGadgetAddress,
			string(queryMsgBytes),
		)
		return err == nil
	}, e2eutils.EventuallyWaitTimeOut, e2eutils.EventuallyPollTime)

	var cfgResp Config
	err = json.Unmarshal(queryConfigResponse.Data, &cfgResp)
	require.NoError(t, err)
	t.Logf("Response config query from CW contract: %+v", cfgResp)
	require.Equal(t, opConsumerChainId, cfgResp.ConsumerId)
}

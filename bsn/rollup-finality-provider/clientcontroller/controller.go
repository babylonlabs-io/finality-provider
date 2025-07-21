package clientcontroller

import (
	"context"
	"encoding/json"
	"fmt"

	sdkErr "cosmossdk.io/errors"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/babylonlabs-io/babylon/v3/client/babylonclient"
	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"
	rollupfpconfig "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

var _ api.ConsumerController = &RollupBSNController{}

// nolint:revive // Ignore stutter warning - full name provides clarity
type RollupBSNController struct {
	Cfg       *rollupfpconfig.RollupFPConfig
	ethClient *ethclient.Client
	bbnClient *bbnclient.Client
	logger    *zap.Logger
}

func NewRollupBSNController(
	rollupFPCfg *rollupfpconfig.RollupFPConfig,
	logger *zap.Logger,
) (*RollupBSNController, error) {
	if rollupFPCfg == nil {
		return nil, fmt.Errorf("nil config for rollup BSN controller")
	}
	if err := rollupFPCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ethClient, err := ethclient.Dial(rollupFPCfg.RollupNodeRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create rollup node client: %w", err)
	}

	babylonConfig := rollupFPCfg.GetBabylonConfig()
	if err := babylonConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config for Babylon client: %w", err)
	}

	bc, err := bbnclient.New(
		&babylonConfig,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Babylon client: %w", err)
	}

	cc := &RollupBSNController{
		Cfg:       rollupFPCfg,
		ethClient: ethClient,
		bbnClient: bc,
		logger:    logger,
	}

	return cc, nil
}

func (cc *RollupBSNController) QuerySmartContractState(ctx context.Context, contractAddress string, queryData string) (*wasmtypes.QuerySmartContractStateResponse, error) {
	clientCtx := client.Context{Client: cc.bbnClient.RPCClient}
	queryClient := wasmtypes.NewQueryClient(clientCtx)

	resp, err := queryClient.SmartContractState(ctx, &wasmtypes.QuerySmartContractStateRequest{
		Address:   contractAddress,
		QueryData: wasmtypes.RawContractMessage(queryData),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}

	return resp, nil
}

func (cc *RollupBSNController) ReliablySendMsg(ctx context.Context, msg sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*babylonclient.RelayerTxResponse, error) {
	return cc.reliablySendMsgs(ctx, []sdk.Msg{msg}, expectedErrs, unrecoverableErrs)
}

// queryContractConfig queries the finality contract for its config
// nolint:unused
func (cc *RollupBSNController) queryContractConfig(ctx context.Context) (*Config, error) {
	query := QueryMsg{
		Config: &Config{},
	}
	jsonData, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config query: %w", err)
	}

	stateResp, err := cc.QuerySmartContractState(ctx, cc.Cfg.FinalityContractAddress, string(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to query smart contract state: %w", err)
	}
	if len(stateResp.Data) == 0 {
		return nil, fmt.Errorf("no config found")
	}

	var resp *Config
	err = json.Unmarshal(stateResp.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config response: %w", err)
	}

	return resp, nil
}

func (cc *RollupBSNController) reliablySendMsgs(ctx context.Context, msgs []sdk.Msg, expectedErrs []*sdkErr.Error, unrecoverableErrs []*sdkErr.Error) (*babylonclient.RelayerTxResponse, error) {
	resp, err := cc.bbnClient.ReliablySendMsgs(
		ctx,
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to reliably send messages: %w", err)
	}

	return types.NewBabylonTxResponse(resp), nil
}

func (cc *RollupBSNController) Close() error {
	cc.ethClient.Close()

	if err := cc.bbnClient.Stop(); err != nil {
		return fmt.Errorf("failed to stop Babylon client: %w", err)
	}

	return nil
}

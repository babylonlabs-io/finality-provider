package client

import (
	"fmt"
	"sync"
	"time"

	wasmdparams "github.com/CosmWasm/wasmd/app/params"
	"github.com/babylonlabs-io/babylon/v3/app/params"
	"github.com/babylonlabs-io/babylon/v3/client/babylonclient"
	"github.com/babylonlabs-io/finality-provider/cosmwasmclient/config"
	"github.com/babylonlabs-io/finality-provider/cosmwasmclient/query"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"go.uber.org/zap"
)

type Client struct {
	mu sync.Mutex
	*query.QueryClient

	provider *babylonclient.CosmosProvider
	timeout  time.Duration
	logger   *zap.Logger
	cfg      *config.CosmwasmConfig
}

func New(cfg *config.CosmwasmConfig, chainName string, encodingCfg wasmdparams.EncodingConfig, logger *zap.Logger) (*Client, error) {
	var (
		zapLogger *zap.Logger
		err       error
	)

	// ensure cfg is valid
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// use the existing logger or create a new one if not given
	zapLogger = logger
	if zapLogger == nil {
		zapLogger, err = newRootLogger("console", true)
		if err != nil {
			return nil, fmt.Errorf("failed to create root logger: %w", err)
		}
	}

	provider, err := cfg.ToCosmosProviderConfig().NewProvider(
		"", // TODO: set home path
		chainName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create cosmos provider: %w", err)
	}

	cp, ok := provider.(*babylonclient.CosmosProvider)
	if !ok {
		return nil, fmt.Errorf("failed to cast provider to CosmosProvider")
	}
	cp.PCfg.KeyDirectory = cfg.KeyDirectory
	cp.Cdc = &params.EncodingConfig{
		InterfaceRegistry: encodingCfg.InterfaceRegistry,
		Codec:             encodingCfg.Codec,
		TxConfig:          encodingCfg.TxConfig,
		Amino:             encodingCfg.Amino,
	}

	// initialise Cosmos provider
	// NOTE: this will create a RPC client. The RPC client will be used for
	// submitting txs and making ad hoc queries. It won't create WebSocket
	// connection with wasmd node
	if err = cp.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize cosmos provider: %w", err)
	}

	// create a queryClient so that the Client inherits all query functions
	// TODO: merge this RPC client with the one in `cp` after Cosmos side
	// finishes the migration to new RPC client
	// see https://github.com/strangelove-ventures/cometbft-client
	c, err := rpchttp.NewWithTimeout(cp.PCfg.RPCAddr, "/websocket", uint(cfg.Timeout.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC client: %w", err)
	}
	queryClient, err := query.NewWithClient(c, cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create query client: %w", err)
	}

	return &Client{
		QueryClient: queryClient,
		provider:    cp,
		timeout:     cfg.Timeout,
		logger:      zapLogger,
		cfg:         cfg,
	}, nil
}

func (c *Client) GetConfig() *config.CosmwasmConfig {
	return c.cfg
}

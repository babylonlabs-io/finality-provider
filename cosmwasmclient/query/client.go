package query

import (
	"fmt"
	"time"

	"github.com/babylonlabs-io/finality-provider/cosmwasmclient/config"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	"github.com/cosmos/cosmos-sdk/client"
)

// QueryClient is a client that can only perform queries to a Babylon node
// It only requires `Cfg` to have `Timeout` and `RPCAddr`, but not other fields
// such as keyring, chain ID, etc..
//
//nolint:revive
type QueryClient struct {
	RPCClient rpcclient.Client
	timeout   time.Duration
}

// New creates a new QueryClient according to the given config
func New(cfg *config.WasmQueryConfig) (*QueryClient, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	tmClient, err := client.NewClientFromNode(cfg.RPCAddr)
	if err != nil {
		return nil, err
	}

	return &QueryClient{
		RPCClient: tmClient,
		timeout:   cfg.Timeout,
	}, nil
}

// NewWithClient creates a new QueryClient with a given existing rpcClient and timeout
// used by `client/` where `ChainClient` already creates an rpc client
func NewWithClient(rpcClient rpcclient.Client, timeout time.Duration) (*QueryClient, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive")
	}

	client := &QueryClient{
		RPCClient: rpcClient,
		timeout:   timeout,
	}

	return client, nil
}

func (c *QueryClient) Start() error {
	return c.RPCClient.Start()
}

func (c *QueryClient) Stop() error {
	return c.RPCClient.Stop()
}

func (c *QueryClient) IsRunning() bool {
	return c.RPCClient.IsRunning()
}

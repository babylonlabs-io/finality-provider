package query

import (
	"context"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/client"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
)

func (c *QueryClient) QueryWasm(ctx context.Context, f func(ctx context.Context, queryClient wasmtypes.QueryClient) error) error {
	clientCtx := client.Context{Client: c.RPCClient}
	queryClient := wasmtypes.NewQueryClient(clientCtx)

	return f(ctx, queryClient)
}

func (c *QueryClient) ListCodes(ctx context.Context, pagination *sdkquerytypes.PageRequest) (*wasmtypes.QueryCodesResponse, error) {
	var resp *wasmtypes.QueryCodesResponse
	err := c.QueryWasm(ctx, func(ctx context.Context, queryClient wasmtypes.QueryClient) error {
		var err error
		req := &wasmtypes.QueryCodesRequest{
			Pagination: pagination,
		}
		resp, err = queryClient.Codes(ctx, req)

		return err
	})

	return resp, err
}

func (c *QueryClient) ListContractsByCode(ctx context.Context, codeID uint64, pagination *sdkquerytypes.PageRequest) (*wasmtypes.QueryContractsByCodeResponse, error) {
	var resp *wasmtypes.QueryContractsByCodeResponse
	err := c.QueryWasm(ctx, func(ctx context.Context, queryClient wasmtypes.QueryClient) error {
		var err error
		req := &wasmtypes.QueryContractsByCodeRequest{
			CodeId:     codeID,
			Pagination: pagination,
		}
		resp, err = queryClient.ContractsByCode(ctx, req)

		return err
	})

	return resp, err
}

func (c *QueryClient) QuerySmartContractState(ctx context.Context, contractAddress string, queryData string) (*wasmtypes.QuerySmartContractStateResponse, error) {
	var resp *wasmtypes.QuerySmartContractStateResponse
	err := c.QueryWasm(ctx, func(ctx context.Context, queryClient wasmtypes.QueryClient) error {
		var err error
		req := &wasmtypes.QuerySmartContractStateRequest{
			Address:   contractAddress,
			QueryData: wasmtypes.RawContractMessage(queryData),
		}
		resp, err = queryClient.SmartContractState(ctx, req)

		return err
	})

	return resp, err
}

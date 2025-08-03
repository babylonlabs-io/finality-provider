//go:build e2e_rollup

package e2etest_rollup

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"strings"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
)

func StoreWasmCode(ctx context.Context, bbnClient *bbnclient.Client, wasmFile string) error {
	wasmCode, err := os.ReadFile(wasmFile) // #nosec G304
	if err != nil {
		return err
	}
	if strings.HasSuffix(wasmFile, "wasm") { // compress for gas limit
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, err = gz.Write(wasmCode)
		if err != nil {
			return err
		}
		err = gz.Close()
		if err != nil {
			return err
		}
		wasmCode = buf.Bytes()
	}

	storeMsg := &wasmtypes.MsgStoreCode{
		Sender:       bbnClient.MustGetAddr(),
		WASMByteCode: wasmCode,
	}
	_, err = bbnClient.ReliablySendMsg(ctx, storeMsg, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func InstantiateContract(bbnClient *bbnclient.Client, ctx context.Context, codeID uint64, initMsg []byte) error {
	instantiateMsg := &wasmtypes.MsgInstantiateContract{
		Sender: bbnClient.MustGetAddr(),
		Admin:  bbnClient.MustGetAddr(),
		CodeID: codeID,
		Label:  "cw",
		Msg:    initMsg,
		Funds:  nil,
	}

	_, err := bbnClient.ReliablySendMsg(ctx, instantiateMsg, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func ExecuteContract(ctx context.Context, bbnClient *bbnclient.Client, contractAddr string, execMsg []byte, funds []sdk.Coin) error {
	executeMsg := &wasmtypes.MsgExecuteContract{
		Sender:   bbnClient.MustGetAddr(),
		Contract: contractAddr,
		Msg:      execMsg,
		Funds:    funds,
	}

	_, err := bbnClient.ReliablySendMsg(ctx, executeMsg, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func ListCodes(ctx context.Context, bbnClient *bbnclient.Client, pagination *sdkquery.PageRequest) (*wasmtypes.QueryCodesResponse, error) {
	clientCtx := client.Context{Client: bbnClient.RPCClient}
	queryClient := wasmtypes.NewQueryClient(clientCtx)

	resp, err := queryClient.Codes(ctx, &wasmtypes.QueryCodesRequest{
		Pagination: pagination,
	})

	return resp, err
}

func ListContractsByCode(ctx context.Context, bbnClient *bbnclient.Client, codeID uint64, pagination *sdkquery.PageRequest) (*wasmtypes.QueryContractsByCodeResponse, error) {
	clientCtx := client.Context{Client: bbnClient.RPCClient}
	queryClient := wasmtypes.NewQueryClient(clientCtx)

	resp, err := queryClient.ContractsByCode(ctx, &wasmtypes.QueryContractsByCodeRequest{
		CodeId:     codeID,
		Pagination: pagination,
	})

	return resp, err
}

// returns the latest wasm code id.
func GetLatestCodeID(ctx context.Context, bbnClient *bbnclient.Client) (uint64, error) {
	pagination := &sdkquery.PageRequest{
		Limit:   1,
		Reverse: true,
	}
	resp, err := ListCodes(ctx, bbnClient, pagination)
	if err != nil {
		return 0, err
	}

	if len(resp.CodeInfos) == 0 {
		return 0, fmt.Errorf("no codes found")
	}

	return resp.CodeInfos[0].CodeID, nil
}

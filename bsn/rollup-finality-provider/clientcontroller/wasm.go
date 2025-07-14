package clientcontroller

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

func (cc *OPStackL2ConsumerController) QuerySmartContractState(ctx context.Context, contractAddress string, queryData string) (*wasmtypes.QuerySmartContractStateResponse, error) {
	clientCtx := client.Context{Client: cc.bbnClient.RPCClient}
	queryClient := wasmtypes.NewQueryClient(clientCtx)

	return queryClient.SmartContractState(ctx, &wasmtypes.QuerySmartContractStateRequest{
		Address:   contractAddress,
		QueryData: wasmtypes.RawContractMessage(queryData),
	})
}

func (cc *OPStackL2ConsumerController) StoreWasmCode(ctx context.Context, wasmFile string) error {
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
		Sender:       cc.bbnClient.MustGetAddr(),
		WASMByteCode: wasmCode,
	}
	_, err = cc.bbnClient.ReliablySendMsg(ctx, storeMsg, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func InstantiateContract(c *bbnclient.Client, ctx context.Context, codeID uint64, initMsg []byte) error {
	instantiateMsg := &wasmtypes.MsgInstantiateContract{
		Sender: c.MustGetAddr(),
		Admin:  c.MustGetAddr(),
		CodeID: codeID,
		Label:  "cw",
		Msg:    initMsg,
		Funds:  nil,
	}

	_, err := c.ReliablySendMsg(ctx, instantiateMsg, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (cc *OPStackL2ConsumerController) ExecuteContract(ctx context.Context, contractAddr string, execMsg []byte, funds []sdk.Coin) error {
	executeMsg := &wasmtypes.MsgExecuteContract{
		Sender:   cc.bbnClient.MustGetAddr(),
		Contract: contractAddr,
		Msg:      execMsg,
		Funds:    funds,
	}

	_, err := cc.bbnClient.ReliablySendMsg(ctx, executeMsg, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (cc *OPStackL2ConsumerController) ListCodes(ctx context.Context, pagination *sdkquery.PageRequest) (*wasmtypes.QueryCodesResponse, error) {
	clientCtx := client.Context{Client: cc.bbnClient.RPCClient}
	queryClient := wasmtypes.NewQueryClient(clientCtx)

	resp, err := queryClient.Codes(ctx, &wasmtypes.QueryCodesRequest{
		Pagination: pagination,
	})

	return resp, err
}

func (cc *OPStackL2ConsumerController) ListContractsByCode(ctx context.Context, codeID uint64, pagination *sdkquery.PageRequest) (*wasmtypes.QueryContractsByCodeResponse, error) {
	clientCtx := client.Context{Client: cc.bbnClient.RPCClient}
	queryClient := wasmtypes.NewQueryClient(clientCtx)

	resp, err := queryClient.ContractsByCode(ctx, &wasmtypes.QueryContractsByCodeRequest{
		CodeId:     codeID,
		Pagination: pagination,
	})

	return resp, err
}

// returns the latest wasm code id.
func (cc *OPStackL2ConsumerController) GetLatestCodeID(ctx context.Context) (uint64, error) {
	pagination := &sdkquery.PageRequest{
		Limit:   1,
		Reverse: true,
	}
	resp, err := cc.ListCodes(ctx, pagination)
	if err != nil {
		return 0, err
	}

	if len(resp.CodeInfos) == 0 {
		return 0, fmt.Errorf("no codes found")
	}

	return resp.CodeInfos[0].CodeID, nil
}

func (cc *OPStackL2ConsumerController) InstantiateContract(ctx context.Context, codeID uint64, initMsg []byte) error {
	instantiateMsg := &wasmtypes.MsgInstantiateContract{
		Sender: cc.bbnClient.MustGetAddr(),
		Admin:  cc.bbnClient.MustGetAddr(),
		CodeID: codeID,
		Label:  "cw",
		Msg:    initMsg,
		Funds:  nil,
	}

	_, err := cc.bbnClient.ReliablySendMsg(ctx, instantiateMsg, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

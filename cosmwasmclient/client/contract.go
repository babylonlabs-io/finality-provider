package client

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"strings"

	wasmdtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
)

func (c *Client) StoreWasmCode(ctx context.Context, wasmFile string) error {
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

	storeMsg := &wasmdtypes.MsgStoreCode{
		Sender:       c.MustGetAddr(),
		WASMByteCode: wasmCode,
	}
	_, err = c.ReliablySendMsg(ctx, storeMsg, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) InstantiateContract(ctx context.Context, codeID uint64, initMsg []byte) error {
	instantiateMsg := &wasmdtypes.MsgInstantiateContract{
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

func (c *Client) ExecuteContract(ctx context.Context, contractAddr string, execMsg []byte, funds []sdk.Coin) error {
	executeMsg := &wasmdtypes.MsgExecuteContract{
		Sender:   c.MustGetAddr(),
		Contract: contractAddr,
		Msg:      execMsg,
		Funds:    funds,
	}

	_, err := c.ReliablySendMsg(ctx, executeMsg, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

// returns the latest wasm code id.
func (c *Client) GetLatestCodeID(ctx context.Context) (uint64, error) {
	pagination := &sdkquery.PageRequest{
		Limit:   1,
		Reverse: true,
	}
	resp, err := c.ListCodes(ctx, pagination)
	if err != nil {
		return 0, err
	}

	if len(resp.CodeInfos) == 0 {
		return 0, fmt.Errorf("no codes found")
	}

	return resp.CodeInfos[0].CodeID, nil
}

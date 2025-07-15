package types

import (
	"encoding/json"
	"fmt"

	"github.com/babylonlabs-io/babylon/v3/client/babylonclient"
)

// TxResponse handles the transaction response in the interface ConsumerController
// Not every consumer has Events thing in their response,
// so consumer client implementations need to care about Events field.
type TxResponse struct {
	TxHash string
	Events []babylonclient.RelayerEvent
}

func NewBabylonTxResponse(resp *babylonclient.RelayerTxResponse) *babylonclient.RelayerTxResponse {
	events := make([]babylonclient.RelayerEvent, len(resp.Events))
	for i, event := range resp.Events {
		events[i] = babylonclient.RelayerEvent{
			EventType:  event.EventType,
			Attributes: event.Attributes,
		}
	}

	return &babylonclient.RelayerTxResponse{
		Height:    resp.Height,
		TxHash:    resp.TxHash,
		Events:    events,
		Codespace: resp.Codespace,
		Code:      resp.Code,
		Data:      resp.Data,
	}
}

func PrintRespJSON(resp interface{}) {
	jsonBytes, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		fmt.Println("unable to marshal response: ", err)

		return
	}

	fmt.Printf("%s\n", jsonBytes)
}

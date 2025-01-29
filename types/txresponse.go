package types

import (
	"github.com/babylonlabs-io/babylon/client/babylonclient"
	"github.com/cosmos/relayer/v2/relayer/provider"
)

// TxResponse handles the transaction response in the interface ConsumerController
// Not every consumer has Events thing in their response,
// so consumer client implementations need to care about Events field.
type TxResponse struct {
	TxHash string
	Events []babylonclient.RelayerEvent
}

func NewBabylonTxResponse(resp *provider.RelayerTxResponse) *babylonclient.RelayerTxResponse {
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

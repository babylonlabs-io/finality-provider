package types

import (
	"github.com/babylonlabs-io/babylon/v2/client/babylonclient"
)

type TxResponse struct {
	TxHash string
	Events []babylonclient.RelayerEvent
}

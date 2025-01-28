package types

import (
	"github.com/babylonlabs-io/babylon/client/babylonclient"
)

type TxResponse struct {
	TxHash string
	Events []babylonclient.RelayerEvent
}

package service

import (
	"errors"
	"fmt"

	bbntypes "github.com/babylonlabs-io/babylon/types"
)

const instanceTerminatingMsg = "terminating the finality-provider instance due to critical error"

type CriticalError struct {
	err     error
	fpBtcPk *bbntypes.BIP340PubKey
}

func (ce *CriticalError) Error() string {
	return fmt.Sprintf("critical err on finality-provider %s: %s", ce.fpBtcPk.MarshalHex(), ce.err.Error())
}

var (
	ErrFinalityProviderShutDown = errors.New("the finality provider instance is shutting down")
	ErrFinalityProviderJailed   = errors.New("the finality provider instance is jailed")
	ErrFinalityProviderSlashed  = errors.New("the finality provider instance is slashed")
)

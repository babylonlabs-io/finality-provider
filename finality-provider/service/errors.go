package service

import "errors"

var (
	ErrFinalityProviderShutDown = errors.New("the finality provider instance is shutting down")
	ErrFinalityProviderJailed   = errors.New("the finality provider instance is jailed")
	ErrFinalityProviderSlashed  = errors.New("the finality provider instance is slashed")
)

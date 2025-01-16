package clientcontroller

import (
	"errors"
	"strings"

	sdkErr "cosmossdk.io/errors"
	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	finalitytypes "github.com/babylonlabs-io/babylon/x/finality/types"
)

// these errors are considered unrecoverable because they indicate
// something critical in the finality provider program or the consumer chain
var unrecoverableErrors = []*sdkErr.Error{
	finalitytypes.ErrBlockNotFound,
	finalitytypes.ErrInvalidFinalitySig,
	finalitytypes.ErrNoPubRandYet,
	finalitytypes.ErrPubRandNotFound,
	finalitytypes.ErrTooFewPubRand,
	btcstakingtypes.ErrFpAlreadySlashed,
	btcstakingtypes.ErrFpAlreadyJailed,
}

// IsUnrecoverable returns true when the error is in the unrecoverableErrors list
func IsUnrecoverable(err error) bool {
	for _, e := range unrecoverableErrors {
		// cannot use error.Is because the unwrapped error
		// is not the expected error type
		if strings.Contains(err.Error(), e.Error()) {
			return true
		}
	}

	return false
}

type ExpectedError struct {
	error
}

func (e ExpectedError) Error() string {
	if e.error == nil {
		return "expected error"
	}

	return e.error.Error()
}

func (e ExpectedError) Unwrap() error {
	return e.error
}

// Is adds support for errors.Is usage on isExpected
func (ExpectedError) Is(err error) bool {
	_, isExpected := err.(ExpectedError)

	return isExpected
}

// Expected wraps an error in ExpectedError struct
func Expected(err error) error {
	return ExpectedError{err}
}

// IsExpected checks if error is an instance of ExpectedError
func IsExpected(err error) bool {
	return errors.Is(err, ExpectedError{})
}

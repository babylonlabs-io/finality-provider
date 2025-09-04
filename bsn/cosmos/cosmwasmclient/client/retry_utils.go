package client

import (
	"strings"
	"time"

	"cosmossdk.io/errors"
	"github.com/avast/retry-go/v4"
)

// Variables used for retries
var (
	rtyAttNum = uint(2) // this should be in config, but for patch it's okay
	rtyAtt    = retry.Attempts(rtyAttNum)
	rtyDel    = retry.Delay(time.Millisecond * 100) // this should be in config, but for patch it's okay
	rtyErr    = retry.LastErrorOnly(true)
)

func errorContained(err error, errList []*errors.Error) bool {
	for _, e := range errList {
		if strings.Contains(err.Error(), e.Error()) {
			return true
		}
	}

	return false
}

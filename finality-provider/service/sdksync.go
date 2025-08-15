package service

import (
	"flag"
	"sync"
)

// addressPrefixMutex protects concurrent access to Cosmos SDK's global address prefix configuration.
// This addresses race conditions in SetBech32PrefixForAccount/GetBech32AccountAddrPrefix
// that occur when multiple goroutines access the SDK's global config map simultaneously.
//
// The mutex is only active during testing to avoid performance impact in production.
var addressPrefixMutex sync.Mutex

// isTestMode returns true if running in test mode
func isTestMode() bool {
	return flag.Lookup("test.v") != nil
}

// LockAddressPrefix locks the SDK address prefix mutex only during testing
func LockAddressPrefix() {
	if isTestMode() {
		addressPrefixMutex.Lock()
	}
}

// UnlockAddressPrefix unlocks the SDK address prefix mutex only during testing
func UnlockAddressPrefix() {
	if isTestMode() {
		addressPrefixMutex.Unlock()
	}
}

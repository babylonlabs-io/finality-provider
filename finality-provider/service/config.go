package service

import (
	"flag"
	"sync"
)

// ConfigMutex protects concurrent access to Cosmos SDK's global configuration.
// This addresses race conditions in SetBech32PrefixForAccount/GetBech32AccountAddrPrefix
// that occur when multiple goroutines access the SDK's global config map simultaneously.
//
// The mutex is only active during testing to avoid performance impact in production.
var ConfigMutex sync.Mutex

// isTestMode returns true if running in test mode
func isTestMode() bool {
	return flag.Lookup("test.v") != nil
}

// LockConfig locks the SDK config mutex only during testing
func LockConfig() {
	if isTestMode() {
		ConfigMutex.Lock()
	}
}

// UnlockConfig unlocks the SDK config mutex only during testing
func UnlockConfig() {
	if isTestMode() {
		ConfigMutex.Unlock()
	}
}

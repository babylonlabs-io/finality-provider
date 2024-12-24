package testutil

import (
	"fmt"
	mrand "math/rand/v2"
	"net"
	"sync"
	"testing"
)

// Track allocated ports, protected by a mutex
var (
	allocatedPorts = make(map[int]struct{})
	portMutex      sync.Mutex
)

// AllocateUniquePort tries to find an available TCP port on the localhost
// by testing multiple random ports within a specified range.
func AllocateUniquePort(t *testing.T) int {
	randPort := func(base, spread int) int {
		return base + mrand.IntN(spread)
	}

	// Base port and spread range for port selection
	const (
		basePort  = 20000
		portRange = 30000
	)

	// Try up to 10 times to find an available port
	for i := 0; i < 10; i++ {
		port := randPort(basePort, portRange)

		// Lock the mutex to check and modify the shared map
		portMutex.Lock()
		if _, exists := allocatedPorts[port]; exists {
			// Port already allocated, try another one
			portMutex.Unlock()

			continue
		}

		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			portMutex.Unlock()

			continue
		}

		allocatedPorts[port] = struct{}{}
		portMutex.Unlock()

		if err := listener.Close(); err != nil {
			continue
		}

		return port
	}

	// If no available port was found, fail the test
	t.Fatalf("failed to find an available port in range %d-%d", basePort, basePort+portRange)

	return 0
}

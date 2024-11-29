//go:build e2e_op
// +build e2e_op

package e2etest_op

import (
	"testing"
)

// TestFinalityProviderLifeCycle tests the whole life cycle of a finality-provider
// creation -> registration -> randomness commitment ->
// activation with BTC delegation and Covenant sig ->
// vote submission -> block finalization
func TestFinalityProviderLifeCycle(t *testing.T) {
	ctm := StartOpL2ConsumerManager(t)
	waitForOneFinalizedBlock(t, ctm.OpSystem)
	defer ctm.Stop(t)
}

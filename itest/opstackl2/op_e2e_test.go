//go:build e2e_op
// +build e2e_op

package e2etest_op

import (
	"testing"
)

// This test case will be removed by the final PR
func TestOpTestManagerSetup(t *testing.T) {
	ctm := StartOpL2ConsumerManager(t)
	defer ctm.Stop(t)
}

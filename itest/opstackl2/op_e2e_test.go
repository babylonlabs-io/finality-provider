//go:build e2e_op
// +build e2e_op

package e2etest_op

import (
	"testing"
)

// TestFinalityGadget tests finality gadget checks if l2 blocks are finalized and process them
// - setup finality providers
// - update the finality gadget gRPC and then restart op-node
// - set enabled to true in CW contract
// - launch finality gadget
// - check l2 btc finalized block
func TestFinalityGadget(t *testing.T) {
	ctm := StartOpL2ConsumerManager(t)
	defer ctm.Stop(t)

	// ctm.setupFinalityProviders(t)
	ctm.FinalityGadgetClient.QueryBtcStakingActivatedTimestamp()

	// toggleCwKillswitch(t, ctm.OpConsumerController.CwClient, ctm.OpConsumerController.Cfg.OPFinalityGadgetAddress, true)
	// // l2BlockAfterActivation, err := ctm.OpConsumerController.QueryLatestBlockHeight()
	// require.NoError(t, err)

	// ctm.waitForOneFinalizedBlock(t)
}

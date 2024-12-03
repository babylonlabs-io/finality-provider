//go:build e2e_op
// +build e2e_op

package e2etest_op

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPubRandCommitment tests the consumer controller's functions:
// - CommitPubRandList
// - QueryLastPublicRandCommit
func TestPubRandCommitment(t *testing.T) {
	ctm := StartOpL2ConsumerManager(t)
	defer ctm.Stop(t)

	// create and register Babylon FP and OP consumer FP
	fps := ctm.setupBabylonAndConsumerFp(t)

	// send a BTC delegation and wait for activation
	consumerFpPk := fps[1]
	ctm.delegateBTCAndWaitForActivation(t, fps[0], consumerFpPk)

	// get the consumer FP instance
	consumerFpInstance := ctm.getConsumerFpInstance(t, consumerFpPk)

	// commit pub rand with start height 1
	// this will call consumer controller's CommitPubRandList function
	_, err := consumerFpInstance.CommitPubRand(1)
	require.NoError(t, err)

	// query the last pub rand
	pubRand, err := ctm.OpConsumerController.QueryLastPublicRandCommit(consumerFpPk.MustToBTCPK())
	require.NoError(t, err)
	require.NotNil(t, pubRand)

	// check the end height of the pub rand
	consumerCfg := ctm.ConsumerFpApp.GetConfig()
	require.Equal(t, uint64(consumerCfg.NumPubRand), pubRand.EndHeight())
}

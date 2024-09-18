package store_test

import (
	"math/rand"
	"testing"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/stretchr/testify/require"
)

func TestShouldStart(t *testing.T) {
	tcs := []struct {
		name           string
		currFpStatus   proto.FinalityProviderStatus
		expShouldStart bool
	}{
		{
			"Created: Should NOT start",
			proto.FinalityProviderStatus_CREATED,
			false,
		},
		{
			"Slashed: Should NOT start",
			proto.FinalityProviderStatus_SLASHED,
			false,
		},
		{
			"Inactive: Should start",
			proto.FinalityProviderStatus_INACTIVE,
			true,
		},
		{
			"Registered: Should start",
			proto.FinalityProviderStatus_REGISTERED,
			true,
		},
		{
			"Active: Should start",
			proto.FinalityProviderStatus_ACTIVE,
			true,
		},
	}

	r := rand.New(rand.NewSource(10))
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			fp := testutil.GenRandomFinalityProvider(r, t)
			fp.Status = tc.currFpStatus

			shouldStart := fp.ShouldStart()
			require.Equal(t, tc.expShouldStart, shouldStart)
		})
	}
}

func TestShouldSyncStatusFromVotingPower(t *testing.T) {
	tcs := []struct {
		name                string
		votingPowerOnChain  uint64
		currFpStatus        proto.FinalityProviderStatus
		expShouldSyncStatus bool
	}{
		{
			"zero vp and Registered: Should NOT sync",
			0,
			proto.FinalityProviderStatus_REGISTERED,
			false,
		},
		{
			"zero vp and Inactive: Should NOT sync",
			0,
			proto.FinalityProviderStatus_INACTIVE,
			false,
		},
		{
			"zero vp and Slashed: Should NOT sync",
			0,
			proto.FinalityProviderStatus_SLASHED,
			false,
		},
		{
			"zero vp and Created: Should sync",
			0,
			proto.FinalityProviderStatus_CREATED,
			true,
		},
		{
			"zero vp and Active: Should sync",
			0,
			proto.FinalityProviderStatus_ACTIVE,
			true,
		},
		{
			"some vp: Should sync",
			1,
			proto.FinalityProviderStatus_SLASHED,
			true,
		},
		{
			"some vp: Should sync from even inactive",
			1,
			proto.FinalityProviderStatus_INACTIVE,
			true,
		},
		{
			"some vp: Should sync even if it is already active",
			10,
			proto.FinalityProviderStatus_ACTIVE,
			true,
		},
	}

	r := rand.New(rand.NewSource(10))
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			fp := testutil.GenRandomFinalityProvider(r, t)
			fp.Status = tc.currFpStatus

			shouldSync := fp.ShouldSyncStatusFromVotingPower(tc.votingPowerOnChain)
			require.Equal(t, tc.expShouldSyncStatus, shouldSync)
		})
	}
}

package store_test

import (
	"math/rand"
	"os"
	"testing"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	fpstore "github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/testutil"
)

// FuzzFinalityProvidersStore tests save and list finality providers properly
func FuzzFinalityProvidersStore(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		homePath := t.TempDir()
		cfg := config.DefaultDBConfigWithHomePath(homePath)

		fpdb, err := cfg.GetDBBackend()
		require.NoError(t, err)
		vs, err := fpstore.NewFinalityProviderStore(fpdb)
		require.NoError(t, err)

		defer func() {
			err := fpdb.Close()
			require.NoError(t, err)
			err = os.RemoveAll(homePath)
			require.NoError(t, err)
		}()

		fp := testutil.GenRandomFinalityProvider(r, t)
		fpAddr, err := sdk.AccAddressFromBech32(fp.FPAddr)
		require.NoError(t, err)

		// create the fp for the first time
		err = vs.CreateFinalityProvider(
			fpAddr,
			fp.BtcPk,
			fp.Description,
			fp.Commission,
			fp.ChainID,
		)
		require.NoError(t, err)

		// create same finality provider again
		// and expect duplicate error
		err = vs.CreateFinalityProvider(
			fpAddr,
			fp.BtcPk,
			fp.Description,
			fp.Commission,
			fp.ChainID,
		)
		require.ErrorIs(t, err, fpstore.ErrDuplicateFinalityProvider)

		fpList, err := vs.GetAllStoredFinalityProviders()
		require.NoError(t, err)
		require.True(t, fp.BtcPk.IsEqual(fpList[0].BtcPk))

		actualFp, err := vs.GetFinalityProvider(fp.BtcPk)
		require.NoError(t, err)
		require.Equal(t, fp.BtcPk, actualFp.BtcPk)

		_, randomBtcPk, err := datagen.GenRandomBTCKeyPair(r)
		require.NoError(t, err)
		_, err = vs.GetFinalityProvider(randomBtcPk)
		require.ErrorIs(t, err, fpstore.ErrFinalityProviderNotFound)
	})
}

func TestUpdateFpStatusFromVotingPower(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(10))
	anyFpStatus := proto.FinalityProviderStatus(100)

	tcs := []struct {
		name               string
		fpStoredStatus     proto.FinalityProviderStatus
		votingPowerOnChain uint64
		expStatus          proto.FinalityProviderStatus
		expErr             error
	}{
		{
			"zero vp: Active to Inactive",
			proto.FinalityProviderStatus_ACTIVE,
			0,
			proto.FinalityProviderStatus_INACTIVE,
			nil,
		},
		{
			"zero vp: Registered should not update the status, but also not error out",
			proto.FinalityProviderStatus_REGISTERED,
			0,
			proto.FinalityProviderStatus_REGISTERED,
			nil,
		},
		{
			"zero vp: Slashed to Slashed",
			proto.FinalityProviderStatus_SLASHED,
			0,
			proto.FinalityProviderStatus_SLASHED,
			nil,
		},
		{
			"err: Slashed should not update status",
			proto.FinalityProviderStatus_SLASHED,
			15,
			proto.FinalityProviderStatus_SLASHED,
			nil,
		},
		{
			"vp > 0: Registered to Active",
			proto.FinalityProviderStatus_REGISTERED,
			1,
			proto.FinalityProviderStatus_ACTIVE,
			nil,
		},
		{
			"vp > 0: Inactive to Active",
			proto.FinalityProviderStatus_INACTIVE,
			1,
			proto.FinalityProviderStatus_ACTIVE,
			nil,
		},
		{
			"err: fp not found and vp > 0",
			proto.FinalityProviderStatus_INACTIVE,
			1,
			anyFpStatus,
			fpstore.ErrFinalityProviderNotFound,
		},
		{
			"err: fp not found and vp == 0 && active",
			proto.FinalityProviderStatus_ACTIVE,
			0,
			anyFpStatus,
			fpstore.ErrFinalityProviderNotFound,
		},
	}

	homePath := t.TempDir()
	cfg := config.DefaultDBConfigWithHomePath(homePath)

	fpdb, err := cfg.GetDBBackend()
	require.NoError(t, err)
	fps, err := fpstore.NewFinalityProviderStore(fpdb)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := fpdb.Close()
		require.NoError(t, err)
		err = os.RemoveAll(homePath)
		require.NoError(t, err)
	})

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fp := testutil.GenRandomFinalityProvider(r, t)
			fp.Status = tc.fpStoredStatus
			if tc.expErr == nil {
				err = fps.CreateFinalityProvider(
					sdk.MustAccAddressFromBech32(fp.FPAddr),
					fp.BtcPk,
					fp.Description,
					fp.Commission,
					fp.ChainID,
				)
				require.NoError(t, err)

				err = fps.SetFpStatus(fp.BtcPk, fp.Status)
				require.NoError(t, err)
			}

			actStatus, err := fps.UpdateFpStatusFromVotingPower(tc.votingPowerOnChain, fp)
			if tc.expErr != nil {
				require.EqualError(t, err, tc.expErr.Error())

				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expStatus, actStatus)

			storedFp, err := fps.GetFinalityProvider(fp.BtcPk)
			require.NoError(t, err)
			require.Equal(t, tc.expStatus, storedFp.Status)
		})
	}
}

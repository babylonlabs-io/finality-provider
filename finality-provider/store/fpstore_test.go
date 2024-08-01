package store_test

import (
	"math/rand"
	"os"
	"testing"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/finality-provider/finality-provider/config"
	fpstore "github.com/babylonchain/finality-provider/finality-provider/store"
	"github.com/babylonchain/finality-provider/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// FuzzFinalityProvidersStore tests save and list finality providers properly
func FuzzFinalityProvidersStore(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homePath := t.TempDir()
		cfg := config.DefaultDBConfigWithHomePath(homePath)

		fpdb, err := cfg.GetDbBackend()
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
			fp.KeyName,
			fp.ChainID,
			fp.Pop.BtcSig,
		)
		require.NoError(t, err)

		// create same finality provider again
		// and expect duplicate error
		err = vs.CreateFinalityProvider(
			fpAddr,
			fp.BtcPk,
			fp.Description,
			fp.Commission,
			fp.KeyName,
			fp.ChainID,
			fp.Pop.BtcSig,
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

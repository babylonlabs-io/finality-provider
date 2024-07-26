package store_test

import (
	"math/rand"
	"os"
	"testing"

	"github.com/babylonchain/babylon/testutil/datagen"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/store"
	"github.com/babylonlabs-io/finality-provider/testutil"
)

// FuzzEOTSStore tests save and show EOTS key names properly
func FuzzEOTSStore(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homePath := t.TempDir()
		cfg := config.DefaultDBConfigWithHomePath(homePath)

		dbBackend, err := cfg.GetDbBackend()
		require.NoError(t, err)

		vs, err := store.NewEOTSStore(dbBackend)
		require.NoError(t, err)

		defer func() {
			dbBackend.Close()
			err := os.RemoveAll(homePath)
			require.NoError(t, err)
		}()

		expectedKeyName := testutil.GenRandomHexStr(r, 10)
		_, btcPk, err := datagen.GenRandomBTCKeyPair(r)
		require.NoError(t, err)

		// add key name for the first time
		err = vs.AddEOTSKeyName(
			btcPk,
			expectedKeyName,
		)
		require.NoError(t, err)

		// add duplicate key name
		err = vs.AddEOTSKeyName(
			btcPk,
			expectedKeyName,
		)
		require.ErrorIs(t, err, store.ErrDuplicateEOTSKeyName)

		keyNameFromDb, err := vs.GetEOTSKeyName(schnorr.SerializePubKey(btcPk))
		require.NoError(t, err)
		require.Equal(t, expectedKeyName, keyNameFromDb)

		_, randomBtcPk, err := datagen.GenRandomBTCKeyPair(r)
		require.NoError(t, err)
		_, err = vs.GetEOTSKeyName(schnorr.SerializePubKey(randomBtcPk))
		require.ErrorIs(t, err, store.ErrEOTSKeyNameNotFound)
	})
}

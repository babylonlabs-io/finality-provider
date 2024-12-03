package store_test

import (
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/proto"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/stretchr/testify/require"
	"math/rand"
	"os"
	"testing"
	"time"

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

		dbBackend, err := cfg.GetDBBackend()
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

		keyNameFromDB, err := vs.GetEOTSKeyName(schnorr.SerializePubKey(btcPk))
		require.NoError(t, err)
		require.Equal(t, expectedKeyName, keyNameFromDB)

		_, randomBtcPk, err := datagen.GenRandomBTCKeyPair(r)
		require.NoError(t, err)
		_, err = vs.GetEOTSKeyName(schnorr.SerializePubKey(randomBtcPk))
		require.ErrorIs(t, err, store.ErrEOTSKeyNameNotFound)
	})
}

// FuzzSignStore tests save sing records
func FuzzSignStore(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homePath := t.TempDir()
		cfg := config.DefaultDBConfigWithHomePath(homePath)

		dbBackend, err := cfg.GetDBBackend()
		require.NoError(t, err)

		vs, err := store.NewEOTSStore(dbBackend)
		require.NoError(t, err)

		defer func() {
			dbBackend.Close()
			err := os.RemoveAll(homePath)
			require.NoError(t, err)
		}()

		expectedRecord := proto.SigningRecord{
			BlockHash: testutil.GenRandomByteArray(r, 32),
			PublicKey: testutil.GenRandomByteArray(r, 33),
			Signature: testutil.GenRandomByteArray(r, 32),
			Timestamp: time.Now().UnixMilli(),
		}
		expectedHeight := r.Uint64()

		// save for the first time
		err = vs.SaveSignRecord(
			expectedHeight,
			expectedRecord.BlockHash,
			expectedRecord.PublicKey,
			expectedRecord.Signature,
		)
		require.NoError(t, err)

		// try to save the record at the same height
		err = vs.SaveSignRecord(
			expectedHeight,
			expectedRecord.BlockHash,
			expectedRecord.PublicKey,
			expectedRecord.Signature,
		)
		require.ErrorIs(t, err, store.ErrDuplicateSignRecord)

		signRecordFromDB, err := vs.GetSignRecord(expectedHeight)
		require.NoError(t, err)
		require.Equal(t, expectedRecord.PublicKey, signRecordFromDB.PublicKey)
		require.Equal(t, expectedRecord.BlockHash, signRecordFromDB.BlockHash)
		require.Equal(t, expectedRecord.Signature, signRecordFromDB.Signature)

		rndHeight := r.Uint64()
		_, err = vs.GetSignRecord(rndHeight)
		require.ErrorIs(t, err, store.ErrSignRecordNotFound)
	})
}

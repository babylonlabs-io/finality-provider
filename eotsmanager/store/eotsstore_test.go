package store_test

import (
	"bytes"
	"math/rand"
	"os"
	"testing"

	"github.com/babylonlabs-io/babylon/v2/testutil/datagen"
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
		t.Parallel()
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

// FuzzSignStore tests save sign records
func FuzzSignStore(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
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

		expectedHeight := r.Uint64()
		pk := testutil.GenRandomByteArray(r, 32)
		msg := testutil.GenRandomByteArray(r, 32)
		eotsSig := testutil.GenRandomByteArray(r, 32)

		chainID := []byte("test-chain")
		// save for the first time
		err = vs.SaveSignRecord(
			expectedHeight,
			chainID,
			msg,
			pk,
			eotsSig,
		)
		require.NoError(t, err)

		// try to save the record at the same height
		err = vs.SaveSignRecord(
			expectedHeight,
			chainID,
			msg,
			pk,
			eotsSig,
		)
		require.ErrorIs(t, err, store.ErrDuplicateSignRecord)

		signRecordFromDB, found, err := vs.GetSignRecord(pk, chainID, expectedHeight)
		require.True(t, found)
		require.NoError(t, err)
		require.Equal(t, msg, signRecordFromDB.Msg)
		require.Equal(t, eotsSig, signRecordFromDB.Signature)

		rndHeight := r.Uint64()
		_, found, err = vs.GetSignRecord(pk, chainID, rndHeight)
		require.NoError(t, err)
		require.False(t, found)
	})
}

// FuzzListKeysEOTSStore tests getting all keys from store
func FuzzListKeysEOTSStore(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		homePath := t.TempDir()
		cfg := config.DefaultDBConfigWithHomePath(homePath)

		dbBackend, err := cfg.GetDBBackend()
		require.NoError(t, err)

		vs, err := store.NewEOTSStore(dbBackend)
		require.NoError(t, err)

		defer func() {
			dbBackend.Close()
		}()

		expected := make(map[string][]byte)
		for i := 0; i < r.Intn(10); i++ {
			expectedKeyName := testutil.GenRandomHexStr(r, 10)
			_, btcPk, err := datagen.GenRandomBTCKeyPair(r)
			require.NoError(t, err)
			expected[expectedKeyName] = schnorr.SerializePubKey(btcPk)

			err = vs.AddEOTSKeyName(
				btcPk,
				expectedKeyName,
			)
			require.NoError(t, err)
		}

		keys, err := vs.GetAllEOTSKeyNames()
		require.NoError(t, err)

		for keyName, btcPk := range expected {
			gotBtcPk, ok := keys[keyName]
			require.True(t, ok)

			parsedGot, err := schnorr.ParsePubKey(gotBtcPk)
			require.NoError(t, err)
			parsedExpected, err := schnorr.ParsePubKey(btcPk)
			require.NoError(t, err)

			require.Equal(t, parsedExpected, parsedGot)
		}
	})
}

// FuzzDeleteSignRecordsFromHeight tests deleting sign records from a specific height
func FuzzDeleteSignRecordsFromHeight(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		homePath := t.TempDir()
		cfg := config.DefaultDBConfigWithHomePath(homePath)

		dbBackend, err := cfg.GetDBBackend()
		require.NoError(t, err)

		vs, err := store.NewEOTSStore(dbBackend)
		require.NoError(t, err)

		t.Cleanup(func() {
			dbBackend.Close()
			err := os.RemoveAll(homePath)
			require.NoError(t, err)
		})

		chainID := []byte("test-chain")

		// Generate a fixed set of public keys to use
		numPks := 3 + r.Intn(3) // Between 3-5 different public keys
		publicKeys := make([][]byte, numPks)
		for i := range publicKeys {
			publicKeys[i] = testutil.GenRandomByteArray(r, 32)
		}

		// Generate a series of records with different heights and public keys
		numRecords := 10 + r.Intn(20) // Between 10-30 records
		records := make([]struct {
			height  uint64
			pk      []byte
			msg     []byte
			eotsSig []byte
		}, numRecords)

		// Create records with ascending heights and random public keys from our set
		baseHeight := r.Uint64() % 1000 // Start with a random base height
		for i := 0; i < numRecords; i++ {
			records[i].height = baseHeight + uint64(i*100)      // Ensure heights are well-spaced
			records[i].pk = publicKeys[r.Intn(len(publicKeys))] // Pick a random PK from our set
			records[i].msg = testutil.GenRandomByteArray(r, 32)
			records[i].eotsSig = testutil.GenRandomByteArray(r, 32)

			// Save record
			err = vs.SaveSignRecord(
				records[i].height,
				chainID,
				records[i].msg,
				records[i].pk,
				records[i].eotsSig,
			)
			require.NoError(t, err)
		}

		// Verify all records were saved
		for _, record := range records {
			signRecordFromDB, found, err := vs.GetSignRecord(record.pk, chainID, record.height)
			require.True(t, found)
			require.NoError(t, err)
			require.Equal(t, record.msg, signRecordFromDB.Msg)
			require.Equal(t, record.eotsSig, signRecordFromDB.Signature)
		}

		// Choose a specific public key to delete records for
		targetPkIndex := r.Intn(len(publicKeys))
		targetPk := publicKeys[targetPkIndex]

		// Find a cutoff height - ensure it's a height we have records for
		cutoffHeight := records[numRecords/2].height

		// Delete records for the specific public key from the cutoff height
		err = vs.DeleteSignRecordsFromHeight(targetPk, chainID, cutoffHeight)
		require.NoError(t, err)

		// Verify the correct records were deleted
		for _, record := range records {
			signRecordFromDB, found, err := vs.GetSignRecord(record.pk, chainID, record.height)
			require.NoError(t, err)

			if bytes.Equal(record.pk, targetPk) && record.height >= cutoffHeight {
				// This record should be deleted
				require.False(t, found, "Record with targetPk at height %d should be deleted", record.height)
			} else {
				// Other records should still exist
				require.True(t, found, "Record with non-targetPk or below cutoff height %d should exist", record.height)
				require.Equal(t, record.msg, signRecordFromDB.Msg)
				require.Equal(t, record.eotsSig, signRecordFromDB.Signature)
			}
		}

		// Test edge case: delete non-existent records
		nonExistentPk := testutil.GenRandomByteArray(r, 32)
		nonExistentHeight := baseHeight + uint64(numRecords*100) + 1000
		err = vs.DeleteSignRecordsFromHeight(nonExistentPk, chainID, nonExistentHeight)
		require.NoError(t, err)

		// Delete remaining records for the target public key
		err = vs.DeleteSignRecordsFromHeight(targetPk, chainID, 0)
		require.NoError(t, err)

		// Verify all records for targetPk are now deleted
		for _, record := range records {
			signRecordFromDB, found, err := vs.GetSignRecord(record.pk, chainID, record.height)
			require.NoError(t, err)

			if bytes.Equal(record.pk, targetPk) {
				require.False(t, found, "Record with targetPk should be deleted")
			} else {
				require.True(t, found, "Record with different pk should still exist")
				require.Equal(t, record.msg, signRecordFromDB.Msg)
				require.Equal(t, record.eotsSig, signRecordFromDB.Signature)
			}
		}

		// Test with nil arguments - should return error
		err = vs.DeleteSignRecordsFromHeight(nil, chainID, 0)
		require.Error(t, err)
		err = vs.DeleteSignRecordsFromHeight(targetPk, nil, 0)
		require.Error(t, err)
	})
}

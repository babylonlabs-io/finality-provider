package store_test

import (
	"math/rand"
	"os"
	"testing"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
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

// FuzzDeleteSignRecordsInHeightRange tests deleting sign records within a specific height range
func FuzzDeleteSignRecordsInHeightRange(f *testing.F) {
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

		chainID := []byte("test-chain")

		// Generate a series of records with different heights
		numRecords := 20 + r.Intn(30) // Between 20-50 records
		records := make([]struct {
			height  uint64
			pk      []byte
			msg     []byte
			eotsSig []byte
		}, numRecords)

		// Create records with ascending heights
		var baseHeight uint64 = r.Uint64() % 1000 // Start with a random base height
		for i := 0; i < numRecords; i++ {
			records[i].height = baseHeight + uint64(i*50) // Ensure heights are well-spaced
			records[i].pk = testutil.GenRandomByteArray(r, 32)
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

		// Test various range configurations
		testRanges := []struct {
			name       string
			fromHeight uint64
			toHeight   uint64
			shouldKeep func(height uint64) bool
		}{
			{
				name:       "Middle range",
				fromHeight: records[numRecords/4].height,
				toHeight:   records[numRecords/2].height,
				shouldKeep: func(height uint64) bool {
					return height < records[numRecords/4].height || height > records[numRecords/2].height
				},
			},
			{
				name:       "Delete all above",
				fromHeight: records[numRecords/2].height,
				toHeight:   0, // Delete all records from fromHeight and above
				shouldKeep: func(height uint64) bool {
					return height < records[numRecords/2].height
				},
			},
			{
				name:       "Delete below threshold",
				fromHeight: 0,
				toHeight:   records[numRecords/3].height,
				shouldKeep: func(height uint64) bool {
					return height > records[numRecords/3].height
				},
			},
			{
				name:       "Empty range (fromHeight > toHeight)",
				fromHeight: records[numRecords/2].height,
				toHeight:   records[numRecords/4].height, // This is lower than fromHeight
				shouldKeep: func(height uint64) bool {
					// Should keep all records since the range is invalid
					return true
				},
			},
			{
				name:       "Exact single record",
				fromHeight: records[numRecords/2].height,
				toHeight:   records[numRecords/2].height,
				shouldKeep: func(height uint64) bool {
					return height != records[numRecords/2].height
				},
			},
		}

		// Run tests for each range configuration
		for _, testRange := range testRanges {
			t.Run(testRange.name, func(t *testing.T) {
				// First, ensure all records exist by recreating them if needed
				for _, record := range records {
					// Try to get the record first
					_, found, err := vs.GetSignRecord(record.pk, chainID, record.height)
					require.NoError(t, err)

					// If it doesn't exist, recreate it
					if !found {
						err = vs.SaveSignRecord(
							record.height,
							chainID,
							record.msg,
							record.pk,
							record.eotsSig,
						)
						require.NoError(t, err)
					}
				}

				// Delete records in the specified range
				err = vs.DeleteSignRecordsInHeightRange(testRange.fromHeight, testRange.toHeight)
				require.NoError(t, err)

				// Check that records outside the range still exist and those inside are deleted
				for _, record := range records {
					signRecordFromDB, found, err := vs.GetSignRecord(record.pk, chainID, record.height)
					require.NoError(t, err)

					if testRange.shouldKeep(record.height) {
						require.True(t, found, "Record at height %d should exist for test: %s",
							record.height, testRange.name)
						require.Equal(t, record.msg, signRecordFromDB.Msg)
						require.Equal(t, record.eotsSig, signRecordFromDB.Signature)
					} else {
						require.False(t, found, "Record at height %d should be deleted for test: %s",
							record.height, testRange.name)
					}
				}
			})
		}
		
		// Case 1: Delete non-existent range
		nonExistentStart := baseHeight + uint64(numRecords*100)
		err = vs.DeleteSignRecordsInHeightRange(nonExistentStart, nonExistentStart+100)
		require.NoError(t, err)

		// Case 2: Delete all records
		err = vs.DeleteSignRecordsInHeightRange(0, baseHeight+uint64(numRecords*100))
		require.NoError(t, err)

		// Verify all records are now deleted
		for _, record := range records {
			_, found, err := vs.GetSignRecord(record.pk, chainID, record.height)
			require.NoError(t, err)
			require.False(t, found, "Record at height %d should be deleted after deleting all", record.height)
		}
	})
}

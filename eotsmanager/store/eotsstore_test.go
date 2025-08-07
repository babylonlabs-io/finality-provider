package store_test

import (
	"bytes"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"

	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
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
			if err := dbBackend.Close(); err != nil {
				t.Errorf("Error closing database: %v", err)
			}
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
		require.ErrorIs(t, err, store.ErrDuplicateEOTSKeyRecord)

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
			if err := dbBackend.Close(); err != nil {
				t.Errorf("Error closing database: %v", err)
			}
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
			if err := dbBackend.Close(); err != nil {
				t.Errorf("Error closing database: %v", err)
			}
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
			if err := dbBackend.Close(); err != nil {
				t.Errorf("Error closing database: %v", err)
			}
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

// FuzzEOTSStore_BackupWithConcurrentWrites performs fuzz testing for EOTSStore's backup functionality under concurrent writes.
// Ensures that the backup contains only keys written up to the point the backup is initiated, despite ongoing writes.
// Validates the integrity of written keys in the original DB and verifies backup consistency with the expected state.
func FuzzEOTSStore_BackupWithConcurrentWrites(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	type keyPair struct {
		btcPk *btcec.PublicKey
		index int64
	}
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		var (
			wg         sync.WaitGroup
			allKeysMu  sync.Mutex
			writeIndex atomic.Int64
			writeLimit = 100 + r.Intn(201)
			writeSleep = 5 * time.Millisecond
			allKeys    = map[string]keyPair{}
		)

		homePath := t.TempDir()
		cfg := config.DefaultDBConfigWithHomePath(homePath)
		dbBackend, err := cfg.GetDBBackend()
		require.NoError(t, err)

		vs, err := store.NewEOTSStore(dbBackend)
		require.NoError(t, err)

		t.Cleanup(func() {
			if err := dbBackend.Close(); err != nil {
				t.Errorf("Error closing database: %v", err)
			}
		})

		// Write initial key
		initialKeyName := testutil.GenRandomHexStr(r, 10)
		_, initialPk, err := datagen.GenRandomBTCKeyPair(r)
		require.NoError(t, err)

		err = vs.AddEOTSKeyName(initialPk, initialKeyName)
		require.NoError(t, err)

		// Start concurrent writes
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < writeLimit; i++ {
				idx := writeIndex.Add(1)

				kname := testutil.GenRandomHexStr(r, 10)
				_, pk, err := datagen.GenRandomBTCKeyPair(r)
				require.NoError(t, err)

				err = vs.AddEOTSKeyName(pk, kname)
				require.NoError(t, err)

				allKeysMu.Lock()
				allKeys[kname] = keyPair{
					btcPk: pk,
					index: idx,
				}
				allKeysMu.Unlock()

				time.Sleep(writeSleep)
			}
		}()

		sleepMs := 100 + r.Intn(201) // 100 to 300 ms
		// Allow writes to accumulate
		time.Sleep(time.Duration(sleepMs) * time.Millisecond)

		// Capture write index before backup starts
		indexBeforeBackup := writeIndex.Load()

		// Perform backup
		backupHome := t.TempDir()
		backupPath := fmt.Sprintf("%s/data", backupHome)
		dbPath := fmt.Sprintf("%s/data/eots.db", homePath)

		bkpName, err := vs.BackupDB(dbPath, backupPath)
		require.NoError(t, err)

		wg.Wait()

		// Verify the original DB has all keys
		for name, pair := range allKeys {
			got, err := vs.GetEOTSKeyName(schnorr.SerializePubKey(pair.btcPk))
			require.NoError(t, err)
			require.Equal(t, name, got)
		}

		// Open and check backup DB
		cfgBkp := config.DefaultDBConfigWithHomePath(backupHome)
		cfgBkp.DBPath = backupPath
		cfgBkp.DBFileName = bkpName
		dbBackendBkp, err := cfgBkp.GetDBBackend()
		require.NoError(t, err)

		t.Cleanup(func() {
			if err := dbBackendBkp.Close(); err != nil {
				t.Errorf("Error closing backup database: %v", err)
			}
		})

		vsBkp, err := store.NewEOTSStore(dbBackendBkp)
		require.NoError(t, err)

		found := 0
		foundBeforeCutoff := 0
		for name, pair := range allKeys {
			val, err := vsBkp.GetEOTSKeyName(schnorr.SerializePubKey(pair.btcPk))
			if err == nil {
				require.Equal(t, name, val)
				found++
				if pair.index <= indexBeforeBackup {
					foundBeforeCutoff++
				}
			}
		}

		t.Logf("Total keys written: %d", writeLimit)
		t.Logf("Backup cutoff index: %d", indexBeforeBackup)
		t.Logf("Keys found in backup: %d", found)
		t.Logf("Keys expected before cutoff: %d", foundBeforeCutoff)

		require.Greater(t, found, 0, "Backup should have at least some keys")
		require.Equal(t, foundBeforeCutoff, found, "Backup should only contain keys written before cutoff")
	})
}

// TestEOTSStore_BackupTime tests the backup time of the EOTSStore with various sizes of data populated in the database.
func TestEOTSStore_BackupTime(t *testing.T) {
	t.Parallel()

	sizes := []struct {
		name  string
		count int
	}{
		{"S", 1000},
		{"M", 50000},
		{"L", 150000},
		{"XL", 1000000},
	}

	for _, size := range sizes {
		t.Run(size.name, func(t *testing.T) {
			t.Parallel()
			homePath := t.TempDir()
			cfg := config.DefaultDBConfigWithHomePath(homePath)
			dbBackend, err := cfg.GetDBBackend()
			require.NoError(t, err)

			defer func() {
				if err := dbBackend.Close(); err != nil {
					t.Errorf("Error closing database: %v", err)
				}
			}()

			vs, err := store.NewEOTSStore(dbBackend)
			require.NoError(t, err)

			chainID := []byte("test-chain")
			t.Logf("Populating database with %d entries...", size.count)
			wg := sync.WaitGroup{}
			type record struct {
				pk     []byte
				height uint64
			}
			allRecordsMu := sync.Mutex{}
			var allRecords []record
			for i := 0; i < size.count; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					r := rand.New(rand.NewSource(time.Now().UnixNano()))
					pk := testutil.GenRandomByteArray(r, 32)
					msg := testutil.GenRandomByteArray(r, 32)
					sig := testutil.GenRandomByteArray(r, 32)

					err := vs.SaveSignRecord(uint64(i), chainID, msg, pk, sig)
					require.NoError(t, err)

					allRecordsMu.Lock()
					allRecords = append(allRecords, record{
						pk:     pk,
						height: uint64(i),
					})
					allRecordsMu.Unlock()
					time.Sleep(10 * time.Millisecond)
				}()
			}
			wg.Wait()
			t.Logf("Database populated")

			// grind a couple of backups
			const iterations = 3
			for i := 0; i < iterations; i++ {
				backupHome := t.TempDir()
				backupPath := fmt.Sprintf("%s/data", backupHome)
				dbPath := fmt.Sprintf("%s/data/eots.db", homePath)

				startTime := time.Now()
				_, err := vs.BackupDB(dbPath, backupPath)
				duration := time.Since(startTime)
				require.NoError(t, err)
				t.Logf("Backup %d took: %v", i+1, duration)
				var totalSize int64
				err = filepath.WalkDir(backupPath, func(_ string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if !d.IsDir() {
						info, err := d.Info()
						if err != nil {
							return fmt.Errorf("failed to get info: %w", err)
						}
						totalSize += info.Size()
					}

					return nil
				})
				require.NoError(t, err)
				t.Logf("Backup %d size: %.2f MB", i+1, float64(totalSize)/(1024*1024))
			}

			backupHome := t.TempDir()
			backupPath := fmt.Sprintf("%s/data", backupHome)
			dbPath := fmt.Sprintf("%s/data/eots.db", homePath)

			bkpName, err := vs.BackupDB(dbPath, backupPath)
			require.NoError(t, err)

			cfgBkp := config.DefaultDBConfigWithHomePath(backupHome)
			cfgBkp.DBPath = backupPath
			cfgBkp.DBFileName = bkpName
			dbBackendBkp, err := cfgBkp.GetDBBackend()
			if err != nil {
				t.Fatalf("Failed to open backup DB: %v", err)
			}
			defer func() {
				if err := dbBackendBkp.Close(); err != nil {
					t.Errorf("Error closing backup database: %v", err)
				}
			}()

			vsBkp, err := store.NewEOTSStore(dbBackendBkp)
			if err != nil {
				t.Fatalf("Failed to create backup store: %v", err)
			}

			for _, r := range allRecords {
				_, exists, err := vsBkp.GetSignRecord(r.pk, chainID, r.height)
				require.NoError(t, err)
				require.True(t, exists, "Record should exist in backup")
			}
		})
	}
}

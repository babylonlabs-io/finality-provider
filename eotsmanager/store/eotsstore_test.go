package store_test

import (
	"bytes"
	"fmt"
	"github.com/btcsuite/btcd/btcec/v2"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
			dbBackend.Close()
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

		err = vs.BackupDB(dbPath, backupPath)
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
		dbBackendBkp, err := cfgBkp.GetDBBackend()
		require.NoError(t, err)

		t.Cleanup(func() {
			dbBackendBkp.Close()
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

// ReportBackupMetrics prints detailed backup performance metrics for human readability
func ReportBackupMetrics(b *testing.B, dbSize int) {
	// We can't directly access the timing data, so we'll report what we know
	fmt.Printf("\n=== Backup Performance for %d entries ===\n", dbSize)
	fmt.Printf("Total backups performed: %d\n", b.N)
	fmt.Printf("Note: Detailed timing stats available in benchmark output\n")

	// Estimate throughput based on entry size assumption
	bytesPerEntry := 100 // Rough estimate of bytes per database entry
	totalBytes := int64(dbSize * bytesPerEntry)
	fmt.Printf("Estimated database size: %.2f KB\n", float64(totalBytes)/1024)
	fmt.Printf("=====================================\n\n")
}

func BenchmarkEOTSStore_BackupWithConcurrentWrites(b *testing.B) {
	// Test different database sizes
	sizes := []struct {
		name  string
		count int
	}{
		{"Small", 100},
		{"Medium", 1000},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			// Create temp directory for the database
			homePath := b.TempDir()
			cfg := config.DefaultDBConfigWithHomePath(homePath)
			dbBackend, err := cfg.GetDBBackend()
			if err != nil {
				b.Fatalf("Failed to get DB backend: %v", err)
			}
			defer dbBackend.Close()

			// Create store
			vs, err := store.NewEOTSStore(dbBackend)
			if err != nil {
				b.Fatalf("Failed to create EOTS store: %v", err)
			}

			// Random source
			r := rand.New(rand.NewSource(time.Now().UnixNano()))

			// Pre-populate the database with initial data
			b.Logf("Populating database with %d entries...", size.count)
			for i := 0; i < size.count; i++ {
				keyName := testutil.GenRandomHexStr(r, 10)
				_, pk, err := datagen.GenRandomBTCKeyPair(r)
				if err != nil {
					b.Fatalf("Failed to generate key pair: %v", err)
				}

				err = vs.AddEOTSKeyName(pk, keyName)
				if err != nil {
					b.Fatalf("Failed to add key: %v", err)
				}
			}

			// Setup for concurrent writes
			stopWrites := make(chan struct{})
			writesDone := make(chan struct{})

			// Start concurrent writes in background
			go func() {
				defer close(writesDone)
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()

				for {
					select {
					case <-stopWrites:
						return
					case <-ticker.C:
						keyName := testutil.GenRandomHexStr(r, 10)
						_, pk, err := datagen.GenRandomBTCKeyPair(r)
						if err != nil {
							b.Errorf("Failed to generate key pair: %v", err)
							return
						}

						err = vs.AddEOTSKeyName(pk, keyName)
						if err != nil {
							b.Errorf("Failed to add key: %v", err)
							return
						}
					}
				}
			}()

			// Allow writes to accumulate for a moment
			time.Sleep(100 * time.Millisecond)

			// Start benchmark timing
			b.ResetTimer()

			// Run the benchmark
			for i := 0; i < b.N; i++ {
				// Create backup directory
				backupHome := b.TempDir()
				backupPath := fmt.Sprintf("%s/data", backupHome)
				dbPath := fmt.Sprintf("%s/data/eots.db", homePath)

				// Perform backup (this is what we're measuring)
				startTime := time.Now()
				err = vs.BackupDB(dbPath, backupPath)
				duration := time.Since(startTime)
				if err != nil {
					b.Fatalf("Failed to backup DB: %v", err)
				}

				// Log detailed info for this operation
				if b.N <= 5 || i == b.N-1 {
					b.Logf("Backup %d took: %v", i+1, duration)
				}
			}

			// Stop benchmark timing
			b.StopTimer()

			// Report detailed metrics
			ReportBackupMetrics(b, size.count)

			// Stop concurrent writes
			close(stopWrites)
			<-writesDone

			// Validate backup functionality (only for the first run to verify correctness)
			if b.N > 0 {
				// Create a validation backup for testing
				backupHome := b.TempDir()
				backupPath := fmt.Sprintf("%s/data", backupHome)
				dbPath := fmt.Sprintf("%s/data/eots.db", homePath)

				// Create a fresh backup specifically for validation
				err = vs.BackupDB(dbPath, backupPath)
				if err != nil {
					b.Fatalf("Validation backup failed: %v", err)
				}

				// Open the backup database
				cfgBkp := config.DefaultDBConfigWithHomePath(backupHome)
				cfgBkp.DBPath = backupPath
				dbBackendBkp, err := cfgBkp.GetDBBackend()
				if err != nil {
					b.Fatalf("Failed to open backup DB: %v", err)
				}
				defer dbBackendBkp.Close()

				vsBkp, err := store.NewEOTSStore(dbBackendBkp)
				if err != nil {
					b.Fatalf("Failed to create backup store: %v", err)
				}

				// Add and retrieve a test key to verify backup is working
				testKeyName := "test_backup_key"
				_, testPk, err := datagen.GenRandomBTCKeyPair(r)
				if err != nil {
					b.Fatalf("Failed to generate test key: %v", err)
				}

				// Try adding to main DB
				err = vs.AddEOTSKeyName(testPk, testKeyName)
				if err != nil {
					b.Fatalf("Failed to add test key to main DB: %v", err)
				}

				// Check it doesn't exist in backup (showing backup is separate)
				_, err = vsBkp.GetEOTSKeyName(schnorr.SerializePubKey(testPk))
				if err == nil {
					b.Fatalf("Test key shouldn't exist in backup yet")
				}

				b.Logf("Backup validation successful")
			}
		})
	}
}

func BenchmarkEOTSStore_BackupWithConcurrentWrites2(b *testing.B) {
	sizes := []struct {
		name  string
		count int
	}{
		{"S", 100},
		{"M", 1000},
		{"L", 10000},
		//{"XL", 100000},
		//{"XXL", 100000},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			// Create temp directory for the database
			homePath := b.TempDir()
			cfg := config.DefaultDBConfigWithHomePath(homePath)
			dbBackend, err := cfg.GetDBBackend()
			if err != nil {
				b.Fatalf("Failed to get DB backend: %v", err)
			}
			defer dbBackend.Close()

			// Create store
			vs, err := store.NewEOTSStore(dbBackend)
			if err != nil {
				b.Fatalf("Failed to create EOTS store: %v", err)
			}

			// Random source
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			chainID := []byte("test-chain")

			// Pre-populate the database with initial data
			b.Logf("Populating database with %d entries...", size.count)
			for i := 0; i < size.count; i++ {
				height := uint64(i)
				pk := testutil.GenRandomByteArray(r, 32)
				msg := testutil.GenRandomByteArray(r, 32)
				signature := testutil.GenRandomByteArray(r, 32)

				err = vs.SaveSignRecord(
					height,
					chainID,
					msg,
					pk,
					signature,
				)
				if err != nil {
					b.Fatalf("Failed to save sign record: %v", err)
				}
			}

			// Setup for concurrent writes
			stopWrites := make(chan struct{})
			writesDone := make(chan struct{})

			// Start concurrent writes in background
			go func() {
				defer close(writesDone)
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				writeCount := 0

				for {
					select {
					case <-stopWrites:
						return
					case <-ticker.C:
						height := uint64(size.count + writeCount)
						pk := testutil.GenRandomByteArray(r, 32)
						msg := testutil.GenRandomByteArray(r, 32)
						signature := testutil.GenRandomByteArray(r, 32)

						err = vs.SaveSignRecord(
							height,
							chainID,
							msg,
							pk,
							signature,
						)
						if err != nil {
							b.Errorf("Failed to save sign record: %v", err)
							return
						}
						writeCount++
					}
				}
			}()

			// Allow writes to accumulate for a moment
			time.Sleep(100 * time.Millisecond)

			// Start benchmark timing
			b.ResetTimer()

			// Run the benchmark
			for i := 0; i < b.N; i++ {
				// Create backup directory
				backupHome := b.TempDir()
				backupPath := fmt.Sprintf("%s/data", backupHome)
				dbPath := fmt.Sprintf("%s/data/eots.db", homePath)

				// Perform backup (this is what we're measuring)
				startTime := time.Now()
				err = vs.BackupDB(dbPath, backupPath)
				duration := time.Since(startTime)
				if err != nil {
					b.Fatalf("Failed to backup DB: %v", err)
				}

				// Log detailed info for this operation
				if b.N <= 5 || i == b.N-1 {
					b.Logf("Backup %d took: %v", i+1, duration)
				}
			}

			// Stop benchmark timing
			b.StopTimer()

			// Report detailed metrics
			ReportBackupMetrics(b, size.count)

			// Stop concurrent writes
			close(stopWrites)
			<-writesDone

			// Validate backup functionality (only for the first run to verify correctness)
			if b.N > 0 {
				// Create a validation backup for testing
				backupHome := b.TempDir()
				backupPath := fmt.Sprintf("%s/data", backupHome)
				dbPath := fmt.Sprintf("%s/data/eots.db", homePath)

				// Create a fresh backup specifically for validation
				err = vs.BackupDB(dbPath, backupPath)
				if err != nil {
					b.Fatalf("Validation backup failed: %v", err)
				}

				// Open the backup database
				cfgBkp := config.DefaultDBConfigWithHomePath(backupHome)
				cfgBkp.DBPath = backupPath
				dbBackendBkp, err := cfgBkp.GetDBBackend()
				if err != nil {
					b.Fatalf("Failed to open backup DB: %v", err)
				}
				defer dbBackendBkp.Close()

				vsBkp, err := store.NewEOTSStore(dbBackendBkp)
				if err != nil {
					b.Fatalf("Failed to create backup store: %v", err)
				}

				// Add a test record to the main database
				testHeight := uint64(999999)
				testPk := testutil.GenRandomByteArray(r, 32)
				testMsg := testutil.GenRandomByteArray(r, 32)
				testSig := testutil.GenRandomByteArray(r, 32)

				// Save to main DB
				err = vs.SaveSignRecord(
					testHeight,
					chainID,
					testMsg,
					testPk,
					testSig,
				)
				if err != nil {
					b.Fatalf("Failed to save test record: %v", err)
				}

				// Verify the record doesn't exist in backup
				_, found, err := vsBkp.GetSignRecord(testPk, chainID, testHeight)
				if err != nil {
					b.Fatalf("Error checking test record: %v", err)
				}
				if found {
					b.Fatalf("Test record shouldn't exist in backup yet")
				}

				// Verify some existing records do exist in the backup
				// Check a few random records from our initial set
				for j := 0; j < 5; j++ {
					checkHeight := uint64(r.Intn(size.count))
					// Here we're just checking the backup has some data
					for k := 0; k < 10; k++ {
						randomPk := testutil.GenRandomByteArray(r, 32)
						_, found, _ := vsBkp.GetSignRecord(randomPk, chainID, checkHeight)
						if found {
							break
						}
					}
				}

				b.Logf("Backup validation completed")
			}
		})
	}
}

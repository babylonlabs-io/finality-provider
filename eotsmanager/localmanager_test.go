package eotsmanager_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babylonlabs-io/babylon/crypto/eots"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/types"
	"github.com/babylonlabs-io/finality-provider/testutil"
)

// FuzzCreateKey tests the creation of an EOTS key
func FuzzCreateKey(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		fpName := testutil.GenRandomHexStr(r, 4)
		homeDir := filepath.Join(t.TempDir(), "eots-home")
		eotsCfg := eotscfg.DefaultConfigWithHomePath(homeDir)
		dbBackend, err := eotsCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)
		defer func() {
			dbBackend.Close()
			err := os.RemoveAll(homeDir)
			require.NoError(t, err)
		}()

		lm, err := eotsmanager.NewLocalEOTSManager(homeDir, eotsCfg.KeyringBackend, dbBackend, zap.NewNop())
		require.NoError(t, err)

		fpPk, err := lm.CreateKey(fpName)
		require.NoError(t, err)

		fpRecord, err := lm.KeyRecord(fpPk)
		require.NoError(t, err)
		require.Equal(t, fpName, fpRecord.Name)

		sig, err := lm.SignSchnorrSig(fpPk, datagen.GenRandomByteArray(r, 32))
		require.NoError(t, err)
		require.NotNil(t, sig)

		_, err = lm.CreateKey(fpName)
		require.ErrorIs(t, err, types.ErrFinalityProviderAlreadyExisted)
	})
}

func FuzzCreateRandomnessPairList(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		fpName := testutil.GenRandomHexStr(r, 4)
		homeDir := filepath.Join(t.TempDir(), "eots-home")
		eotsCfg := eotscfg.DefaultConfigWithHomePath(homeDir)
		dbBackend, err := eotsCfg.DatabaseConfig.GetDBBackend()
		defer func() {
			dbBackend.Close()
			err := os.RemoveAll(homeDir)
			require.NoError(t, err)
		}()
		require.NoError(t, err)
		lm, err := eotsmanager.NewLocalEOTSManager(homeDir, eotsCfg.KeyringBackend, dbBackend, zap.NewNop())
		require.NoError(t, err)

		fpPk, err := lm.CreateKey(fpName)
		require.NoError(t, err)

		chainID := datagen.GenRandomByteArray(r, 10)
		startHeight := datagen.RandomInt(r, 100)
		num := r.Intn(10) + 1
		pubRandList, err := lm.CreateRandomnessPairList(fpPk, chainID, startHeight, uint32(num))
		require.NoError(t, err)
		require.Len(t, pubRandList, num)

		for i := 0; i < num; i++ {
			sig, err := lm.SignEOTS(fpPk, chainID, datagen.GenRandomByteArray(r, 32), startHeight+uint64(i))
			require.NoError(t, err)
			require.NotNil(t, sig)
		}
	})
}

func FuzzSignRecord(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homeDir := filepath.Join(t.TempDir(), "eots-home")
		eotsCfg := eotscfg.DefaultConfigWithHomePath(homeDir)
		dbBackend, err := eotsCfg.DatabaseConfig.GetDBBackend()
		defer func() {
			dbBackend.Close()
			err := os.RemoveAll(homeDir)
			require.NoError(t, err)
		}()
		require.NoError(t, err)
		lm, err := eotsmanager.NewLocalEOTSManager(homeDir, eotsCfg.KeyringBackend, dbBackend, zap.NewNop())
		require.NoError(t, err)

		startHeight := datagen.RandomInt(r, 100)
		numRand := r.Intn(10) + 1

		msg := datagen.GenRandomByteArray(r, 32)
		numFps := 3
		for i := 0; i < numFps; i++ {
			chainID := datagen.GenRandomByteArray(r, 10)
			fpName := testutil.GenRandomHexStr(r, 4)
			fpPk, err := lm.CreateKey(fpName)
			require.NoError(t, err)
			pubRandList, err := lm.CreateRandomnessPairList(fpPk, chainID, startHeight, uint32(numRand))
			require.NoError(t, err)
			require.Len(t, pubRandList, numRand)

			sig, err := lm.SignEOTS(fpPk, chainID, msg, startHeight)
			require.NoError(t, err)
			require.NotNil(t, sig)

			eotsPk, err := bbntypes.NewBIP340PubKey(fpPk)
			require.NoError(t, err)

			err = eots.Verify(eotsPk.MustToBTCPK(), pubRandList[0], msg, sig)
			require.NoError(t, err)

			// we expect return from db
			sig2, err := lm.SignEOTS(fpPk, chainID, msg, startHeight)
			require.NoError(t, err)
			require.Equal(t, sig, sig2)

			err = eots.Verify(eotsPk.MustToBTCPK(), pubRandList[0], msg, sig2)
			require.NoError(t, err)

			// same height diff msg
			_, err = lm.SignEOTS(fpPk, chainID, datagen.GenRandomByteArray(r, 32), startHeight)
			require.ErrorIs(t, err, types.ErrDoubleSign)
		}
	})
}

// TestKeyAliasingAttackPrevented verifies that the public key verification fix
// prevents the key aliasing attack where an attacker maps their PK to a victim's keyName.
func TestKeyAliasingAttackPrevented(t *testing.T) {
	t.Parallel()

	homeDir := filepath.Join(t.TempDir(), "eots-home")
	eotsCfg := eotscfg.DefaultConfigWithHomePath(homeDir)
	dbBackend, err := eotsCfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)
	defer func() {
		dbBackend.Close()
		os.RemoveAll(homeDir)
	}()

	lm, err := eotsmanager.NewLocalEOTSManager(homeDir, eotsCfg.KeyringBackend, dbBackend, zap.NewNop())
	require.NoError(t, err)

	// Step 1: Victim creates a legitimate EOTS key
	victimKeyName := "victim-fp-key"
	victimPkBytes, err := lm.CreateKey(victimKeyName)
	require.NoError(t, err)

	// Step 2: Attacker generates their own keypair (not stored in keyring)
	attackerPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	attackerPkBytes := schnorr.SerializePubKey(attackerPrivKey.PubKey())

	// Step 3: Attacker exploits SaveEOTSKeyName to map their PK to victim's keyName
	// This simulates the unauthenticated gRPC call (HMAC bypass vulnerability)
	err = lm.SaveEOTSKeyName(attackerPrivKey.PubKey(), victimKeyName)
	require.NoError(t, err)

	// Step 4: Setup - create randomness for victim (as would happen in normal operation)
	chainID := []byte("test-chain")
	height := uint64(100)
	_, err = lm.CreateRandomnessPairList(victimPkBytes, chainID, height, 1)
	require.NoError(t, err)

	// Step 5: Attacker attempts to sign using their PK (which maps to victim's keyName)
	// WITHOUT THE FIX: This would succeed and use victim's private key
	// WITH THE FIX: This should fail with "public key mismatch" error
	msg := []byte("attacker-controlled-message")
	_, err = lm.SignEOTS(attackerPkBytes, chainID, msg, height)

	// Verify the fix blocks the attack
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "public key mismatch"),
		"Expected 'public key mismatch' error, got: %v", err)

	// Verify legitimate victim signing still works
	_, err = lm.SignEOTS(victimPkBytes, chainID, msg, height)
	require.NoError(t, err)
}

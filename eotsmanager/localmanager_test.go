package eotsmanager_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonchain/finality-provider/eotsmanager"
	eotscfg "github.com/babylonchain/finality-provider/eotsmanager/config"
	"github.com/babylonchain/finality-provider/eotsmanager/types"
	"github.com/babylonchain/finality-provider/testutil"
)

var (
	passphrase = "testpass"
	hdPath     = ""
)

// FuzzCreateKey tests the creation of an EOTS key
func FuzzCreateKey(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		fpName := testutil.GenRandomHexStr(r, 4)
		homeDir := filepath.Join(t.TempDir(), "eots-home")
		eotsCfg := eotscfg.DefaultConfigWithHomePath(homeDir)
		dbBackend, err := eotsCfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)
		defer func() {
			dbBackend.Close()
			err := os.RemoveAll(homeDir)
			require.NoError(t, err)
		}()

		lm, err := eotsmanager.NewLocalEOTSManager(homeDir, eotsCfg.KeyringBackend, dbBackend, zap.NewNop())
		require.NoError(t, err)

		fpPk, err := lm.CreateKey(fpName, passphrase, hdPath)
		require.NoError(t, err)

		fpRecord, err := lm.KeyRecord(fpPk, passphrase)
		require.NoError(t, err)
		require.Equal(t, fpName, fpRecord.Name)

		sig, err := lm.SignSchnorrSig(fpPk, datagen.GenRandomByteArray(r, 32), passphrase)
		require.NoError(t, err)
		require.NotNil(t, sig)

		_, err = lm.CreateKey(fpName, passphrase, hdPath)
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
		dbBackend, err := eotsCfg.DatabaseConfig.GetDbBackend()
		defer func() {
			dbBackend.Close()
			err := os.RemoveAll(homeDir)
			require.NoError(t, err)
		}()
		require.NoError(t, err)
		lm, err := eotsmanager.NewLocalEOTSManager(homeDir, eotsCfg.KeyringBackend, dbBackend, zap.NewNop())
		require.NoError(t, err)

		fpPk, err := lm.CreateKey(fpName, passphrase, hdPath)
		require.NoError(t, err)

		chainID := datagen.GenRandomByteArray(r, 10)
		startHeight := datagen.RandomInt(r, 100)
		num := r.Intn(10) + 1
		pubRandList, err := lm.CreateRandomnessPairList(fpPk, chainID, startHeight, uint32(num), passphrase)
		require.NoError(t, err)
		require.Len(t, pubRandList, num)

		for i := 0; i < num; i++ {
			sig, err := lm.SignEOTS(fpPk, chainID, datagen.GenRandomByteArray(r, 32), startHeight+uint64(i), passphrase)
			require.NoError(t, err)
			require.NotNil(t, sig)
		}
	})
}

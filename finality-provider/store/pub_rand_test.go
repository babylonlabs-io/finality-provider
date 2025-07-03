package store_test

import (
	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	"github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/testutil"
	"github.com/cometbft/cometbft/crypto/merkle"
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
)

// FuzzRemoveMerkleProof removal of proofs
func FuzzRemoveMerkleProof(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		homePath := t.TempDir()
		cfg := config.DefaultDBConfigWithHomePath(homePath)

		db, err := cfg.GetDBBackend()
		require.NoError(t, err)
		vs, err := store.NewPubRandProofStore(db)
		require.NoError(t, err)

		numPubRand := uint64(r.Intn(1000))
		chainID := []byte("test-chain")
		rl, err := datagen.GenRandomPubRandList(r, numPubRand)
		require.NoError(t, err)
		fp := testutil.GenRandomFinalityProvider(r, t)

		defer func() {
			err := db.Close()
			require.NoError(t, err)
		}()

		startHeight := uint64(1)
		err = vs.AddPubRandProofList(chainID, fp.GetBIP340BTCPK().MustMarshal(), startHeight, numPubRand, rl.ProofList)
		require.NoError(t, err)

		targetHeight := uint64(r.Intn(1000))
		err = vs.RemovePubRandProofList(chainID, fp.GetBIP340BTCPK().MustMarshal(), targetHeight)
		require.NoError(t, err)

		_, err = vs.GetPubRandProofList(chainID, fp.GetBIP340BTCPK().MustMarshal(), startHeight, targetHeight)
		require.ErrorIs(t, err, store.ErrPubRandProofNotFound)
	})
}
func FuzzAddDuplicateMerkleProof(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		homePath := t.TempDir()
		cfg := config.DefaultDBConfigWithHomePath(homePath)

		db, err := cfg.GetDBBackend()
		require.NoError(t, err)
		vs, err := store.NewPubRandProofStore(db)
		require.NoError(t, err)

		defer func() {
			err := db.Close()
			require.NoError(t, err)
		}()

		// Generate random test data
		numPubRand := uint64(r.Intn(100) + 5) // Ensure reasonable number of elements
		chainID := []byte("test-chain")
		rl, err := datagen.GenRandomPubRandList(r, numPubRand)
		require.NoError(t, err)
		fp := testutil.GenRandomFinalityProvider(r, t)
		startHeight := uint64(1)
		pk := fp.GetBIP340BTCPK().MustMarshal()

		// First insertion - should succeed
		err = vs.AddPubRandProofList(chainID, pk, startHeight, numPubRand, rl.ProofList)
		require.NoError(t, err)

		// Create a second list with the same proofs
		secondProofList := make([]*merkle.Proof, len(rl.ProofList))
		copy(secondProofList, rl.ProofList)

		// Second insertion of the same proofs
		err = vs.AddPubRandProofList(chainID, pk, startHeight, numPubRand, secondProofList)
		require.NoError(t, err)

		// Verify we can retrieve all proofs
		for i := uint64(0); i < numPubRand; i++ {
			height := startHeight + i
			storedBytes, err := vs.GetPubRandProof(chainID, pk, height)
			require.NoError(t, err)
			require.NotNil(t, storedBytes)

			// Verify the proof matches what we expect
			originalBytes, err := rl.ProofList[i].ToProto().Marshal()
			require.NoError(t, err)

			require.Equal(t, originalBytes, storedBytes,
				"Proof at height %d doesn't match expected value", height)
		}

		// Now try with a different set of proofs at the same heights
		// This will verify that we're actually replacing values
		differentProofList, err := datagen.GenRandomPubRandList(r, numPubRand)
		require.NoError(t, err)

		// Add the different proofs at the same heights
		err = vs.AddPubRandProofList(chainID, pk, startHeight, numPubRand, differentProofList.ProofList)
		require.NoError(t, err)

		// Check if the proofs were actually replaced
		for i := uint64(0); i < numPubRand; i++ {
			height := startHeight + i
			storedBytes, err := vs.GetPubRandProof(chainID, pk, height)
			require.NoError(t, err)

			expectedProof := differentProofList.ProofList[i]

			originalBytes, err := expectedProof.ToProto().Marshal()
			require.NoError(t, err)

			require.Equal(t, originalBytes, storedBytes,
				"Proof at height %d wasn't replaced with new value", height)
		}
	})
}

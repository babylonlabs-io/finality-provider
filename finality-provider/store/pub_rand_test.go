package store_test

import (
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/testutil"
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

package keyring_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/stretchr/testify/require"

	eotscfg "github.com/babylonlabs-io/finality-provider/eotsmanager/config"

	fpkr "github.com/babylonlabs-io/finality-provider/keyring"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/testutil"
)

var (
	passphrase = "testpass"
	hdPath     = ""
)

// FuzzCreatePoP tests the creation of PoP
func FuzzCreatePoP(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		keyName := testutil.GenRandomHexStr(r, 4)
		sdkCtx := testutil.GenSdkContext(r, t)

		kc, err := fpkr.NewChainKeyringController(sdkCtx, keyName, keyring.BackendTest)
		require.NoError(t, err)

		eotsHome := filepath.Join(t.TempDir(), "eots-home")
		eotsCfg := eotscfg.DefaultConfigWithHomePath(eotsHome)
		dbBackend, err := eotsCfg.DatabaseConfig.GetDBBackend()
		require.NoError(t, err)
		em, err := eotsmanager.NewLocalEOTSManager(eotsHome, eotsCfg.KeyringBackend, dbBackend, zap.NewNop())
		defer func() {
			dbBackend.Close()
			err := os.RemoveAll(eotsHome)
			require.NoError(t, err)
		}()
		require.NoError(t, err)

		btcPkBytes, err := em.CreateKey(keyName, passphrase, hdPath)
		require.NoError(t, err)
		btcPk, err := types.NewBIP340PubKey(btcPkBytes)
		require.NoError(t, err)
		keyInfo, err := kc.CreateChainKey(passphrase, hdPath, "")
		require.NoError(t, err)

		fpAddr := keyInfo.AccAddress
		fpRecord, err := em.KeyRecord(btcPk.MustMarshal(), passphrase)
		require.NoError(t, err)
		pop, err := kc.CreatePop(fpAddr, fpRecord.PrivKey)
		require.NoError(t, err)
		err = pop.Verify(fpAddr, btcPk, &chaincfg.SimNetParams)
		require.NoError(t, err)
	})
}

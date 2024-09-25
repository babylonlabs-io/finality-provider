package testutil

import (
	"encoding/hex"
	"math/rand"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/crypto/eots"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/babylonlabs-io/finality-provider/finality-provider/store"

	sdkmath "cosmossdk.io/math"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/types"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/codec"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/types"
)

func GenRandomByteArray(r *rand.Rand, length uint64) []byte {
	newHeaderBytes := make([]byte, length)
	r.Read(newHeaderBytes)
	return newHeaderBytes
}

func GenRandomHexStr(r *rand.Rand, length uint64) string {
	randBytes := GenRandomByteArray(r, length)
	return hex.EncodeToString(randBytes)
}

func RandomDescription(r *rand.Rand) *stakingtypes.Description {
	des := stakingtypes.NewDescription(GenRandomHexStr(r, 10), "", "", "", "")
	return &des
}

func AddRandomSeedsToFuzzer(f *testing.F, num uint) {
	// Seed based on the current time
	r := rand.New(rand.NewSource(time.Now().Unix()))
	var idx uint
	for idx = 0; idx < num; idx++ {
		f.Add(r.Int63())
	}
}

func GenPublicRand(r *rand.Rand, t *testing.T) *bbn.SchnorrPubRand {
	_, eotsPR, err := eots.RandGen(r)
	require.NoError(t, err)
	return bbn.NewSchnorrPubRandFromFieldVal(eotsPR)
}

func GenRandomFinalityProvider(r *rand.Rand, t *testing.T) *store.StoredFinalityProvider {
	// generate BTC key pair
	btcSK, btcPK, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)
	bip340PK := bbn.NewBIP340PubKeyFromBTCPK(btcPK)

	fpAddr, err := sdk.AccAddressFromBech32(datagen.GenRandomAccount().Address)
	require.NoError(t, err)

	// generate and verify PoP, correct case
	pop, err := bstypes.NewPoPBTC(fpAddr, btcSK)
	require.NoError(t, err)
	err = pop.Verify(fpAddr, bip340PK, &chaincfg.SimNetParams)
	require.NoError(t, err)

	return &store.StoredFinalityProvider{
		FPAddr:      fpAddr.String(),
		KeyName:     GenRandomHexStr(r, 4),
		ChainID:     "chain-test",
		BtcPk:       bip340PK.MustToBTCPK(),
		Description: RandomDescription(r),
		Commission:  ZeroCommissionRate(),
		Pop: &proto.ProofOfPossession{
			BtcSig: pop.BtcSig,
		},
	}
}

func GenValidSlashingRate(r *rand.Rand) sdkmath.LegacyDec {
	return sdkmath.LegacyNewDecWithPrec(int64(datagen.RandomInt(r, 41)+10), 2)
}

func GenBlocks(r *rand.Rand, startHeight, endHeight uint64) []*types.BlockInfo {
	blocks := make([]*types.BlockInfo, 0)
	for i := startHeight; i <= endHeight; i++ {
		b := &types.BlockInfo{
			Height: i,
			Hash:   datagen.GenRandomByteArray(r, 32),
		}
		blocks = append(blocks, b)
	}

	return blocks
}

// GenStoredFinalityProvider generates a random finality-provider from the keyring and store it in DB
func GenStoredFinalityProvider(r *rand.Rand, t *testing.T, app *service.FinalityProviderApp, passphrase, hdPath string, eotsPk *bbntypes.BIP340PubKey) *store.StoredFinalityProvider {
	// generate keyring
	keyName := GenRandomHexStr(r, 4)
	chainID := GenRandomHexStr(r, 4)

	cfg := app.GetConfig()
	_, err := service.CreateChainKey(cfg.BabylonConfig.KeyDirectory, cfg.BabylonConfig.ChainID, keyName, keyring.BackendTest, passphrase, hdPath, "")
	require.NoError(t, err)

	res, err := app.CreateFinalityProvider(keyName, chainID, passphrase, hdPath, eotsPk, RandomDescription(r), ZeroCommissionRate())
	require.NoError(t, err)

	btcPk, err := bbn.NewBIP340PubKeyFromHex(res.FpInfo.BtcPkHex)
	require.NoError(t, err)
	storedFp, err := app.GetFinalityProviderStore().GetFinalityProvider(btcPk.MustToBTCPK())
	require.NoError(t, err)

	return storedFp
}

func GenSdkContext(r *rand.Rand, t *testing.T) client.Context {
	chainID := "testchain-" + GenRandomHexStr(r, 4)
	dir := t.TempDir()
	return client.Context{}.
		WithChainID(chainID).
		WithCodec(codec.MakeCodec()).
		WithKeyringDir(dir)
}

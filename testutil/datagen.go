package testutil

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	sdkmath "cosmossdk.io/math"
	"github.com/babylonlabs-io/babylon/v4/crypto/eots"
	"github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/v4/types"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	fpkr "github.com/babylonlabs-io/finality-provider/keyring"

	"github.com/babylonlabs-io/finality-provider/codec"
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
	_, btcPK, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)
	bip340PK := bbn.NewBIP340PubKeyFromBTCPK(btcPK)

	fpAddr, err := sdk.AccAddressFromBech32(datagen.GenRandomAccount().Address)
	require.NoError(t, err)

	commRates := ZeroCommissionRate()
	zeroRate := commRates.Rate

	return &store.StoredFinalityProvider{
		FPAddr:      fpAddr.String(),
		ChainID:     "chain-test",
		BtcPk:       bip340PK.MustToBTCPK(),
		Description: RandomDescription(r),
		Commission:  &zeroRate,
		CommissionInfo: &proto.CommissionInfo{
			MaxRate:       commRates.MaxRate.String(),
			MaxChangeRate: commRates.MaxChangeRate.String(),
			UpdateTime:    timestamppb.New(time.Now().Add(-25 * time.Hour).UTC()),
		},
	}
}

func GenValidSlashingRate(r *rand.Rand) sdkmath.LegacyDec {
	return sdkmath.LegacyNewDecWithPrec(int64(datagen.RandomInt(r, 41)+10), 2)
}

func GenBlocksDesc(r *rand.Rand, startHeight, endHeight uint64) []types.BlockDescription {
	blocks := make([]types.BlockDescription, 0)
	for i := startHeight; i <= endHeight; i++ {
		blocks = append(blocks, types.NewBlockInfo(i, datagen.GenRandomByteArray(r, 32), false))
	}

	return blocks
}

func CreateChainKey(keyringDir, chainID, keyName, backend, passphrase, hdPath, mnemonic string) (*types.ChainKeyInfo, error) {
	sdkCtx, err := fpkr.CreateClientCtx(
		keyringDir, chainID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create client ctx: %w", err)
	}

	krController, err := fpkr.NewChainKeyringController(
		sdkCtx,
		keyName,
		backend,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create chain keyring controller: %w", err)
	}

	info, err := krController.CreateChainKey(passphrase, hdPath, mnemonic)
	if err != nil {
		return nil, fmt.Errorf("failed to create chain key: %w", err)
	}

	return info, nil
}

func GenSdkContext(r *rand.Rand, t *testing.T) client.Context {
	chainID := "testchain-" + GenRandomHexStr(r, 4)
	dir := t.TempDir()

	return client.Context{}.
		WithChainID(chainID).
		WithCodec(codec.MakeCodec()).
		WithKeyringDir(dir)
}

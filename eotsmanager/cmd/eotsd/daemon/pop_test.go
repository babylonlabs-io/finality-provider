package daemon_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/cmd/eotsd/daemon"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cometbft/cometbft/crypto/tmhash"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

var hardcodedPopToVerify daemon.PoPExport = daemon.PoPExport{
	EotsPublicKey: "3d0bebcbe800236ce8603c5bb1ab6c2af0932e947db4956a338f119797c37f1e",
	BabyPublicKey: "A0V6yw74EdvoAWVauFqkH/GVM9YIpZitZf6bVEzG69tT",

	BabySignEotsPk: "AOoIG2cwC2IMiJL3OL0zLEIUY201X1qKumDr/1qDJ4oQvAp78W1nb5EnVasRPQ/XrKXqudUDnZFprLd0jaRJtQ==",
	EotsSignBaby:   "pR6vxgU0gXq+VqO+y7dHpZgHTz3zr5hdqXXh0WcWNkqUnRjHrizhYAHDMV8gh4vks4PqzKAIgZ779Wqwf5UrXQ==",

	BabyAddress: "bbn1f04czxeqprn0s9fe7kdzqyde2e6nqj63dllwsm",
}

func TestPoPVerifyEotsSignBaby(t *testing.T) {
	eotsPubKey, err := bbntypes.NewBIP340PubKeyFromHex(hardcodedPopToVerify.EotsPublicKey)
	require.NoError(t, err)

	schnorrSigBase64, err := base64.StdEncoding.DecodeString(hardcodedPopToVerify.EotsSignBaby)
	require.NoError(t, err)

	schnorrSig, err := schnorr.ParseSignature(schnorrSigBase64)
	require.NoError(t, err)

	sha256Addr := tmhash.Sum([]byte(hardcodedPopToVerify.BabyAddress))
	require.True(t, schnorrSig.Verify(sha256Addr, eotsPubKey.MustToBTCPK()))
}

func TestPoPVerifyBabySignEotsPk(t *testing.T) {

	babyPubKeyBz, err := base64.StdEncoding.DecodeString(hardcodedPopToVerify.BabyPublicKey)
	require.NoError(t, err)

	babyPubKey := &secp256k1.PubKey{
		Key: babyPubKeyBz,
	}
	require.NotNil(t, babyPubKey)

	babySignBtcDoc := daemon.NewCosmosSignDoc(
		hardcodedPopToVerify.BabyAddress,
		hardcodedPopToVerify.EotsPublicKey,
	)
	babySignBtcMarshaled, err := json.Marshal(babySignBtcDoc)
	require.NoError(t, err)

	babySignEotsBz := sdk.MustSortJSON(babySignBtcMarshaled)

	secp256SigBase64, err := base64.StdEncoding.DecodeString(hardcodedPopToVerify.BabySignEotsPk)
	require.NoError(t, err)

	require.True(t, babyPubKey.VerifySignature(babySignEotsBz, secp256SigBase64))
}

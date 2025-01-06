package daemon_test

import (
	"testing"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/cmd/eotsd/daemon"
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
	t.Parallel()
	valid, err := daemon.VerifyEotsSignBaby(
		hardcodedPopToVerify.EotsPublicKey,
		hardcodedPopToVerify.BabyAddress,
		hardcodedPopToVerify.EotsSignBaby,
	)
	require.NoError(t, err)
	require.True(t, valid)
}

func TestPoPVerifyBabySignEotsPk(t *testing.T) {
	t.Parallel()
	valid, err := daemon.VerifyBabySignEots(
		hardcodedPopToVerify.BabyPublicKey,
		hardcodedPopToVerify.BabyAddress,
		hardcodedPopToVerify.EotsPublicKey,
		hardcodedPopToVerify.BabySignEotsPk,
	)
	require.NoError(t, err)
	require.True(t, valid)
}

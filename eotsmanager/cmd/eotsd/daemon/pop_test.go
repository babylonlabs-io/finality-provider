package daemon_test

import (
	"testing"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/cmd/eotsd/daemon"
	"github.com/stretchr/testify/require"
)

var (
	hardcodedPopToVerify = daemon.PoPExport{
		EotsPublicKey: "3d0bebcbe800236ce8603c5bb1ab6c2af0932e947db4956a338f119797c37f1e",
		BabyPublicKey: "A0V6yw74EdvoAWVauFqkH/GVM9YIpZitZf6bVEzG69tT",

		BabySignEotsPk: "AOoIG2cwC2IMiJL3OL0zLEIUY201X1qKumDr/1qDJ4oQvAp78W1nb5EnVasRPQ/XrKXqudUDnZFprLd0jaRJtQ==",
		EotsSignBaby:   "pR6vxgU0gXq+VqO+y7dHpZgHTz3zr5hdqXXh0WcWNkqUnRjHrizhYAHDMV8gh4vks4PqzKAIgZ779Wqwf5UrXQ==",

		BabyAddress: "bbn1f04czxeqprn0s9fe7kdzqyde2e6nqj63dllwsm",
	}

	popsToVerify = []daemon.PoPExport{
		hardcodedPopToVerify,
		daemon.PoPExport{
			EotsPublicKey: "b1bc317bacf02fba17abea4f695c89997d55fe513a56ad8126237226212dd487",
			BabyPublicKey: "AyAU94yfGIa+MPq60oR3QnNn1DJ+9cZZrHDCu4Nx1Uo3",

			BabySignEotsPk: "HuPBTw5L3q3vqmb6F77ZSKfVP2pD1zCQvXbd+4JloAxzz/rpGQIvmvt4ZgcLI9vnL8bjltkmIlToRESHdiSbXA==",
			EotsSignBaby:   "L9wYf+rg2fQRzy2rau2MTO9V5sEBCSUWtWcxJxweSrvkDpR6tJTwKz3Ba0c/q7yNCT91Ag7H4rKMVhKmyN4tkQ==",

			BabyAddress: "bbn1ayrme3m73xv294t50k7v5pfj6pauyps03atepn",
		},
	}
)

func TestPoPValidEotsSignBaby(t *testing.T) {
	t.Parallel()

	for _, pop := range popsToVerify {
		valid, err := daemon.ValidEotsSignBaby(
			pop.EotsPublicKey,
			pop.BabyAddress,
			pop.EotsSignBaby,
		)
		require.NoError(t, err)
		require.True(t, valid)
	}
}

func TestPoPValidBabySignEotsPk(t *testing.T) {
	t.Parallel()
	for _, pop := range popsToVerify {
		valid, err := daemon.ValidBabySignEots(
			pop.BabyPublicKey,
			pop.BabyAddress,
			pop.EotsPublicKey,
			pop.BabySignEotsPk,
		)
		require.NoError(t, err)
		require.True(t, valid)
	}
}

func TestPoPVerify(t *testing.T) {
	t.Parallel()
	for _, pop := range popsToVerify {
		valid, err := daemon.VerifyPopExport(pop)
		require.NoError(t, err)
		require.True(t, valid)
	}
}

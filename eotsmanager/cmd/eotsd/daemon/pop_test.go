package daemon_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/cmd/eotsd/daemon"
	"github.com/stretchr/testify/require"
)

var popsToVerify = []daemon.PoPExport{
	daemon.PoPExport{
		EotsPublicKey: "3d0bebcbe800236ce8603c5bb1ab6c2af0932e947db4956a338f119797c37f1e",
		BabyPublicKey: "A0V6yw74EdvoAWVauFqkH/GVM9YIpZitZf6bVEzG69tT",

		BabySignEotsPk: "GO7xlC+BIypdcQdnIDsM+Ts75X9JKTOkDpXt5t4TSOIt/P1puAHVNhaYbweStVs25J9uRK+4XfrjD0M+t0Qy4g==",
		EotsSignBaby:   "pR6vxgU0gXq+VqO+y7dHpZgHTz3zr5hdqXXh0WcWNkqUnRjHrizhYAHDMV8gh4vks4PqzKAIgZ779Wqwf5UrXQ==",

		BabyAddress: "bbn1f04czxeqprn0s9fe7kdzqyde2e6nqj63dllwsm",
	},
	daemon.PoPExport{
		EotsPublicKey: "b1bc317bacf02fba17abea4f695c89997d55fe513a56ad8126237226212dd487",
		BabyPublicKey: "AyAU94yfGIa+MPq60oR3QnNn1DJ+9cZZrHDCu4Nx1Uo3",

		BabySignEotsPk: "kNcxCeqmQFQO//LvqhbzUQwbD3+/FfSbrvfyEa4Xf1MP5YDZp0XKYlOh+wA6dqFsb5IA7Wciz0WRbkGxRwxHVg==",
		EotsSignBaby:   "L9wYf+rg2fQRzy2rau2MTO9V5sEBCSUWtWcxJxweSrvkDpR6tJTwKz3Ba0c/q7yNCT91Ag7H4rKMVhKmyN4tkQ==",

		BabyAddress: "bbn1ayrme3m73xv294t50k7v5pfj6pauyps03atepn",
	},
}

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
		valid, err := daemon.ValidPopExport(pop)
		require.NoError(t, err)
		require.True(t, valid)
	}
}

func TestPoPValidate(t *testing.T) {
	t.Parallel()
	validateCmd := daemon.NewPopValidateExportCmd()

	tmp := t.TempDir()

	for i, pop := range popsToVerify {
		jsonString, err := json.MarshalIndent(pop, "", "  ")
		require.NoError(t, err)

		writer := bytes.NewBuffer([]byte{})
		validateCmd.SetOutput(writer)

		fileName := filepath.Join(tmp, fmt.Sprintf("%d-pop-out.json", i))
		err = os.WriteFile(fileName, jsonString, 0644)
		require.NoError(t, err)

		validateCmd.SetArgs([]string{fileName})

		err = validateCmd.ExecuteContext(context.Background())
		require.NoError(t, err)

		require.Equal(t, writer.String(), "Proof of Possession is valid!\n")
	}
}

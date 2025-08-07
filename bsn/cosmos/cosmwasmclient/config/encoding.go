package config

import (
	"os"

	sdklogs "cosmossdk.io/log"
	wasmapp "github.com/CosmWasm/wasmd/app"
	wasmdparams "github.com/CosmWasm/wasmd/app/params"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	dbm "github.com/cosmos/cosmos-db"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
)

// GetWasmdEncodingConfig creates a temporary WasmApp and returns its EncodingConfig.
func GetWasmdEncodingConfig() wasmdparams.EncodingConfig {
	return GetWasmdEncodingConfigWithPrefix("")
}

// GetWasmdEncodingConfigWithPrefix creates a temporary WasmApp with the specified account prefix and returns its EncodingConfig.
func GetWasmdEncodingConfigWithPrefix(accountPrefix string) wasmdparams.EncodingConfig {
	// Set account prefix if provided before creating the app
	if accountPrefix != "" {
		config := sdk.GetConfig()
		config.SetBech32PrefixForAccount(accountPrefix, accountPrefix+"pub")
		config.SetBech32PrefixForValidator(accountPrefix+"valoper", accountPrefix+"valoperpub")
		config.SetBech32PrefixForConsensusNode(accountPrefix+"valcons", accountPrefix+"valconspub")
	}

	// Create a temporary directory
	tempDir := func() string {
		dir, err := os.MkdirTemp("", "wasmd")
		if err != nil {
			panic("failed to create temp dir: " + err.Error())
		}

		return dir
	}

	// Initialize WasmApp
	tempApp := wasmapp.NewWasmApp(
		sdklogs.NewNopLogger(),
		dbm.NewMemDB(),
		nil,
		false,
		simtestutil.NewAppOptionsWithFlagHome(tempDir()),
		[]wasmkeeper.Option{},
	)

	// Create EncodingConfig
	encodingConfig := wasmdparams.EncodingConfig{
		InterfaceRegistry: tempApp.InterfaceRegistry(),
		Codec:             tempApp.AppCodec(),
		TxConfig:          tempApp.TxConfig(),
		Amino:             tempApp.LegacyAmino(),
	}

	return encodingConfig
}

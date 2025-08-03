package config_test

import (
	"github.com/babylonlabs-io/finality-provider/bsn/cosmos/cosmwasmclient/config"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWasmQueryConfig ensures that the default Babylon query config is valid
func TestWasmQueryConfig(t *testing.T) {
	t.Parallel()
	defaultConfig := config.DefaultWasmQueryConfig()
	err := defaultConfig.Validate()
	require.NoError(t, err)
}

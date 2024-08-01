package cmd_test

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	fpcmd "github.com/babylonchain/finality-provider/finality-provider/cmd"
	fpcfg "github.com/babylonchain/finality-provider/finality-provider/config"
	"github.com/babylonchain/finality-provider/util"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	goflags "github.com/jessevdk/go-flags"
)

func TestPersistClientCtx(t *testing.T) {
	ctx := client.Context{}
	cmd := cobra.Command{}

	tempDir := t.TempDir()
	defaultHome := filepath.Join(tempDir, "defaultHome")

	cmd.Flags().String(flags.FlagHome, defaultHome, "The application home directory")
	cmd.Flags().String(flags.FlagChainID, "", "chain id")

	err := fpcmd.PersistClientCtx(ctx)(&cmd, []string{})
	require.NoError(t, err)

	// verify that has the defaults to ctx
	ctx = client.GetClientContextFromCmd(&cmd)
	require.Equal(t, defaultHome, ctx.HomeDir)
	require.Equal(t, "", ctx.ChainID)

	flagHomeValue := filepath.Join(tempDir, "flagHome")
	err = cmd.Flags().Set(flags.FlagHome, flagHomeValue)
	require.NoError(t, err)

	err = fpcmd.PersistClientCtx(ctx)(&cmd, []string{})
	require.NoError(t, err)

	ctx = client.GetClientContextFromCmd(&cmd)
	require.Equal(t, flagHomeValue, ctx.HomeDir)

	r := rand.New(rand.NewSource(10))
	randChainID := datagen.GenRandomHexStr(r, 10)

	// creates fpd config with chainID at flagHomeValue
	err = util.MakeDirectory(flagHomeValue)
	require.NoError(t, err)

	config := fpcfg.DefaultConfigWithHome(flagHomeValue)
	config.BabylonConfig.ChainID = randChainID
	fileParser := goflags.NewParser(&config, goflags.Default)

	err = goflags.NewIniParser(fileParser).WriteFile(fpcfg.ConfigFile(flagHomeValue), goflags.IniIncludeComments|goflags.IniIncludeDefaults)
	require.NoError(t, err)

	// parses the ctx from cmd with config, should modify the chain ID
	err = fpcmd.PersistClientCtx(ctx)(&cmd, []string{})
	require.NoError(t, err)

	ctx = client.GetClientContextFromCmd(&cmd)
	require.Equal(t, flagHomeValue, ctx.HomeDir)
	require.Equal(t, randChainID, ctx.ChainID)

	flagChainID := "chainIDFromFlag"
	err = cmd.Flags().Set(flags.FlagChainID, flagChainID)
	require.NoError(t, err)

	// parses the ctx from cmd with config, but it has set in flags which should give
	// preference over the config set, so it should use from the flag value set.
	err = fpcmd.PersistClientCtx(ctx)(&cmd, []string{})
	require.NoError(t, err)

	ctx = client.GetClientContextFromCmd(&cmd)
	require.Equal(t, flagHomeValue, ctx.HomeDir)
	require.Equal(t, flagChainID, ctx.ChainID)
}

package daemon

import (
	"fmt"
	"github.com/babylonlabs-io/finality-provider/bsn/cosmos/config"
	cosmosservice "github.com/babylonlabs-io/finality-provider/bsn/cosmos/service"
	"net"
	"path/filepath"

	"github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	commoncmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/common"
	fpdaemon "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
)

// CommandStart returns the start command of fpd daemon.
func CommandStart(binaryName string) *cobra.Command {
	cmd := fpdaemon.CommandStartTemplate(binaryName)
	cmd.RunE = clientctx.RunEWithClientCtx(runStartCmd)

	return cmd
}

func runStartCmd(ctx client.Context, cmd *cobra.Command, _ []string) error {
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	homePath = util.CleanAndExpandPath(homePath)
	flags := cmd.Flags()

	fpStr, err := flags.GetString(commoncmd.FpEotsPkFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", commoncmd.FpEotsPkFlag, err)
	}

	rpcListener, err := flags.GetString(commoncmd.RPCListenerFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", commoncmd.RPCListenerFlag, err)
	}

	cfg, err := config.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if cfg.Common.BabylonConfig.KeyringBackend != "test" {
		return fmt.Errorf("the keyring backend in config must be `test` for automatic signing, got %s", cfg.Common.BabylonConfig.KeyringBackend)
	}

	if rpcListener != "" {
		_, err := net.ResolveTCPAddr("tcp", rpcListener)
		if err != nil {
			return fmt.Errorf("invalid RPC listener address %s, %w", rpcListener, err)
		}
		cfg.Common.RPCListener = rpcListener
	}

	logger, err := log.NewRootLoggerWithFile(config.LogFile(homePath), cfg.Common.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	dbBackend, err := cfg.Common.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	fpApp, err := cosmosservice.NewCosmosBSNFinalityProviderAppFromConfig(cfg, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create finality-provider app: %w", err)
	}

	if err := fpdaemon.StartApp(cmd.Context(), fpApp, fpStr); err != nil {
		return fmt.Errorf("failed to start app: %w", err)
	}

	fpServer := service.NewFinalityProviderServer(cfg.Common, logger, fpApp, dbBackend)

	if err := fpServer.RunUntilShutdown(cmd.Context()); err != nil {
		return fmt.Errorf("failed to run server until shutdown: %w", err)
	}

	return nil
}

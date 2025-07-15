package daemon

import (
	"fmt"
	"net"
	"path/filepath"

	rollupfpcfg "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/config"
	rollupservice "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/service"
	clientctx "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	commoncmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/common"
	fpdaemon "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
)

// CommandStart returns the start command of fpd daemon.
func CommandStart() *cobra.Command {
	cmd := fpdaemon.CommandStartTemplate()
	cmd.RunE = clientctx.RunEWithClientCtx(runStartCmd)

	return cmd
}

func runStartCmd(ctx client.Context, cmd *cobra.Command, _ []string) error {
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return err
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

	cfg, err := rollupfpcfg.LoadConfig(homePath)
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

	logger, err := log.NewRootLoggerWithFile(fpcfg.LogFile(homePath), cfg.Common.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	dbBackend, err := cfg.Common.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	fpApp, err := rollupservice.NewRollupBSNFinalityProviderAppFromConfig(cfg, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create finality-provider app: %w", err)
	}

	if err := fpdaemon.StartApp(fpApp, fpStr); err != nil {
		return fmt.Errorf("failed to start app: %w", err)
	}

	fpServer := service.NewFinalityProviderServer(cfg.Common, logger, fpApp, dbBackend)

	return fpServer.RunUntilShutdown(cmd.Context())
}

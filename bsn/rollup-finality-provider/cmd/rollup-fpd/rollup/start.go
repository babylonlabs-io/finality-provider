package rollup

import (
	"context"
	"fmt"
	"net"
	"path/filepath"

	rollupcfg "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/config"
	rollupservice "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/service"
	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	common "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/common"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
)

// CommandStart returns the start command of fpd daemon.
func CommandStart() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "start",
		Short:   "Start the finality-provider app daemon.",
		Long:    `Start the finality-provider app. Note that eotsd should be started beforehand`,
		Example: `fpd start --home /home/user/.fpd`,
		Args:    cobra.NoArgs,
		RunE:    fpcmd.RunEWithClientCtx(runStartCmd),
	}
	cmd.Flags().String(common.FpEotsPkFlag, "", "The EOTS public key of the finality-provider to start")
	cmd.Flags().String(common.RPCListenerFlag, "", "The address that the RPC server listens to")
	cmd.Flags().String(flags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")

	return cmd
}

func runStartCmd(ctx client.Context, cmd *cobra.Command, _ []string) error {
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return err
	}
	homePath = util.CleanAndExpandPath(homePath)
	flags := cmd.Flags()

	fpStr, err := flags.GetString(common.FpEotsPkFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", common.FpEotsPkFlag, err)
	}

	rpcListener, err := flags.GetString(common.RPCListenerFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", common.RPCListenerFlag, err)
	}

	cfg, err := rollupcfg.LoadRollupFPConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	return RunCommandStartWithCfg(cmd.Context(), cfg, fpStr, rpcListener, homePath)
}

func RunCommandStartWithCfg(ctx context.Context, cfg *rollupcfg.RollupFPConig, fpStr, rpcListener, homePath string) error {
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

	fpApp, err := rollupservice.NewRollupFinalityProviderAppFromConfig(ctx, cfg, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create finality-provider app: %w", err)
	}

	if err := common.StartApp(fpApp, fpStr); err != nil {
		return fmt.Errorf("failed to start app: %w", err)
	}

	fpServer := service.NewFinalityProviderServer(cfg.Common, logger, fpApp, dbBackend)

	return fpServer.RunUntilShutdown(ctx)
}

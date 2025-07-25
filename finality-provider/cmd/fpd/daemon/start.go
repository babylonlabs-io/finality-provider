package daemon

import (
	"context"
	"fmt"
	"net"
	"path/filepath"

	"github.com/babylonlabs-io/babylon/v3/types"
	clientctx "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	commoncmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/common"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
)

// CommandStart returns the start command of fpd daemon.
func CommandStart(binaryName string) *cobra.Command {
	cmd := CommandStartTemplate(binaryName)
	cmd.RunE = clientctx.RunEWithClientCtx(runStartCmd)

	return cmd
}

func CommandStartTemplate(binaryName string) *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "start",
		Short:   "Start the finality-provider app daemon.",
		Long:    `Start the finality-provider app. Note that eotsd should be started beforehand`,
		Example: fmt.Sprintf(`%s start --home /home/user/.fpd`, binaryName),
		Args:    cobra.NoArgs,
	}
	cmd.Flags().String(commoncmd.FpEotsPkFlag, "", "The EOTS public key of the finality-provider to start")
	cmd.Flags().String(commoncmd.RPCListenerFlag, "", "The address that the RPC server listens to")
	cmd.Flags().String(flags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")

	return cmd
}

func runStartCmd(ctx client.Context, cmd *cobra.Command, _ []string) error {
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return fmt.Errorf("failed to get home path: %w", err)
	}
	homePath = util.CleanAndExpandPath(homePath)
	cfg, err := fpcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	flags := cmd.Flags()

	fpStr, err := flags.GetString(commoncmd.FpEotsPkFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", commoncmd.FpEotsPkFlag, err)
	}

	rpcListener, err := flags.GetString(commoncmd.RPCListenerFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", commoncmd.RPCListenerFlag, err)
	}

	if cfg.BabylonConfig.KeyringBackend != "test" {
		return fmt.Errorf("the keyring backend in config must be `test` for automatic signing, got %s", cfg.BabylonConfig.KeyringBackend)
	}

	if rpcListener != "" {
		_, err := net.ResolveTCPAddr("tcp", rpcListener)
		if err != nil {
			return fmt.Errorf("invalid RPC listener address %s, %w", rpcListener, err)
		}
		cfg.RPCListener = rpcListener
	}

	logger, err := log.NewRootLoggerWithFile(fpcfg.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	fpApp, err := service.NewFinalityProviderAppFromConfig(cfg, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create finality-provider app: %w", err)
	}

	if err := StartApp(cmd.Context(), fpApp, fpStr); err != nil {
		return fmt.Errorf("failed to start app: %w", err)
	}

	fpServer := service.NewFinalityProviderServer(cfg, logger, fpApp, dbBackend)

	if err := fpServer.RunUntilShutdown(cmd.Context()); err != nil {
		return fmt.Errorf("failed to run finality provider server: %w", err)
	}

	return nil
}

// StartApp starts the app and the handle of finality providers if needed based on flags.
func StartApp(
	ctx context.Context,
	fpApp *service.FinalityProviderApp,
	fpPkStr string,
) error {
	// only start the app without starting any finality provider instance
	// this is needed for new finality provider registration or unjailing
	// finality providers
	if err := fpApp.Start(ctx); err != nil {
		return fmt.Errorf("failed to start the finality provider app: %w", err)
	}

	// fp instance will be started if public key is specified
	if fpPkStr != "" {
		// start the finality-provider instance with the given public key
		fpPk, err := types.NewBIP340PubKeyFromHex(fpPkStr)
		if err != nil {
			return fmt.Errorf("invalid finality provider public key %s: %w", fpPkStr, err)
		}

		if err := fpApp.StartFinalityProvider(ctx, fpPk); err != nil {
			return fmt.Errorf("failed to start finality provider: %w", err)
		}

		return nil
	}

	storedFps, err := fpApp.GetFinalityProviderStore().GetAllStoredFinalityProviders()
	if err != nil {
		return fmt.Errorf("failed to get all stored finality providers: %w", err)
	}

	if len(storedFps) == 1 {
		if err := fpApp.StartFinalityProvider(ctx, types.NewBIP340PubKeyFromBTCPK(storedFps[0].BtcPk)); err != nil {
			return fmt.Errorf("failed to start finality provider: %w", err)
		}

		return nil
	}

	if len(storedFps) > 1 {
		return fmt.Errorf("%d finality providers found in DB. Please specify the EOTS public key", len(storedFps))
	}

	fpApp.Logger().Info("No finality providers found in DB. Waiting for registration.")

	return nil
}

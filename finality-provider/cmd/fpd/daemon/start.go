package daemon

import (
	"fmt"
	"net"
	"path/filepath"

	"github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/util"
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
	cmd.Flags().String(fpEotsPkFlag, "", "The EOTS public key of the finality-provider to start")
	cmd.Flags().String(passphraseFlag, "", "The pass phrase used to decrypt the private key")
	cmd.Flags().String(rpcListenerFlag, "", "The address that the RPC server listens to")
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

	fpStr, err := flags.GetString(fpEotsPkFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", fpEotsPkFlag, err)
	}

	rpcListener, err := flags.GetString(rpcListenerFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", rpcListenerFlag, err)
	}

	passphrase, err := flags.GetString(passphraseFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", passphraseFlag, err)
	}

	cfg, err := fpcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
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

	fpApp, err := loadApp(logger, cfg, dbBackend)
	if err != nil {
		return fmt.Errorf("failed to load app: %w", err)
	}

	if err := startApp(fpApp, fpStr, passphrase); err != nil {
		return fmt.Errorf("failed to start app: %w", err)
	}

	fpServer := service.NewFinalityProviderServer(cfg, logger, fpApp, dbBackend)

	return fpServer.RunUntilShutdown(cmd.Context())
}

// loadApp initialize an finality provider app based on config and flags set.
func loadApp(
	logger *zap.Logger,
	cfg *fpcfg.Config,
	dbBackend walletdb.DB,
) (*service.FinalityProviderApp, error) {
	fpApp, err := service.NewFinalityProviderAppFromConfig(cfg, dbBackend, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create finality-provider app: %w", err)
	}

	return fpApp, nil
}

// startApp starts the app and the handle of finality providers if needed based on flags.
func startApp(
	fpApp *service.FinalityProviderApp,
	fpPkStr, passphrase string,
) error {
	// only start the app without starting any finality provider instance
	// this is needed for new finality provider registration or unjailing
	// finality providers
	if err := fpApp.Start(); err != nil {
		return fmt.Errorf("failed to start the finality provider app: %w", err)
	}

	// fp instance will be started if public key is specified
	if fpPkStr != "" {
		// start the finality-provider instance with the given public key
		fpPk, err := types.NewBIP340PubKeyFromHex(fpPkStr)
		if err != nil {
			return fmt.Errorf("invalid finality provider public key %s: %w", fpPkStr, err)
		}

		return fpApp.StartFinalityProvider(fpPk, passphrase)
	}

	storedFps, err := fpApp.GetFinalityProviderStore().GetAllStoredFinalityProviders()
	if err != nil {
		return err
	}

	if len(storedFps) == 1 {
		return fpApp.StartFinalityProvider(types.NewBIP340PubKeyFromBTCPK(storedFps[0].BtcPk), passphrase)
	}

	if len(storedFps) > 1 {
		return fmt.Errorf("%d finality providers found in DB. Please specify the EOTS public key", len(storedFps))
	}

	fpApp.Logger().Info("No finality providers found in DB. Waiting for registration.")

	return nil
}

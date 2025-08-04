package daemon

import (
	"fmt"
	"net"
	"path/filepath"

	"context"

	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	rollupfpcc "github.com/babylonlabs-io/finality-provider/bsn/rollup/clientcontroller"
	rollupfpcfg "github.com/babylonlabs-io/finality-provider/bsn/rollup/config"
	rollupservice "github.com/babylonlabs-io/finality-provider/bsn/rollup/service"
	clientctx "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	commoncmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/common"
	fpdaemon "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
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

	logger, err := log.NewRootLoggerWithFile(rollupfpcfg.LogFile(homePath), cfg.Common.LogLevel)
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

	if err := validateRollupFP(cmd.Context(), fpApp, fpStr, logger); err != nil {
		return fmt.Errorf("failed to validate rollup finality provider: %w", err)
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

func validateRollupFP(ctx context.Context, fpApp *service.FinalityProviderApp, fpStr string, logger *zap.Logger) error {
	rollupController, ok := fpApp.GetConsumerController().(*rollupfpcc.RollupBSNController)
	if !ok {
		return fmt.Errorf("expected RollupBSNController but got different controller type")
	}

	var fpToValidate *bbntypes.BIP340PubKey

	if fpStr == "" {
		// If no FP is specified, then check DB
		storedFps, err := fpApp.GetFinalityProviderStore().GetAllStoredFinalityProviders()
		if err != nil {
			return fmt.Errorf("failed to get stored finality providers: %w", err)
		}

		if len(storedFps) != 1 {
			return fmt.Errorf("%d finality providers found in DB. Please specify the EOTS public key", len(storedFps))
		}

		fpToValidate = bbntypes.NewBIP340PubKeyFromBTCPK(storedFps[0].BtcPk)
	} else {
		// If FP is specified, then use this one
		fpPk, err := bbntypes.NewBIP340PubKeyFromHex(fpStr)
		if err != nil {
			return fmt.Errorf("invalid finality provider public key %s: %w", fpStr, err)
		}
		fpToValidate = fpPk
	}

	allowed, err := rollupController.QueryFinalityProviderInAllowlist(ctx, fpToValidate.MustToBTCPK())
	if err != nil {
		return fmt.Errorf("failed to check allowlist for FP %s: %w", fpToValidate.MarshalHex(), err)
	}
	if !allowed {
		return fmt.Errorf("finality provider %s is not in allowlist", fpToValidate.MarshalHex())
	}

	logger.Info("Finality provider verified in allowlist", zap.String("fp_pk", fpToValidate.MarshalHex()))

	return nil
}

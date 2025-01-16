package daemon

import (
	"fmt"
	"net"

	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	eotsservice "github.com/babylonlabs-io/finality-provider/eotsmanager/service"
	"github.com/babylonlabs-io/finality-provider/log"
)

func NewStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Extractable One Time Signature Daemon",
		Long:  "Start the Extractable One Time Signature Daemon and run it until shutdown.",
		RunE:  startFn,
	}

	cmd.Flags().String(sdkflags.FlagHome, config.DefaultEOTSDir, "The path to the eotsd home directory")
	cmd.Flags().String(rpcListenerFlag, "", "The address that the RPC server listens to")

	return cmd
}

func startFn(cmd *cobra.Command, _ []string) error {
	homePath, err := getHomePath(cmd)
	if err != nil {
		return fmt.Errorf("failed to load home flag: %w", err)
	}

	cfg, err := config.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load config at %s: %w", homePath, err)
	}

	rpcListener, err := cmd.Flags().GetString(rpcListenerFlag)
	if err != nil {
		return fmt.Errorf("failed to get RPC listener flag: %w", err)
	}
	if rpcListener != "" {
		_, err := net.ResolveTCPAddr("tcp", rpcListener)
		if err != nil {
			return fmt.Errorf("invalid RPC listener address %s: %w", rpcListener, err)
		}
		cfg.RPCListener = rpcListener
	}

	logger, err := log.NewRootLoggerWithFile(config.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to load the logger: %w", err)
	}

	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	eotsManager, err := eotsmanager.NewLocalEOTSManager(homePath, cfg.KeyringBackend, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager: %w", err)
	}

	eotsServer := eotsservice.NewEOTSManagerServer(cfg, logger, eotsManager, dbBackend)

	return eotsServer.RunUntilShutdown(cmd.Context())
}

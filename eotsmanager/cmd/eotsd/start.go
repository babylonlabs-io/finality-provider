package main

import (
	"fmt"
	"net"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	eotsservice "github.com/babylonlabs-io/finality-provider/eotsmanager/service"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/lightningnetwork/lnd/signal"
	"github.com/spf13/cobra"
)

func NewStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Extractable One Time Signature Daemon",
		Long:  "Start the Extractable One Time Signature Daemon and run it until shutdown.",
		RunE:  startFn,
	}

	cmd.Flags().String(homeFlag, config.DefaultEOTSDir, "The path to the eotsd home directory")
	cmd.Flags().String(rpcListenerFlag, "", "The address that the RPC server listens to")

	return cmd
}

func startFn(cmd *cobra.Command, args []string) error {
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
		cfg.RpcListener = rpcListener
	}

	logger, err := log.NewRootLoggerWithFile(config.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to load the logger: %w", err)
	}

	dbBackend, err := cfg.DatabaseConfig.GetDbBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	eotsManager, err := eotsmanager.NewLocalEOTSManager(homePath, cfg.KeyringBackend, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager: %w", err)
	}

	// Hook interceptor for os signals.
	shutdownInterceptor, err := signal.Intercept()
	if err != nil {
		return fmt.Errorf("failed to set up shutdown interceptor: %w", err)
	}

	eotsServer := eotsservice.NewEOTSManagerServer(cfg, logger, eotsManager, dbBackend, shutdownInterceptor)

	return eotsServer.RunUntilShutdown()
}

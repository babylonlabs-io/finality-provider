package daemon

import (
	"fmt"
	"net"
	"path/filepath"

	"github.com/lightningnetwork/lnd/signal"
	"github.com/urfave/cli"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	eotsservice "github.com/babylonlabs-io/finality-provider/eotsmanager/service"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/util"
)

var StartCommand = cli.Command{
	Name:        "start",
	Usage:       "Start the Extractable One Time Signature Daemon.",
	Description: "Start the Extractable One Time Signature Daemon.",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "The path to the eotsd home directory",
			Value: config.DefaultEOTSDir,
		},
		cli.StringFlag{
			Name:  rpcListenerFlag,
			Usage: "The address that the RPC server listens to",
		},
	},
	Action: startFn,
}

func startFn(ctx *cli.Context) error {
	homePath, err := getHomeFlag(ctx)
	if err != nil {
		return fmt.Errorf("failed to load home flag: %w", err)
	}

	cfg, err := config.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load config at %s: %w", homePath, err)
	}

	rpcListener := ctx.String(rpcListenerFlag)
	if rpcListener != "" {
		_, err := net.ResolveTCPAddr("tcp", rpcListener)
		if err != nil {
			return fmt.Errorf("invalid RPC listener address %s, %w", rpcListener, err)
		}
		cfg.RpcListener = rpcListener
	}

	logger, err := log.NewRootLoggerWithFile(config.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to load the logger")
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
		return err
	}

	eotsServer := eotsservice.NewEOTSManagerServer(cfg, logger, eotsManager, dbBackend, shutdownInterceptor)

	return eotsServer.RunUntilShutdown()
}

func getHomeFlag(ctx *cli.Context) (string, error) {
	return getCleanPath(ctx, homeFlag)
}

func getCleanPath(ctx *cli.Context, flag string) (string, error) {
	absPath, err := filepath.Abs(ctx.String(flag))
	if err != nil {
		return "", err
	}
	return util.CleanAndExpandPath(absPath), nil
}

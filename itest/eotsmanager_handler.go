package e2etest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/service"
)

type EOTSServerHandler struct {
	t          *testing.T
	eotsServer *service.Server
	cfg        *config.Config
}

func NewEOTSServerHandler(t *testing.T, cfg *config.Config, eotsHomeDir string) *EOTSServerHandler {
	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)
	loggerConfig := zap.NewDevelopmentConfig()
	loggerConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	logger, err := loggerConfig.Build()
	require.NoError(t, err)
	eotsManager, err := eotsmanager.NewLocalEOTSManager(eotsHomeDir, cfg.KeyringBackend, dbBackend, logger)
	require.NoError(t, err)

	eotsServer := service.NewEOTSManagerServer(cfg, logger, eotsManager, dbBackend)

	return &EOTSServerHandler{
		t:          t,
		eotsServer: eotsServer,
		cfg:        cfg,
	}
}

func (eh *EOTSServerHandler) Start(ctx context.Context) {
	go eh.startServer(ctx)
}

func (eh *EOTSServerHandler) startServer(ctx context.Context) {
	err := eh.eotsServer.RunUntilShutdown(ctx)
	require.NoError(eh.t, err)
}

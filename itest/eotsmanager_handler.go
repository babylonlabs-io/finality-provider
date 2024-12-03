package e2e_utils

import (
	"testing"

	"github.com/lightningnetwork/lnd/signal"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/service"
)

type EOTSServerHandler struct {
	t           *testing.T
	interceptor *signal.Interceptor
	eotsServers []*service.Server
}

func NewEOTSServerHandlerMultiFP(
	t *testing.T, logger *zap.Logger, configs []*config.Config, eotsHomeDirs []string, shutdownInterceptor *signal.Interceptor,
) *EOTSServerHandler {
	eotsServers := make([]*service.Server, 0, len(configs))
	for i, cfg := range configs {
		dbBackend, err := cfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)

		eotsManager, err := eotsmanager.NewLocalEOTSManager(eotsHomeDirs[i], cfg.KeyringBackend, dbBackend, logger)
		require.NoError(t, err)

		eotsServer := service.NewEOTSManagerServer(cfg, logger, eotsManager, dbBackend, *shutdownInterceptor)
		eotsServers = append(eotsServers, eotsServer)
	}

	return &EOTSServerHandler{
		t:           t,
		eotsServers: eotsServers,
		interceptor: shutdownInterceptor,
	}
}

func NewEOTSServerHandler(t *testing.T, logger *zap.Logger, cfg *config.Config, eotsHomeDir string) *EOTSServerHandler {
	// create shutdown interceptor
	shutdownInterceptor, err := signal.Intercept()
	require.NoError(t, err)
	// this need refactor of NewEOTSServerHandler
	return NewEOTSServerHandlerMultiFP(t, logger, []*config.Config{cfg}, []string{eotsHomeDir}, &shutdownInterceptor)
}

func (eh *EOTSServerHandler) Start() {
	go eh.startServer()
}

func (eh *EOTSServerHandler) startServer() {
	for _, eotsServer := range eh.eotsServers {
		go func(eotsServer *service.Server) {
			err := eotsServer.RunUntilShutdown()
			require.NoError(eh.t, err)
		}(eotsServer)
	}
}

func (eh *EOTSServerHandler) Stop() {
	eh.interceptor.RequestShutdown()
}

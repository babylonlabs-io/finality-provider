package e2e_utils

import (
	"context"
	"os"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/service"
)

type EOTSServerHandler struct {
	t           *testing.T
	eotsServer  *service.Server
	eotsManager *eotsmanager.LocalEOTSManager
	cfg         *config.Config
}

func NewEOTSServerHandler(t *testing.T, cfg *config.Config, eotsHomeDir string) *EOTSServerHandler {
	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	require.NoError(t, err)
	loggerConfig := zap.NewDevelopmentConfig()
	loggerConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	logger, err := loggerConfig.Build()
	require.NoError(t, err)

	fileParser := flags.NewParser(cfg, flags.Default)
	err = flags.NewIniParser(fileParser).WriteFile(config.CfgFile(eotsHomeDir), flags.IniIncludeComments|flags.IniIncludeDefaults)
	require.NoError(t, err)

	eotsManager, err := eotsmanager.NewLocalEOTSManager(eotsHomeDir, cfg.KeyringBackend, dbBackend, logger)
	require.NoError(t, err)

	eotsServer := service.NewEOTSManagerServer(cfg, logger, eotsManager, dbBackend)

	return &EOTSServerHandler{
		t:           t,
		eotsServer:  eotsServer,
		eotsManager: eotsManager,
		cfg:         cfg,
	}
}

func (eh *EOTSServerHandler) Config() *config.Config {
	return eh.cfg
}

func (eh *EOTSServerHandler) Start(ctx context.Context) {
	go eh.startServer(ctx)
}

func (eh *EOTSServerHandler) startServer(ctx context.Context) {
	err := eh.eotsServer.RunUntilShutdown(ctx)
	require.NoError(eh.t, err)
}

func (eh *EOTSServerHandler) CreateKey(name string) ([]byte, error) {
	return eh.eotsManager.CreateKey(name)
}

func (eh *EOTSServerHandler) GetFPPrivKey(t *testing.T, fpPk []byte) *btcec.PrivateKey {
	privKey, err := eh.eotsManager.KeyRecord(fpPk)
	require.NoError(t, err)
	return privKey.PrivKey
}

// SetHMACKey sets the HMAC key in the environment for the EOTS server
func (eh *EOTSServerHandler) SetHMACKey(hmacKey string) error {
	if err := os.Setenv(client.HMACKeyEnvVar, hmacKey); err != nil {
		return err
	}

	eh.cfg.HMACKey = hmacKey

	return nil
}

package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func GetTestLogger(t *testing.T) *zap.Logger {
	loggerConfig := zap.NewDevelopmentConfig()
	loggerConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	logger, err := loggerConfig.Build()

	require.NoError(t, err)

	return logger
}

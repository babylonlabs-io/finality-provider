package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/finality-provider/config"
)

var fpCfg = config.DefaultConfig()

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr string
	}{
		{
			name:    "valid config",
			cfg:     &fpCfg,
			wantErr: "",
		},
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: "config cannot be nil",
		},
		{
			name: "zero signature submission interval",
			cfg: &config.Config{
				SignatureSubmissionInterval: 0,
				SubmissionRetryInterval:     time.Second,
				BatchSubmissionSize:         100,
				MaxSubmissionRetries:        50,
				NumPubRand:                  100,
				PollerConfig:                defaultPollerConfig(),
			},
			wantErr: "timing configuration validation failed: signature submission interval must be positive, got 0s",
		},
		{
			name: "zero submission retry interval",
			cfg: &config.Config{
				SignatureSubmissionInterval: time.Second,
				SubmissionRetryInterval:     0,
				BatchSubmissionSize:         100,
				MaxSubmissionRetries:        50,
				NumPubRand:                  100,
				PollerConfig:                defaultPollerConfig(),
			},
			wantErr: "timing configuration validation failed: submission retry interval must be positive, got 0s",
		},
		{
			name: "zero batch submission size",
			cfg: &config.Config{
				SignatureSubmissionInterval: time.Second,
				SubmissionRetryInterval:     time.Second,
				BatchSubmissionSize:         0,
				MaxSubmissionRetries:        50,
				NumPubRand:                  100,
				PollerConfig:                defaultPollerConfig(),
			},
			wantErr: "batch and retry configuration validation failed: batch submission size must be positive, got 0",
		},
		{
			name: "batch submission size exceeds maximum",
			cfg: &config.Config{
				SignatureSubmissionInterval: time.Second,
				SubmissionRetryInterval:     time.Second,
				BatchSubmissionSize:         config.MaxBatchSize + 1,
				MaxSubmissionRetries:        50,
				NumPubRand:                  100,
				PollerConfig:                defaultPollerConfig(),
			},
			wantErr: "batch and retry configuration validation failed: batch submission size must not exceed 100, got 101",
		},
		{
			name: "zero max submission retries",
			cfg: &config.Config{
				SignatureSubmissionInterval: time.Second,
				SubmissionRetryInterval:     time.Second,
				BatchSubmissionSize:         100,
				MaxSubmissionRetries:        0,
				NumPubRand:                  100,
				PollerConfig:                defaultPollerConfig(),
			},
			wantErr: "batch and retry configuration validation failed: max submission retries must be positive, got 0",
		},
		{
			name: "num pub rand exceeds maximum",
			cfg: &config.Config{
				SignatureSubmissionInterval: time.Second,
				SubmissionRetryInterval:     time.Second,
				BatchSubmissionSize:         100,
				MaxSubmissionRetries:        50,
				NumPubRand:                  config.MaxPubRand + 1,
				PollerConfig:                defaultPollerConfig(),
			},
			wantErr: "batch and retry configuration validation failed: number of public randomness must not exceed 500000, got 500001",
		},
		{
			name: "nil poller config",
			cfg: &config.Config{
				SignatureSubmissionInterval: time.Second,
				SubmissionRetryInterval:     time.Second,
				TimestampingDelayBlocks:     1,
				BatchSubmissionSize:         100,
				MaxSubmissionRetries:        50,
				NumPubRand:                  config.MinPubRand,
				PollerConfig:                nil,
			},
			wantErr: "poller config cannot be empty",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

// Helper function to create a default valid ChainPollerConfig for testing
func defaultPollerConfig() *config.ChainPollerConfig {
	return &config.ChainPollerConfig{
		BufferSize:                     1000,
		PollInterval:                   time.Second,
		StaticChainScanningStartHeight: 1,
		AutoChainScanningMode:          true,
		PollSize:                       1000,
	}
}

package metrics

import (
	"fmt"
	"net"
	"time"
)

const (
	defaultFpMetricsPort         = 2112
	defaultEotsMetricsPort       = 2113
	defaultMetricsHost           = "127.0.0.1"
	defaultMetricsUpdateInterval = 100 * time.Millisecond
)

type Config struct {
	Host           string        `long:"host" description:"IP of the Prometheus server"`
	Port           int           `long:"port" description:"Port of the Prometheus server"`
	UpdateInterval time.Duration `long:"updateinterval" description:"The interval of Prometheus metrics updated"`
}

func (cfg *Config) Validate() error {
	if cfg.Port < 0 || cfg.Port > 65535 {
		return fmt.Errorf("invalid port: %d", cfg.Port)
	}

	ip := net.ParseIP(cfg.Host)
	if ip == nil {
		return fmt.Errorf("invalid host: %v", cfg.Host)
	}

	return nil
}

func (cfg *Config) Address() (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), nil
}

func DefaultFpConfig() *Config {
	return &Config{
		Port:           defaultFpMetricsPort,
		Host:           defaultMetricsHost,
		UpdateInterval: defaultMetricsUpdateInterval,
	}
}

func DefaultEotsConfig() *Config {
	return &Config{
		Port:           defaultEotsMetricsPort,
		Host:           defaultMetricsHost,
		UpdateInterval: defaultMetricsUpdateInterval,
	}
}

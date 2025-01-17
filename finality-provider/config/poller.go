package config

import (
	"fmt"
	"time"
)

var (
	defaultBufferSize        = uint32(1000)
	defaultPollingInterval   = 1 * time.Second
	defaultStaticStartHeight = uint64(1)
	defaultPollSize          = uint32(1000)
)

type ChainPollerConfig struct {
	BufferSize                     uint32        `long:"buffersize" description:"The maximum number of Babylon blocks that can be stored in the buffer"`
	PollInterval                   time.Duration `long:"pollinterval" description:"The interval between each polling of blocks; the value should be set depending on the block production time but could be set smaller for quick catching up"`
	StaticChainScanningStartHeight uint64        `long:"staticchainscanningstartheight" description:"The static height from which we start polling the chain"`
	AutoChainScanningMode          bool          `long:"autochainscanningmode" description:"Automatically discover the height from which to start polling the chain"`
	PollSize                       uint32        `long:"pollsize" description:"The poll batch size when polling for blocks"`
}

func DefaultChainPollerConfig() ChainPollerConfig {
	return ChainPollerConfig{
		BufferSize:                     defaultBufferSize,
		PollInterval:                   defaultPollingInterval,
		PollSize:                       defaultPollSize,
		StaticChainScanningStartHeight: defaultStaticStartHeight,
		AutoChainScanningMode:          true,
	}
}

func (c ChainPollerConfig) Validate() error {
	if c.BufferSize == 0 {
		return fmt.Errorf("invalid buffersize: %d", c.BufferSize)
	}

	if c.PollSize == 0 {
		return fmt.Errorf("invalid pollinterval: %d", c.PollInterval)
	}

	return nil
}

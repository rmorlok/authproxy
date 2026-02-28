package request_log

import (
	"time"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

type captureConfig struct {
	expiration            time.Duration
	recordFullRequest     bool
	maxFullRequestSize    uint64
	maxFullResponseSize   uint64
	maxResponseWait       time.Duration
	fullRequestExpiration time.Duration
}

func (c *captureConfig) setFromConfig(cfg *sconfig.HttpLogging) {
	c.expiration = cfg.GetRetention()
	c.recordFullRequest = cfg.GetFullRequestRecording() == sconfig.FullRequestRecordingAlways
	c.maxFullRequestSize = cfg.GetMaxRequestSize()
	c.maxFullResponseSize = cfg.GetMaxResponseSize()
	c.maxResponseWait = cfg.GetMaxResponseWait()
	c.fullRequestExpiration = cfg.GetFullRequestRetention()
}

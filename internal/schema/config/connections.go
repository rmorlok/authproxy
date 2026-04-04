package config

import "time"

const DefaultSetupTtl = 24 * time.Hour

// Connections contains configuration for connection management.
type Connections struct {
	// SetupTtl is the maximum time a connection can remain in an incomplete setup state
	// before it is automatically cleaned up. Defaults to 24 hours.
	SetupTtl *HumanDuration `json:"setup_ttl,omitempty" yaml:"setup_ttl,omitempty"`
}

// GetSetupTtlOrDefault returns the configured setup TTL, or 24 hours if not configured.
func (c *Connections) GetSetupTtlOrDefault() time.Duration {
	if c == nil || c.SetupTtl == nil {
		return DefaultSetupTtl
	}
	return c.SetupTtl.Duration
}

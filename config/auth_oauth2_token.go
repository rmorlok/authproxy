package config

import "time"

type AuthOauth2Token struct {
	Endpoint       string            `json:"endpoint" yaml:"endpoint"`
	QueryOverrides map[string]string `json:"query_overrides,omitempty" yaml:"query_overrides,omitempty"`
	FormOverrides  map[string]string `json:"form_overrides,omitempty" yaml:"form_overrides,omitempty"`

	// RefreshTimeout is how long to time out the HTTP request to refresh a token. Default is 30s if not specified.
	RefreshTimeout *HumanDuration `json:"refresh_timeout,omitempty" yaml:"refresh_timeout,omitempty"`
}

func (a *AuthOauth2Token) GetRefreshTimeout() time.Duration {
	if a == nil || a.RefreshTimeout == nil {
		return 30 * time.Second
	}

	return a.RefreshTimeout.Duration
}

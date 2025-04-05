package config

import "time"

type AuthOauth2Token struct {
	Endpoint       string            `json:"endpoint" yaml:"endpoint"`
	QueryOverrides map[string]string `json:"query_overrides,omitempty" yaml:"query_overrides,omitempty"`
	FormOverrides  map[string]string `json:"form_overrides,omitempty" yaml:"form_overrides,omitempty"`

	// RefreshTimeout is how long to time out the HTTP request to refresh a token. Default is 30s if not specified.
	RefreshTimeout *HumanDuration `json:"refresh_timeout,omitempty" yaml:"refresh_timeout,omitempty"`

	// RefreshInBackground controls if the system should proactively refresh tokens in the background. Default
	// value is `true`. If set to false, tokens will not be refreshed until they are detected to be expired when used.
	// The global setting for refreshing in the background must also be enabled for this to work.
	RefreshInBackground *bool `json:"refresh_in_background,omitempty" yaml:"refresh_in_background,omitempty"`

	// RefreshTimeBeforeExpiry is the time prior to token expiry to refresh the tokens. The granularity of this setting
	// is limited by the global cron schedule for running refresh. If not specified the global value or default is used.
	RefreshTimeBeforeExpiry *HumanDuration `json:"refresh_time_before_expiry,omitempty" yaml:"refresh_time_before_expiry,omitempty"`
}

func (a *AuthOauth2Token) GetRefreshTimeout() time.Duration {
	if a == nil || a.RefreshTimeout == nil {
		return 30 * time.Second
	}

	return a.RefreshTimeout.Duration
}

func (a *AuthOauth2Token) GetRefreshInBackgroundOrDefault() bool {
	if a == nil || a.RefreshInBackground == nil {
		return true
	}
	return *a.RefreshInBackground
}

func (a *AuthOauth2Token) GetRefreshTimeBeforeExpiryOrDefault(fallback time.Duration) time.Duration {
	if a == nil || a.RefreshTimeBeforeExpiry == nil {
		return fallback
	}

	return a.RefreshTimeBeforeExpiry.Duration
}

package config

import (
	"log"
	"time"
)

const (
	DefaultInitiateToRedirectTtl = 30 * time.Second
	DefaultOAuthRoundTripTtl     = 1 * time.Hour
)

type OAuth struct {
	// InitiateToRedirectTtl is the time allowed between the oauth initiate API call, and the time when the browser
	// completes the redirect from the auth proxy public service. This value must be less than RoundTripTtl. This value
	// should be as small as possible as the handoff from the API to the redirect involves a one-time-use auth token
	// in the query parameters, which could be used to steal the session.
	InitiateToRedirectTtl HumanDuration `json:"initiate_to_redirect_ttl" yaml:"initiate_to_redirect_ttl"`

	// RoundTripTtl is the time we allow for the user to go through the oauth flow, from the initiate call, all the
	// way back to returning to AuthProxy to exchange the auth token for an access token. The purpose of this timeout
	// is to reduce the time that a redirect link from auth proxy would be valid for the purposes of phishing other
	// peoples credentials using this link as the basis.
	RoundTripTtl HumanDuration `json:"round_trip_ttl" yaml:"round_trip_ttl"`

	// RefreshTokensInBackground controls if the system should proactively refresh tokens in the background. Default
	// value is `true`. If set to false, tokens will not be refreshed until they are detected to be expired when used.
	RefreshTokensInBackground *bool `json:"refresh_tokens_in_background" yaml:"refresh_tokens_in_background"`

	// RefreshTokensTimeBeforeExpiry is the default time prior to token expiry to refresh the tokens. This value can be
	// overridden on a per-connector basis, but the granularity of this value is limited by the cron for running refresh.
	// If not specified the default value is 10 minutes.
	RefreshTokensTimeBeforeExpiry *HumanDuration `json:"refresh_tokens_time_before_expiry" yaml:"refresh_tokens_time_before_expiry"`

	// RefreshTokensCronSchedule is the schedule at which the background job to refresh oauth tokens will run. If not
	// specified, runs every 10 minutes.
	RefreshTokensCronSchedule string `json:"refresh_tokens_cron_schedule" yaml:"refresh_tokens_cron_schedule"`
}

func (o *OAuth) GetRoundTripTtlOrDefault() time.Duration {
	if o == nil || o.RoundTripTtl.Duration == 0 {
		return DefaultOAuthRoundTripTtl
	}

	return o.RoundTripTtl.Duration
}

func (o *OAuth) GetInitiateToRedirectTtlOrDefault() time.Duration {
	roundTripTtl := o.GetRoundTripTtlOrDefault()
	initiateToRedirectTtl := DefaultInitiateToRedirectTtl

	if o != nil && o.InitiateToRedirectTtl.Duration != 0 {
		initiateToRedirectTtl = o.InitiateToRedirectTtl.Duration
	}

	if roundTripTtl < initiateToRedirectTtl {
		// Log a warning if the round-trip TTL is less than the initiate-to-redirect TTL.
		if roundTripTtl < initiateToRedirectTtl {
			log.Printf("Warning: RoundTripTtl (%v) is less than InitiateToRedirectTtl (%v). Using RoundTripTtl value.", roundTripTtl, initiateToRedirectTtl)
			return roundTripTtl
		}
	}

	return initiateToRedirectTtl
}

func (o *OAuth) GetRefreshTokensInBackgroundOrDefault() bool {
	if o == nil || o.RefreshTokensInBackground == nil {
		return true
	}

	return *o.RefreshTokensInBackground
}

func (o *OAuth) GetRefreshTokensTimeBeforeExpiryOrDefault() time.Duration {
	if o == nil || o.RefreshTokensTimeBeforeExpiry == nil {
		return 10 * time.Minute
	}

	return o.RefreshTokensTimeBeforeExpiry.Duration
}

func (o *OAuth) GetRefreshTokensCronScheduleOrDefault() string {
	if o == nil || o.RefreshTokensCronSchedule == "" {
		return "*/10 * * * *"
	}

	return o.RefreshTokensCronSchedule
}

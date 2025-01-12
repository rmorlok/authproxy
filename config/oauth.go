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

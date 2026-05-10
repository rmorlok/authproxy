package request_log

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
)

// ResponseSource identifies *who* produced the response that a LogRecord
// captures. Until this PR every 429 in the log looked the same — a 3rd
// party rate-limiting us was indistinguishable from the connector-level
// reactive backoff returning a synthetic 429. Stamping this field at the
// point of synthesis closes that gap and reserves a value for the
// upcoming proxy-side rate-limit resource enforcement (#223).
type ResponseSource string

const (
	// ResponseSourceUpstream means the response (including any 429) came
	// from the 3rd-party service — the default and historically the only
	// possibility.
	ResponseSourceUpstream ResponseSource = "upstream"

	// ResponseSourceConnectorRateLimiter means the connector-level reactive
	// limiter short-circuited the request because the connection is in
	// cool-down from a prior real upstream 429. No upstream call was made
	// for this attempt.
	ResponseSourceConnectorRateLimiter ResponseSource = "connector_rate_limiter"

	// ResponseSourceRateLimit is reserved for the proxy-side rate-limit
	// resource enforcement (#223). Defining it here keeps the schema
	// stable across the two PRs; population happens in #223.
	ResponseSourceRateLimit ResponseSource = "rate_limit"
)

// IsValidResponseSource reports whether s is a recognised ResponseSource.
// Unknown values returned from old DB rows or from forward-compatible
// readers are not coerced — callers can choose to treat them as upstream
// or to log a warning.
func IsValidResponseSource(s ResponseSource) bool {
	switch s {
	case ResponseSourceUpstream,
		ResponseSourceConnectorRateLimiter,
		ResponseSourceRateLimit:
		return true
	}
	return false
}

// RateLimitMatch describes a single rate-limit rule that matched the
// request. The full set of matches is captured in
// LogRecord.RateLimitMatched so observers can see every rule that fired
// (in either enforce or observe mode), not just the most-restrictive one
// that ultimately decided the request's fate.
type RateLimitMatch struct {
	Id     apid.ID           `json:"id"`
	Mode   string            `json:"mode"`
	Bucket map[string]string `json:"bucket,omitempty"`
}

// Attribution carries per-request log attribution that the proxy stack
// populates as the request flows through it. The rate-limit
// round-tripper writes Source when it short-circuits with a synthetic
// 429; the (future) rate-limit resource enforcement writes the
// RateLimit* fields when a rule fires. The request-log round-tripper
// reads the value on its way back to populate LogRecord.
//
// Use a pointer in the context so middlewares writing to the same
// Attribution see each other's edits — the pointer doesn't change, the
// struct fields do.
type Attribution struct {
	Source           ResponseSource
	RateLimitId      apid.ID
	RateLimitMode    string
	RateLimitBucket  map[string]string
	RateLimitMatched []RateLimitMatch
}

type attributionKey struct{}

// ContextWithAttribution returns a context that carries attr. The proxy
// bootstrap installs an empty Attribution on the request context before
// the round-tripper chain runs; middlewares mutate the struct in place.
func ContextWithAttribution(ctx context.Context, attr *Attribution) context.Context {
	if attr == nil {
		return ctx
	}
	return context.WithValue(ctx, attributionKey{}, attr)
}

// AttributionFromContext returns the Attribution carried by ctx, or nil
// when no proxy middleware has installed one. Returning nil rather than
// a zero-value struct lets callers distinguish "nothing was stamped" from
// "stamped but blank" — the LogRecord-population path treats nil as
// ResponseSourceUpstream.
func AttributionFromContext(ctx context.Context) *Attribution {
	if ctx == nil {
		return nil
	}
	a, _ := ctx.Value(attributionKey{}).(*Attribution)
	return a
}

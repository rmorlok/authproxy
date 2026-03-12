package connectors

import (
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"time"
)

const (
	DefaultMaxRetryAfter        = 15 * time.Minute
	DefaultDefaultRetryAfter    = 60 * time.Second
	DefaultBackoffInitial       = 1 * time.Second
	DefaultBackoffMultiplier    = 2.0
	DefaultBackoffMaxInterval   = 5 * time.Minute
	DefaultBackoffJitterFraction = 0.1
)

// RateLimiting configures how 429 rate limiting responses from the 3rd party are handled. When a 429 is received,
// the system will parse retry-after information from the response headers and block subsequent requests for that
// connection until the retry-after period has elapsed.
type RateLimiting struct {
	// Disabled turns off 429-based rate limiting entirely. When true, 429 responses are passed through
	// to the caller without any blocking of future requests.
	Disabled bool `json:"disabled,omitempty" yaml:"disabled,omitempty"`

	// RetryAfterHeaders is an ordered list of response header names to check for retry-after information.
	// The system checks headers in order and uses the first one that contains a parseable value.
	// Supported value formats: integer seconds, HTTP-date (RFC 7231), ISO 8601 timestamp.
	// Defaults to ["Retry-After"] if unset.
	RetryAfterHeaders []string `json:"retry_after_headers,omitempty" yaml:"retry_after_headers,omitempty"`

	// MaxRetryAfter caps the maximum backoff duration to prevent unreasonably long waits.
	// Defaults to 15 minutes if unset.
	MaxRetryAfter *common.HumanDuration `json:"max_retry_after,omitempty" yaml:"max_retry_after,omitempty"`

	// DefaultRetryAfter is used when a 429 is received but no parseable retry-after header is found
	// and exponential backoff is not configured. Defaults to 60 seconds if unset.
	DefaultRetryAfter *common.HumanDuration `json:"default_retry_after,omitempty" yaml:"default_retry_after,omitempty"`

	// ExponentialBackoff configures backoff behavior when no retry-after header is present and
	// consecutive 429s are received. If unset, DefaultRetryAfter is used as a flat backoff.
	ExponentialBackoff *ExponentialBackoff `json:"exponential_backoff,omitempty" yaml:"exponential_backoff,omitempty"`
}

// ExponentialBackoff configures exponential backoff for 429 responses that lack retry-after headers.
type ExponentialBackoff struct {
	// InitialInterval is the starting backoff duration. Defaults to 1 second.
	InitialInterval *common.HumanDuration `json:"initial_interval,omitempty" yaml:"initial_interval,omitempty"`

	// Multiplier is the backoff multiplier applied for each consecutive 429. Defaults to 2.0.
	Multiplier *float64 `json:"multiplier,omitempty" yaml:"multiplier,omitempty"`

	// MaxInterval caps the maximum backoff interval. Defaults to 5 minutes.
	MaxInterval *common.HumanDuration `json:"max_interval,omitempty" yaml:"max_interval,omitempty"`

	// JitterFraction adds randomness to the backoff duration (0.0 to 1.0). The actual backoff will be
	// between (1-jitter)*computed and (1+jitter)*computed. Defaults to 0.1.
	JitterFraction *float64 `json:"jitter_fraction,omitempty" yaml:"jitter_fraction,omitempty"`
}

func (r *RateLimiting) Clone() *RateLimiting {
	if r == nil {
		return nil
	}

	clone := *r

	if r.RetryAfterHeaders != nil {
		clone.RetryAfterHeaders = make([]string, len(r.RetryAfterHeaders))
		copy(clone.RetryAfterHeaders, r.RetryAfterHeaders)
	}

	if r.MaxRetryAfter != nil {
		v := *r.MaxRetryAfter
		clone.MaxRetryAfter = &v
	}

	if r.DefaultRetryAfter != nil {
		v := *r.DefaultRetryAfter
		clone.DefaultRetryAfter = &v
	}

	if r.ExponentialBackoff != nil {
		clone.ExponentialBackoff = r.ExponentialBackoff.Clone()
	}

	return &clone
}

func (r *RateLimiting) Validate(vc *common.ValidationContext) error {
	if r == nil {
		return nil
	}

	result := &multierror.Error{}

	if r.MaxRetryAfter != nil && r.MaxRetryAfter.Duration <= 0 {
		result = multierror.Append(result, vc.PushField("max_retry_after").NewError("must be positive"))
	}

	if r.DefaultRetryAfter != nil && r.DefaultRetryAfter.Duration <= 0 {
		result = multierror.Append(result, vc.PushField("default_retry_after").NewError("must be positive"))
	}

	if r.ExponentialBackoff != nil {
		if err := r.ExponentialBackoff.Validate(vc.PushField("exponential_backoff")); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result.ErrorOrNil()
}

// GetRetryAfterHeaders returns the configured retry-after headers, defaulting to ["Retry-After"].
func (r *RateLimiting) GetRetryAfterHeaders() []string {
	if r == nil || len(r.RetryAfterHeaders) == 0 {
		return []string{"Retry-After"}
	}
	return r.RetryAfterHeaders
}

// GetMaxRetryAfter returns the configured max retry-after duration, defaulting to 15 minutes.
func (r *RateLimiting) GetMaxRetryAfter() time.Duration {
	if r == nil || r.MaxRetryAfter == nil {
		return DefaultMaxRetryAfter
	}
	return r.MaxRetryAfter.Duration
}

// GetDefaultRetryAfter returns the configured default retry-after duration, defaulting to 60 seconds.
func (r *RateLimiting) GetDefaultRetryAfter() time.Duration {
	if r == nil || r.DefaultRetryAfter == nil {
		return DefaultDefaultRetryAfter
	}
	return r.DefaultRetryAfter.Duration
}

func (eb *ExponentialBackoff) Clone() *ExponentialBackoff {
	if eb == nil {
		return nil
	}

	clone := *eb

	if eb.InitialInterval != nil {
		v := *eb.InitialInterval
		clone.InitialInterval = &v
	}

	if eb.Multiplier != nil {
		v := *eb.Multiplier
		clone.Multiplier = &v
	}

	if eb.MaxInterval != nil {
		v := *eb.MaxInterval
		clone.MaxInterval = &v
	}

	if eb.JitterFraction != nil {
		v := *eb.JitterFraction
		clone.JitterFraction = &v
	}

	return &clone
}

func (eb *ExponentialBackoff) Validate(vc *common.ValidationContext) error {
	if eb == nil {
		return nil
	}

	result := &multierror.Error{}

	if eb.InitialInterval != nil && eb.InitialInterval.Duration <= 0 {
		result = multierror.Append(result, vc.PushField("initial_interval").NewError("must be positive"))
	}

	if eb.Multiplier != nil && *eb.Multiplier <= 0 {
		result = multierror.Append(result, vc.PushField("multiplier").NewError("must be positive"))
	}

	if eb.MaxInterval != nil && eb.MaxInterval.Duration <= 0 {
		result = multierror.Append(result, vc.PushField("max_interval").NewError("must be positive"))
	}

	if eb.JitterFraction != nil && (*eb.JitterFraction < 0 || *eb.JitterFraction > 1) {
		result = multierror.Append(result, vc.PushField("jitter_fraction").NewError("must be between 0.0 and 1.0"))
	}

	return result.ErrorOrNil()
}

// GetInitialInterval returns the configured initial interval, defaulting to 1 second.
func (eb *ExponentialBackoff) GetInitialInterval() time.Duration {
	if eb == nil || eb.InitialInterval == nil {
		return DefaultBackoffInitial
	}
	return eb.InitialInterval.Duration
}

// GetMultiplier returns the configured multiplier, defaulting to 2.0.
func (eb *ExponentialBackoff) GetMultiplier() float64 {
	if eb == nil || eb.Multiplier == nil {
		return DefaultBackoffMultiplier
	}
	return *eb.Multiplier
}

// GetMaxInterval returns the configured max interval, defaulting to 5 minutes.
func (eb *ExponentialBackoff) GetMaxInterval() time.Duration {
	if eb == nil || eb.MaxInterval == nil {
		return DefaultBackoffMaxInterval
	}
	return eb.MaxInterval.Duration
}

// GetJitterFraction returns the configured jitter fraction, defaulting to 0.1.
func (eb *ExponentialBackoff) GetJitterFraction() float64 {
	if eb == nil || eb.JitterFraction == nil {
		return DefaultBackoffJitterFraction
	}
	return *eb.JitterFraction
}

package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

// RateLimitJson is the API envelope around a rate-limit resource definition.
//
//	@Description	Rate-limit API response
type RateLimitJson struct {
	Id          apid.ID            `json:"id" yaml:"id" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace   string             `json:"namespace" yaml:"namespace" example:"root.acme"`
	Definition  rlschema.RateLimit `json:"definition" yaml:"definition"`
	Labels      map[string]string  `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time          `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at" yaml:"updated_at"`
}

type ListRateLimitsResponseJson struct {
	Items  []RateLimitJson `json:"items" yaml:"items"`
	Cursor string          `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

// CreateRateLimitRequestJson is the request body for POST /rate-limits.
//
//	@Description	Request to create a rate limit
type CreateRateLimitRequestJson struct {
	Namespace   string             `json:"namespace" yaml:"namespace" example:"root.acme"`
	Definition  rlschema.RateLimit `json:"definition" yaml:"definition"`
	Labels      map[string]string  `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// UpdateRateLimitRequestJson is the request body for PATCH /rate-limits/:id.
//
//	@Description	Request to update a rate limit
type UpdateRateLimitRequestJson struct {
	Definition  *rlschema.RateLimit `json:"definition,omitempty" yaml:"definition,omitempty"`
	Labels      *map[string]string  `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations *map[string]string  `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// ProxyRequestJson is the wire shape used by API endpoints that accept a
// synthetic proxy request, such as rate-limit dry-run.
//
//	@Description	Request to proxy or simulate an HTTP request
type ProxyRequestJson struct {
	URL      string            `json:"url" yaml:"url" example:"https://api.example.com/v1/users"`
	Method   string            `json:"method" yaml:"method" example:"GET"`
	Headers  map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Labels   map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	BodyRaw  []byte            `json:"body_raw,omitempty" yaml:"body_raw,omitempty"`
	BodyJson interface{}       `json:"body_json,omitempty" yaml:"body_json,omitempty"`
}

// DryRunRequestJson is the request body for POST /rate-limits/_dry_run.
//
//	@Description	Request to simulate rate-limit matching
type DryRunRequestJson struct {
	Request     ProxyRequestJson  `json:"request" yaml:"request"`
	RequestType string            `json:"request_type" yaml:"request_type" example:"proxy"`
	Context     DryRunContextJson `json:"context" yaml:"context"`
}

type DryRunContextJson struct {
	ConnectionId *apid.ID `json:"connection_id,omitempty" yaml:"connection_id,omitempty" swaggertype:"string"`
	ActorId      *apid.ID `json:"actor_id,omitempty" yaml:"actor_id,omitempty" swaggertype:"string"`
	Namespace    *string  `json:"namespace,omitempty" yaml:"namespace,omitempty" example:"root.acme"`
}

type DryRunResponseJson struct {
	RequestLabelSnapshot map[string]string      `json:"request_label_snapshot" yaml:"request_label_snapshot"`
	Matched              []DryRunMatchJson      `json:"matched" yaml:"matched"`
	NotMatched           []DryRunNotMatchedJson `json:"not_matched" yaml:"not_matched"`
}

type DryRunMatchJson struct {
	RateLimitId      apid.ID `json:"rate_limit_id" yaml:"rate_limit_id" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace        string  `json:"namespace" yaml:"namespace" example:"root.acme"`
	EffectiveMode    string  `json:"effective_mode" yaml:"effective_mode" example:"enforce"`
	BucketKey        string  `json:"bucket_key" yaml:"bucket_key" example:"rate_limit:rl_test550e8400abcde:actor:act_test"`
	AlgorithmSummary string  `json:"algorithm_summary" yaml:"algorithm_summary" example:"fixed_window window=1m limit=100"`
	WouldAllow       bool    `json:"would_allow" yaml:"would_allow" example:"true"`
	Remaining        int     `json:"remaining" yaml:"remaining" example:"99"`
	RetryAfterMs     int64   `json:"retry_after_ms" yaml:"retry_after_ms" example:"0"`
	PeekFailed       bool    `json:"peek_failed" yaml:"peek_failed" example:"false"`
}

type DryRunNotMatchedJson struct {
	RateLimitId apid.ID `json:"rate_limit_id" yaml:"rate_limit_id" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace   string  `json:"namespace" yaml:"namespace" example:"root.acme"`
	Reason      string  `json:"reason" yaml:"reason" example:"method did not match"`
}

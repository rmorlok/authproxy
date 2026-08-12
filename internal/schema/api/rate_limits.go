package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

// RateLimitJson is the API envelope around a rate-limit resource definition.
//
//	@Description	Rate-limit API response
type RateLimitJson struct {
	Id          apid.ID             `json:"id" yaml:"id" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace   string              `json:"namespace" yaml:"namespace" example:"root.acme"`
	Name        common.ResourceName `json:"name" yaml:"name" swaggertype:"string" example:"public-api"`
	Definition  rlschema.RateLimit  `json:"definition" yaml:"definition"`
	Labels      map[string]string   `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string   `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time           `json:"createdAt" yaml:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt" yaml:"updatedAt"`
}

type ListRateLimitsResponseJson struct {
	Items  []RateLimitJson `json:"items" yaml:"items"`
	Cursor string          `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

// CreateRateLimitRequestJson is the request body for POST /rate-limits.
//
//	@Description	Request to create a rate limit
type CreateRateLimitRequestJson struct {
	Namespace   string               `json:"namespace" yaml:"namespace" example:"root.acme"`
	Name        *common.ResourceName `json:"name,omitempty" yaml:"name,omitempty" swaggertype:"string" example:"public-api"`
	Definition  rlschema.RateLimit   `json:"definition" yaml:"definition"`
	Labels      map[string]string    `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string    `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// UpdateRateLimitRequestJson is the request body for PATCH /rate-limits/:id.
//
//	@Description	Request to update a rate limit
type UpdateRateLimitRequestJson struct {
	Name        *common.ResourceName `json:"name,omitempty" yaml:"name,omitempty" swaggertype:"string" example:"public-api"`
	Definition  *rlschema.RateLimit  `json:"definition,omitempty" yaml:"definition,omitempty"`
	Labels      *map[string]string   `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations *map[string]string   `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// ProxyRequestJson is the wire shape used by API endpoints that accept a
// synthetic proxy request, such as rate-limit dry-run.
//
//	@Description	Request to proxy or simulate an HTTP request
type ProxyRequestJson struct {
	URL      string                       `json:"url" yaml:"url" example:"https://api.example.com/v1/users"`
	Method   string                       `json:"method" yaml:"method" example:"GET"`
	Headers  map[string]common.HeadersVal `json:"headers,omitempty" yaml:"headers,omitempty" swaggertype:"object"`
	Labels   map[string]string            `json:"labels,omitempty" yaml:"labels,omitempty"`
	BodyRaw  []byte                       `json:"bodyRaw,omitempty" yaml:"bodyRaw,omitempty"`
	BodyJson interface{}                  `json:"bodyJson,omitempty" yaml:"bodyJson,omitempty"`
}

// DryRunRequestJson is the request body for POST /rate-limits/_dry_run.
//
//	@Description	Request to simulate rate-limit matching
type DryRunRequestJson struct {
	Request     ProxyRequestJson  `json:"request" yaml:"request"`
	RequestType string            `json:"requestType" yaml:"requestType" example:"proxy"`
	Context     DryRunContextJson `json:"context" yaml:"context"`
}

type DryRunContextJson struct {
	ConnectionId *apid.ID `json:"connectionId,omitempty" yaml:"connectionId,omitempty" swaggertype:"string"`
	ActorId      *apid.ID `json:"actorId,omitempty" yaml:"actorId,omitempty" swaggertype:"string"`
	Namespace    *string  `json:"namespace,omitempty" yaml:"namespace,omitempty" example:"root.acme"`
}

type DryRunResponseJson struct {
	RequestLabelSnapshot map[string]string      `json:"requestLabelSnapshot" yaml:"requestLabelSnapshot"`
	Matched              []DryRunMatchJson      `json:"matched" yaml:"matched"`
	NotMatched           []DryRunNotMatchedJson `json:"notMatched" yaml:"notMatched"`
}

type DryRunMatchJson struct {
	RateLimitId      apid.ID `json:"rateLimitId" yaml:"rateLimitId" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace        string  `json:"namespace" yaml:"namespace" example:"root.acme"`
	EffectiveMode    string  `json:"effectiveMode" yaml:"effectiveMode" example:"enforce"`
	BucketKey        string  `json:"bucketKey" yaml:"bucketKey" example:"rate_limit:rl_test550e8400abcde:actor:act_test"`
	AlgorithmSummary string  `json:"algorithmSummary" yaml:"algorithmSummary" example:"fixed_window window=1m limit=100"`
	WouldAllow       bool    `json:"wouldAllow" yaml:"wouldAllow" example:"true"`
	Remaining        int     `json:"remaining" yaml:"remaining" example:"99"`
	RetryAfterMs     int64   `json:"retryAfterMs" yaml:"retryAfterMs" example:"0"`
	PeekFailed       bool    `json:"peekFailed" yaml:"peekFailed" example:"false"`
}

type DryRunNotMatchedJson struct {
	RateLimitId apid.ID `json:"rateLimitId" yaml:"rateLimitId" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace   string  `json:"namespace" yaml:"namespace" example:"root.acme"`
	Reason      string  `json:"reason" yaml:"reason" example:"method did not match"`
}

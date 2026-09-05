package api

import (
	"github.com/rmorlok/authproxy/internal/apid"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

type ListRateLimitsResponseJson struct {
	apiv1alpha1.ResourceList[rlschema.RateLimit] `json:",inline" yaml:",inline"`
}

func NewListRateLimitsResponseJson(
	items []rlschema.RateLimit,
	continueToken string,
) ListRateLimitsResponseJson {
	return ListRateLimitsResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			rlschema.RateLimitKind,
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
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

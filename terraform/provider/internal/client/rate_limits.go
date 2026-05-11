package client

import (
	"context"
	"fmt"
	"time"
)

// Wire models for the rate-limit resource. These mirror the server-side
// `routes.RateLimitJson` and `rlschema.RateLimit` shapes, but are defined
// locally so the TF provider binary doesn't pull in the full internal
// schema package. Field names use the same `json:` tags the server emits.

type RateLimitPathMatch struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type RateLimitSelector struct {
	LabelSelector string              `json:"label_selector,omitempty"`
	Methods       []string            `json:"methods,omitempty"`
	PathMatch     *RateLimitPathMatch `json:"path_match,omitempty"`
	RequestTypes  []string            `json:"request_types,omitempty"`
}

type RateLimitBucket struct {
	Dimensions []string `json:"dimensions,omitempty"`
}

type RateLimitFixedWindow struct {
	Window string `json:"window"`
	Limit  int    `json:"limit"`
}

type RateLimitSlidingWindow struct {
	Window string `json:"window"`
	Limit  int    `json:"limit"`
	Mode   string `json:"mode"`
}

type RateLimitTokenBucket struct {
	Capacity   int     `json:"capacity"`
	RefillRate float64 `json:"refill_rate"`
}

// RateLimitAlgorithm is a tagged union: exactly one variant is set per
// rule. The server's schema validator enforces this at write time; the
// provider-side ConfigValidator on the resource enforces it at plan time
// so authors see the error before \`terraform apply\`.
type RateLimitAlgorithm struct {
	FixedWindow   *RateLimitFixedWindow   `json:"fixed_window,omitempty"`
	SlidingWindow *RateLimitSlidingWindow `json:"sliding_window,omitempty"`
	TokenBucket   *RateLimitTokenBucket   `json:"token_bucket,omitempty"`
}

// RateLimitDefinition is the JSON-serialised "definition" payload of a
// RateLimit resource.
type RateLimitDefinition struct {
	Mode      string             `json:"mode,omitempty"`
	Selector  RateLimitSelector  `json:"selector"`
	Bucket    RateLimitBucket    `json:"bucket"`
	Algorithm RateLimitAlgorithm `json:"algorithm"`
}

// RateLimit is the server's RateLimitJson envelope.
type RateLimit struct {
	Id          string              `json:"id"`
	Namespace   string              `json:"namespace"`
	Definition  RateLimitDefinition `json:"definition"`
	Labels      map[string]string   `json:"labels,omitempty"`
	Annotations map[string]string   `json:"annotations,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type CreateRateLimitRequest struct {
	Namespace   string              `json:"namespace"`
	Definition  RateLimitDefinition `json:"definition"`
	Labels      map[string]string   `json:"labels,omitempty"`
	Annotations map[string]string   `json:"annotations,omitempty"`
}

// UpdateRateLimitRequest uses pointers so the provider can send partial
// updates (only changed fields land in the body). nil = "no change".
type UpdateRateLimitRequest struct {
	Definition  *RateLimitDefinition `json:"definition,omitempty"`
	Labels      *map[string]string   `json:"labels,omitempty"`
	Annotations *map[string]string   `json:"annotations,omitempty"`
}

func (c *Client) CreateRateLimit(ctx context.Context, req CreateRateLimitRequest) (*RateLimit, error) {
	var rl RateLimit
	err := c.post(ctx, "/api/v1/rate-limits", req, &rl)
	return &rl, err
}

func (c *Client) GetRateLimit(ctx context.Context, id string) (*RateLimit, error) {
	var rl RateLimit
	err := c.get(ctx, fmt.Sprintf("/api/v1/rate-limits/%s", id), &rl)
	return &rl, err
}

func (c *Client) UpdateRateLimit(ctx context.Context, id string, req UpdateRateLimitRequest) (*RateLimit, error) {
	var rl RateLimit
	err := c.patch(ctx, fmt.Sprintf("/api/v1/rate-limits/%s", id), req, &rl)
	return &rl, err
}

func (c *Client) DeleteRateLimit(ctx context.Context, id string) error {
	return c.delete(ctx, fmt.Sprintf("/api/v1/rate-limits/%s", id))
}

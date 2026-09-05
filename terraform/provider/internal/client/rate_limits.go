package client

import (
	"context"
	"fmt"
	"time"
)

const (
	RateLimitAPIVersion = "authproxy.net/v1alpha1"
	RateLimitKind       = "RateLimit"
)

// These local wire models keep the Terraform provider independent from the
// server's internal packages while mirroring its canonical resource contract.
type RateLimitPathMatch struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type RateLimitSelector struct {
	LabelSelector string              `json:"labelSelector,omitempty"`
	Methods       []string            `json:"methods,omitempty"`
	PathMatch     *RateLimitPathMatch `json:"pathMatch,omitempty"`
	RequestTypes  []string            `json:"requestTypes,omitempty"`
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
	RefillRate float64 `json:"refillRate"`
}

type RateLimitAlgorithm struct {
	FixedWindow   *RateLimitFixedWindow   `json:"fixedWindow,omitempty"`
	SlidingWindow *RateLimitSlidingWindow `json:"slidingWindow,omitempty"`
	TokenBucket   *RateLimitTokenBucket   `json:"tokenBucket,omitempty"`
}

type ObjectReference struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	ID         string `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
}

type RateLimitScope struct {
	NamespaceMatcher string           `json:"namespaceMatcher,omitempty"`
	ConnectorRef     *ObjectReference `json:"connectorRef,omitempty"`
	ConnectionRef    *ObjectReference `json:"connectionRef,omitempty"`
}

type RateLimitSpec struct {
	Scope     *RateLimitScope    `json:"scope,omitempty"`
	Mode      string             `json:"mode,omitempty"`
	Selector  RateLimitSelector  `json:"selector"`
	Bucket    RateLimitBucket    `json:"bucket"`
	Algorithm RateLimitAlgorithm `json:"algorithm"`
}

type RateLimitMetadata struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   *time.Time        `json:"createdAt,omitempty"`
	UpdatedAt   *time.Time        `json:"updatedAt,omitempty"`
}

type RateLimitMetadataPatch struct {
	Name        *string            `json:"name,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty"`
}

type RateLimitStatus struct {
	EffectiveMode string `json:"effectiveMode"`
}

type RateLimit struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   RateLimitMetadata `json:"metadata"`
	Spec       RateLimitSpec     `json:"spec"`
	Status     *RateLimitStatus  `json:"status,omitempty"`
}

type CreateRateLimitRequest struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   RateLimitMetadata `json:"metadata"`
	Spec       RateLimitSpec     `json:"spec"`
}

type RateLimitSpecPatch struct {
	// Scope is intentionally not omitted when nil. Provider updates send the
	// complete desired spec, so a nil scope must be encoded as JSON null to
	// clear a previously configured namespace matcher or resource reference.
	Scope     *RateLimitScope     `json:"scope"`
	Mode      *string             `json:"mode,omitempty"`
	Selector  *RateLimitSelector  `json:"selector,omitempty"`
	Bucket    *RateLimitBucket    `json:"bucket,omitempty"`
	Algorithm *RateLimitAlgorithm `json:"algorithm,omitempty"`
}

type UpdateRateLimitRequest struct {
	APIVersion string                  `json:"apiVersion"`
	Kind       string                  `json:"kind"`
	Metadata   *RateLimitMetadataPatch `json:"metadata"`
	Spec       *RateLimitSpecPatch     `json:"spec"`
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

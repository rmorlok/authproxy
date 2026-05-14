package iface

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	rlschema "github.com/rmorlok/authproxy/internal/schema/rate_limit"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// RateLimit is the core abstraction for a rate-limit resource.
type RateLimit interface {
	GetId() apid.ID
	GetNamespace() string
	GetDefinition() rlschema.RateLimit
	GetLabels() map[string]string
	GetAnnotations() map[string]string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
}

type ListRateLimitsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[RateLimit]
	Enumerate(context.Context, pagination.EnumerateCallback[RateLimit]) error
}

type ListRateLimitsBuilder interface {
	ListRateLimitsExecutor
	Limit(int32) ListRateLimitsBuilder
	ForNamespaceMatcher(matcher string) ListRateLimitsBuilder
	ForNamespaceMatchers(matchers []string) ListRateLimitsBuilder
	OrderBy(database.RateLimitOrderByField, pagination.OrderBy) ListRateLimitsBuilder
	IncludeDeleted() ListRateLimitsBuilder
	ForLabelSelector(selector string) ListRateLimitsBuilder
}

// DryRunRateLimitRequest is the input to C.DryRunRateLimit. Mirrors the
// fields a real proxy request would carry (so the matcher sees the same
// shape) plus the identity / context that drives label resolution.
type DryRunRateLimitRequest struct {
	// Request describes the HTTP call to simulate.
	Request DryRunRequestPayload

	// Context is the identity the request runs under. When ConnectionId
	// is set, the implementation hydrates Namespace + Connector +
	// Labels from the connection (the way httpf.ForConnection does at
	// runtime). Manual Labels always merge on top.
	Context DryRunRequestContext
}

type DryRunRequestPayload struct {
	Method      string
	Path        string
	RequestType string
	Headers     map[string]string
}

type DryRunRequestContext struct {
	ConnectionId *apid.ID
	ActorId      *apid.ID
	Namespace    *string
	Labels       map[string]string
}

// DryRunRateLimitResult is the structured output of C.DryRunRateLimit.
// No JSON tags here — wire-format shaping is the route layer's job.
type DryRunRateLimitResult struct {
	// Namespace is the namespace the dry-run was actually evaluated
	// against (post-hydration). Returned so callers can show the
	// effective scope when input was a Connection.
	Namespace string

	// RequestLabelSnapshot is the per-request label snapshot the
	// matcher saw. Useful for debugging "why didn't my labelSelector
	// match?".
	RequestLabelSnapshot map[string]string

	// Matched is one entry per cached rule whose selector applied,
	// including observe-mode rules.
	Matched []DryRunRateLimitMatch

	// NotMatched is one entry per cached rule that did *not* apply,
	// each carrying a short reason explaining which clause refused.
	NotMatched []DryRunRateLimitNotMatched
}

type DryRunRateLimitMatch struct {
	RateLimitId      apid.ID
	Namespace        string
	EffectiveMode    string
	BucketKey        string
	AlgorithmSummary string
	WouldAllow       bool
	Remaining        int
	RetryAfterMs     int64
	PeekFailed       bool
}

type DryRunRateLimitNotMatched struct {
	RateLimitId apid.ID
	Namespace   string
	Reason      string
}

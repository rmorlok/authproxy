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

// DryRunRateLimitRequest is the input to C.DryRunRateLimit. Reuses
// ProxyRequest so the dry-run accepts the same shape a real proxy call
// does — URL / Method / Headers / Labels / Body. RequestType stays as a
// sibling field because the real proxy path also takes it separately
// from the request payload (Proxy.ProxyRequest(ctx, reqType, req)).
type DryRunRateLimitRequest struct {
	// Request is the proxy-shaped request to simulate. Labels on the
	// request take precedence over labels carried forward from a
	// supplied connection.
	Request ProxyRequest

	// RequestType selects the kind of traffic to simulate (proxy,
	// probe, etc.). Must be one of common.RequestType.
	RequestType string

	// Context is the identity the request runs under. When ConnectionId
	// is set, the implementation hydrates Namespace + Connector +
	// Labels from the connection (the way httpf.ForConnection does at
	// runtime).
	Context DryRunRequestContext
}

type DryRunRequestContext struct {
	ConnectionId *apid.ID
	ActorId      *apid.ID
	Namespace    *string
}

// DryRunRateLimitResult is the structured output of C.DryRunRateLimit.
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

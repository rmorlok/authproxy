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

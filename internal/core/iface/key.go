package iface

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type Key interface {
	GetId() apid.ID
	GetNamespace() string
	GetState() database.KeyState
	GetLabels() map[string]string
	GetAnnotations() map[string]string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
}

type ListKeysExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Key]
	Enumerate(context.Context, pagination.EnumerateCallback[Key]) error
}

type ListKeysBuilder interface {
	ListKeysExecutor
	Limit(int32) ListKeysBuilder
	ForNamespaceMatcher(matcher string) ListKeysBuilder
	ForNamespaceMatchers(matchers []string) ListKeysBuilder
	ForState(database.KeyState) ListKeysBuilder
	OrderBy(database.KeyOrderByField, pagination.OrderBy) ListKeysBuilder
	IncludeDeleted() ListKeysBuilder
	ForLabelSelector(selector string) ListKeysBuilder
}

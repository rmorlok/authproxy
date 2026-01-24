package iface

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type Namespace interface {
	GetPath() string
	GetState() database.NamespaceState
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
}

/*
 * These interfaces wrap the database equivalents so this service can provide decryption and potentially caching.
 */

type ListNamespacesExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Namespace]
	Enumerate(context.Context, func(pagination.PageResult[Namespace]) (keepGoing bool, err error)) error
}

type ListNamespacesBuilder interface {
	ListNamespacesExecutor
	Limit(int32) ListNamespacesBuilder
	ForPathPrefix(string) ListNamespacesBuilder
	ForDepth(depth uint64) ListNamespacesBuilder
	ForChildrenOf(path string) ListNamespacesBuilder
	ForNamespaceMatcher(matcher string) ListNamespacesBuilder
	ForNamespaceMatchers(matchers []string) ListNamespacesBuilder
	ForState(database.NamespaceState) ListNamespacesBuilder
	OrderBy(database.NamespaceOrderByField, pagination.OrderBy) ListNamespacesBuilder
	IncludeDeleted() ListNamespacesBuilder
}

package iface

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type Namespace interface {
	GetPath() string
	GetState() database.NamespaceState
	GetEncryptionKeyId() *apid.ID
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetLabels() map[string]string
	GetAnnotations() map[string]string
}

/*
 * These interfaces wrap the database equivalents so this service can provide decryption and potentially caching.
 */

type ListNamespacesExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Namespace]
	Enumerate(context.Context, pagination.EnumerateCallback[Namespace]) error
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
	ForLabelSelector(selector string) ListNamespacesBuilder
}

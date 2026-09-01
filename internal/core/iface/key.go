package iface

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type Key interface {
	GetId() apid.ID
	GetNamespace() string
	GetName() common.ResourceName
	GetState() keyschema.KeyState
	GetLabels() map[string]string
	GetAnnotations() map[string]string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetResource() *keyschema.Key
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
	ForName(name common.ResourceName) ListKeysBuilder
	ForState(database.KeyState) ListKeysBuilder
	OrderBy(database.KeyOrderByField, pagination.OrderBy) ListKeysBuilder
	IncludeDeleted() ListKeysBuilder
	ForLabelSelector(selector string) ListKeysBuilder
}

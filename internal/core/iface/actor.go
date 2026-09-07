package iface

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	authschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// Actor is the hydrated core view of a persisted Actor resource.
type Actor interface {
	GetId() apid.ID
	GetNamespace() string
	GetName() common.ResourceName
	GetExternalId() string
	GetPermissions() []authschema.Permission
	GetLabels() map[string]string
	GetAnnotations() map[string]string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	CanSelfSign() bool
	GetResource() *actorschema.Actor
}

type ListActorsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Actor]
	Enumerate(context.Context, pagination.EnumerateCallback[Actor]) error
}

type ListActorsBuilder interface {
	ListActorsExecutor
	ForExternalId(externalID string) ListActorsBuilder
	ForName(name common.ResourceName) ListActorsBuilder
	ForNamespaceMatcher(matcher string) ListActorsBuilder
	ForNamespaceMatchers(matchers []string) ListActorsBuilder
	Limit(int32) ListActorsBuilder
	OrderBy(database.ActorOrderByField, pagination.OrderBy) ListActorsBuilder
	IncludeDeleted() ListActorsBuilder
	ForLabelSelector(selector string) ListActorsBuilder
}

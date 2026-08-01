package iface

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type ListConnectionsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Connection]
	Enumerate(context.Context, pagination.EnumerateCallback[Connection]) error
}

type ListConnectionsBuilder interface {
	ListConnectionsExecutor
	Limit(int32) ListConnectionsBuilder
	ForState(database.ConnectionState) ListConnectionsBuilder
	ForStates([]database.ConnectionState) ListConnectionsBuilder
	ForConnectorId(id apid.ID) ListConnectionsBuilder
	ForNamespaceMatcher(matcher string) ListConnectionsBuilder
	ForNamespaceMatchers(matchers []string) ListConnectionsBuilder
	ForName(name common.ResourceName) ListConnectionsBuilder
	OrderBy(database.ConnectionOrderByField, pagination.OrderBy) ListConnectionsBuilder
	IncludeDeleted() ListConnectionsBuilder
	WithDeletedHandling(database.DeletedHandling) ListConnectionsBuilder
	ForLabelSelector(selector string) ListConnectionsBuilder
}

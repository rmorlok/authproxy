package iface

import (
	"context"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type ListConnectionsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Connection]
	Enumerate(context.Context, func(pagination.PageResult[Connection]) (keepGoing bool, err error)) error
}

type ListConnectionsBuilder interface {
	ListConnectionsExecutor
	Limit(int32) ListConnectionsBuilder
	ForState(database.ConnectionState) ListConnectionsBuilder
	ForStates([]database.ConnectionState) ListConnectionsBuilder
	ForNamespaceMatcher(matcher string) ListConnectionsBuilder
	OrderBy(database.ConnectionOrderByField, pagination.OrderBy) ListConnectionsBuilder
	IncludeDeleted() ListConnectionsBuilder
	WithDeletedHandling(database.DeletedHandling) ListConnectionsBuilder
}

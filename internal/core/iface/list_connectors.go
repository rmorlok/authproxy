package iface

import (
	"context"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type ListConnectorsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Connector]
	Enumerate(context.Context, func(pagination.PageResult[Connector]) (keepGoing bool, err error)) error
}

type ListConnectorsBuilder interface {
	ListConnectorsExecutor
	Limit(int32) ListConnectorsBuilder
	ForId(uuid.UUID) ListConnectorsBuilder
	ForState(database.ConnectorVersionState) ListConnectorsBuilder
	ForStates([]database.ConnectorVersionState) ListConnectorsBuilder
	ForNamespaceMatcher(string) ListConnectorsBuilder
	ForNamespaceMatchers([]string) ListConnectorsBuilder
	OrderBy(database.ConnectorOrderByField, pagination.OrderBy) ListConnectorsBuilder
	IncludeDeleted() ListConnectorsBuilder
	ForLabelSelector(selector string) ListConnectorsBuilder
}

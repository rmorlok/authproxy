package iface

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type ListConnectorsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Connector]
	Enumerate(context.Context, pagination.EnumerateCallback[Connector]) error
}

type ListConnectorsBuilder interface {
	ListConnectorsExecutor
	Limit(int32) ListConnectorsBuilder
	ForId(apid.ID) ListConnectorsBuilder
	ForState(database.ConnectorDefinitionVersionState) ListConnectorsBuilder
	ForStates([]database.ConnectorDefinitionVersionState) ListConnectorsBuilder
	ForNamespaceMatcher(string) ListConnectorsBuilder
	ForNamespaceMatchers([]string) ListConnectorsBuilder
	ForName(name common.ResourceName) ListConnectorsBuilder
	OrderBy(database.ConnectorOrderByField, pagination.OrderBy) ListConnectorsBuilder
	IncludeDeleted() ListConnectorsBuilder
	ForLabelSelector(selector string) ListConnectorsBuilder
}

package iface

import (
	"context"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

/*
 * These interfaces wrap the database equivalents so this service can provide decryption and potentially caching.
 */

type ListConnectorsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Connector]
	Enumerate(context.Context, func(pagination.PageResult[Connector]) (keepGoing bool, err error)) error
}

type ListConnectorsBuilder interface {
	ListConnectorsExecutor
	Limit(int32) ListConnectorsBuilder
	ForType(string) ListConnectorsBuilder
	ForId(uuid.UUID) ListConnectorsBuilder
	ForState(database.ConnectorVersionState) ListConnectorsBuilder
	ForStates([]database.ConnectorVersionState) ListConnectorsBuilder
	ForNamespaceMatcher(string) ListConnectorsBuilder
	OrderBy(database.ConnectorOrderByField, pagination.OrderBy) ListConnectorsBuilder
	IncludeDeleted() ListConnectorsBuilder
}

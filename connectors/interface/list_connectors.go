package _interface

import (
	"context"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/database"
)

/*
 * These interfaces wrap the database equivalents so this service can provide decryption and potentially caching.
 */

type ListConnectorsExecutor interface {
	FetchPage(context.Context) database.PageResult[Connector]
	Enumerate(context.Context, func(database.PageResult[Connector]) (keepGoing bool, err error)) error
}

type ListConnectorsBuilder interface {
	ListConnectorsExecutor
	Limit(int32) ListConnectorsBuilder
	ForType(string) ListConnectorsBuilder
	ForId(uuid.UUID) ListConnectorsBuilder
	ForConnectorVersionState(database.ConnectorVersionState) ListConnectorsBuilder
	OrderBy(database.ConnectorOrderByField, database.OrderBy) ListConnectorsBuilder
	IncludeDeleted() ListConnectorsBuilder
}

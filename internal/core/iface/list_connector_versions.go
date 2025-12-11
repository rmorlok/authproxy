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

type ListConnectorVersionsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[ConnectorVersion]
	Enumerate(context.Context, func(pagination.PageResult[ConnectorVersion]) (keepGoing bool, err error)) error
}

type ListConnectorVersionsBuilder interface {
	ListConnectorVersionsExecutor
	Limit(int32) ListConnectorVersionsBuilder
	ForType(string) ListConnectorVersionsBuilder
	ForId(uuid.UUID) ListConnectorVersionsBuilder
	ForVersion(version uint64) ListConnectorVersionsBuilder
	ForState(database.ConnectorVersionState) ListConnectorVersionsBuilder
	ForStates([]database.ConnectorVersionState) ListConnectorVersionsBuilder
	ForNamespaceMatcher(string) ListConnectorVersionsBuilder
	OrderBy(database.ConnectorVersionOrderByField, pagination.OrderBy) ListConnectorVersionsBuilder
	IncludeDeleted() ListConnectorVersionsBuilder
}

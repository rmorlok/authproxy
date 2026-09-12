package iface

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

/*
 * These interfaces wrap the database equivalents so this service can provide decryption and potentially caching.
 */

type ListConnectorGenerationsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Connector]
	Enumerate(context.Context, pagination.EnumerateCallback[Connector]) error
}

type ListConnectorGenerationsBuilder interface {
	ListConnectorGenerationsExecutor
	Limit(int32) ListConnectorGenerationsBuilder
	ForId(apid.ID) ListConnectorGenerationsBuilder
	ForGeneration(generation uint64) ListConnectorGenerationsBuilder
	ForState(database.ConnectorGenerationState) ListConnectorGenerationsBuilder
	ForStates([]database.ConnectorGenerationState) ListConnectorGenerationsBuilder
	ForNamespaceMatcher(string) ListConnectorGenerationsBuilder
	ForNamespaceMatchers([]string) ListConnectorGenerationsBuilder
	ForName(name common.ResourceName) ListConnectorGenerationsBuilder
	OrderBy(database.ConnectorGenerationOrderByField, pagination.OrderBy) ListConnectorGenerationsBuilder
	IncludeDeleted() ListConnectorGenerationsBuilder
	ForLabelSelector(selector string) ListConnectorGenerationsBuilder
}

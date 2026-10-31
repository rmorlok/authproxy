package iface

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// ListToolsExecutor runs a management Tool listing one page at a time or across
// every remaining page. Callers must still filter results by permission.
type ListToolsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Tool]
	Enumerate(context.Context, pagination.EnumerateCallback[Tool]) error
}

// ListToolsBuilder filters a management listing. Namespace scope is required and
// should be the caller's effective permission scope; an empty scope matches
// nothing. Other filters are optional and intersect. Invalid filters surface as
// ErrInvalidArgument when the listing runs.
type ListToolsBuilder interface {
	ListToolsExecutor
	Limit(int32) ListToolsBuilder
	ForNamespaceMatcher(string) ListToolsBuilder
	ForNamespaceMatchers([]string) ListToolsBuilder
	ForName(common.ResourceName) ListToolsBuilder
	ForConnectionId(apid.ID) ListToolsBuilder
	ForLabelSelector(string) ListToolsBuilder
	OrderBy(database.ToolOrderByField, pagination.OrderBy) ListToolsBuilder
}

package iface

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// ListToolSetsExecutor runs a logical ToolSet listing one page at a time or across
// every remaining page. Each item is the ToolSet's default generation projection.
// Callers must still filter results by permission.
type ListToolSetsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[ToolSet]
	Enumerate(context.Context, pagination.EnumerateCallback[ToolSet]) error
}

// ListToolSetsBuilder filters a management listing of logical ToolSets. Namespace
// scope is required and should be the caller's effective permission scope; an
// empty scope matches nothing. Other filters are optional and intersect. Invalid
// filters surface as ErrInvalidArgument when the listing runs.
type ListToolSetsBuilder interface {
	ListToolSetsExecutor
	Limit(int32) ListToolSetsBuilder
	ForNamespaceMatcher(string) ListToolSetsBuilder
	ForNamespaceMatchers([]string) ListToolSetsBuilder
	ForName(common.ResourceName) ListToolSetsBuilder
	ForLabelSelector(string) ListToolSetsBuilder
	OrderBy(database.ToolSetOrderByField, pagination.OrderBy) ListToolSetsBuilder
}

// ListToolSetGenerationsExecutor runs a generation listing one page at a time or
// across every remaining page, with one item per generation.
type ListToolSetGenerationsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[ToolSet]
	Enumerate(context.Context, pagination.EnumerateCallback[ToolSet]) error
}

// ListToolSetGenerationsBuilder pages one ToolSet's generations, newest first
// unless OrderBy is set. ForToolSetId is required.
type ListToolSetGenerationsBuilder interface {
	ListToolSetGenerationsExecutor
	Limit(int32) ListToolSetGenerationsBuilder
	ForToolSetId(apid.ID) ListToolSetGenerationsBuilder
	OrderBy(database.ToolSetGenerationOrderByField, pagination.OrderBy) ListToolSetGenerationsBuilder
}

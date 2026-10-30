package iface

import (
	"errors"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// ErrToolSetConflict means a ToolSet write lost a race with another writer: a
// draft revision or the logical selector revision changed after it was read.
// Callers should reload and retry.
var ErrToolSetConflict = errors.New("toolset changed concurrently")

// ToolSet is the management view of one ToolSet generation projected with its
// logical identity and current shared connection selector. It carries the
// decrypted generation definition, so it is only exposed through management
// resources.
type ToolSet interface {
	GetId() apid.ID
	GetNamespace() string
	GetName() common.ResourceName

	// GetGeneration returns the addressed generation number.
	GetGeneration() uint64

	// GetState returns the generation's observed release state.
	GetState() toolsetschema.ToolSetReleaseState

	// GetResource returns a detached canonical resource for API responses.
	GetResource() *toolsetschema.ToolSet
}

// ListToolSetsRequest filters a management listing of logical ToolSets.
// NamespaceMatchers is required and should be the caller's effective permission
// scope; an empty list matches nothing. Other filters are optional and intersect.
type ListToolSetsRequest struct {
	NamespaceMatchers []string
	Name              common.ResourceName
	LabelSelector     string

	// Limit is the requested page size. Zero selects the default page size;
	// larger values are capped at the database maximum.
	Limit int32

	// OrderByField optionally selects the sort column; empty sorts by ID.
	OrderByField database.ToolSetOrderByField

	// OrderBy is the OrderByField direction, ascending unless descending is set.
	OrderBy pagination.OrderBy
}

// ListToolSetGenerationsRequest pages one ToolSet's generations.
type ListToolSetGenerationsRequest struct {
	// Limit is the requested page size. Zero selects the default page size;
	// larger values are capped at the database maximum.
	Limit int32

	// OrderByField optionally selects the sort column; empty is newest first.
	OrderByField database.ToolSetGenerationOrderByField

	// OrderBy is the OrderByField direction, ascending unless descending is set.
	OrderBy pagination.OrderBy
}

// ToolSetPage is one page of ToolSet resources plus an opaque cursor that is
// empty at the end. Logical listings project each ToolSet's default generation;
// generation listings contain one item per generation.
type ToolSetPage struct {
	Items  []ToolSet
	Cursor string
}

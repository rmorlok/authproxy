package iface

import (
	"errors"

	"github.com/rmorlok/authproxy/internal/apid"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
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

// ConnectionToolSetBinding is the management view of one ToolSet assigned to a
// connection. It reports installation state only and grants no execution access.
type ConnectionToolSetBinding interface {
	GetConnectionId() apid.ID

	// GetToolSetId and GetToolSetNamespace identify the parent ToolSet, whose
	// namespace can be an ancestor of the connection's. Callers authorize it
	// separately because seeing a connection does not reveal its ToolSets.
	GetToolSetId() apid.ID
	GetToolSetNamespace() string

	// GetResource returns a detached canonical resource for API responses.
	GetResource() *schemaapi.ConnectionToolSetBinding
}

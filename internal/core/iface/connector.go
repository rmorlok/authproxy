package iface

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// Connector is the hydrated core representation of a logical connector at a
// selected definition version.
type Connector interface {
	GetId() apid.ID
	GetNamespace() string
	GetName() common.ResourceName
	GetVersion() uint64
	GetState() database.ConnectorDefinitionVersionState
	GetHash() string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetLabels() map[string]string
	GetAnnotations() map[string]string
	GetDefinition() *cschema.ConnectorDefinition
	GetResource() *cschema.Connector
	SetState(ctx context.Context, state database.ConnectorDefinitionVersionState) error
}

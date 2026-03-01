package iface

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

type ConnectorVersion interface {
	GetId() apid.ID
	GetNamespace() string
	GetVersion() uint64
	GetState() database.ConnectorVersionState
	GetHash() string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetLabels() map[string]string
	GetDefinition() *cschema.Connector
	SetState(ctx context.Context, state database.ConnectorVersionState) error
}

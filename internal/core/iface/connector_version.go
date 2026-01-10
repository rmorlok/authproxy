package iface

import (
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

type ConnectorVersion interface {
	GetId() uuid.UUID
	GetNamespace() string
	GetVersion() uint64
	GetState() database.ConnectorVersionState
	GetType() string
	GetHash() string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetDefinition() *cschema.Connector
}

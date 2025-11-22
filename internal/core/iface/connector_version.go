package iface

import (
	"time"

	"github.com/google/uuid"
	cfg "github.com/rmorlok/authproxy/internal/config/connectors"
	"github.com/rmorlok/authproxy/internal/database"
)

type ConnectorVersion interface {
	GetID() uuid.UUID
	GetNamespacePath() string
	GetVersion() uint64
	GetState() database.ConnectorVersionState
	GetType() string
	GetHash() string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetDefinition() *cfg.Connector
}

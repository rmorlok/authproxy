package _interface

import (
	"github.com/google/uuid"
	cfg "github.com/rmorlok/authproxy/config/connectors"
	"github.com/rmorlok/authproxy/database"
)

type ConnectorVersion interface {
	GetID() uuid.UUID
	GetVersion() uint64
	GetState() database.ConnectorVersionState
	GetType() string
	GetHash() string
	GetDefinition() *cfg.Connector
}

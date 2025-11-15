package core

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/aplog"
	cfg "github.com/rmorlok/authproxy/internal/config/connectors"
	"github.com/rmorlok/authproxy/internal/database"
)

func newTestConnection(c cfg.Connector) *connection {
	return newTestConnectionWithDetails(uuid.New(), database.ConnectionStateReady, c)
}

func newTestConnectionWithDetails(u uuid.UUID, s database.ConnectionState, c cfg.Connector) *connection {
	cv := NewTestConnectorVersion(c)
	return &connection{
		Connection: database.Connection{
			ID:               u,
			State:            s,
			ConnectorId:      cv.GetID(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      cv.s,
		cv:     cv,
		logger: aplog.NewNoopLogger(),
	}
}

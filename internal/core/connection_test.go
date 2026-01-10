package core

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

func newTestConnection(c cschema.Connector) *connection {
	return newTestConnectionWithDetails(uuid.New(), database.ConnectionStateReady, c)
}

func newTestConnectionWithDetails(u uuid.UUID, s database.ConnectionState, c cschema.Connector) *connection {
	cv := NewTestConnectorVersion(c)
	return &connection{
		Connection: database.Connection{
			Id:               u,
			State:            s,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      cv.s,
		cv:     cv,
		logger: aplog.NewNoopLogger(),
	}
}

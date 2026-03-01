package core

import (
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

func newTestConnection(c cschema.Connector) *connection {
	return newTestConnectionWithDetails(apid.New(apid.PrefixActor), database.ConnectionStateReady, c)
}

func newTestConnectionWithDetails(u apid.ID, s database.ConnectionState, c cschema.Connector) *connection {
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

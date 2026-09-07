package core

import (
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

func newTestConnection(definition cschema.ConnectorDefinition) *connection {
	return newTestConnectionWithDetails(apid.New(apid.PrefixActor), database.ConnectionStateConfigured, definition)
}

func newTestConnectionWithDetails(u apid.ID, s database.ConnectionState, definition cschema.ConnectorDefinition) *connection {
	c := NewTestConnector(definition)
	return &connection{
		Connection: database.Connection{
			Id:               u,
			State:            s,
			ConnectorId:      c.GetId(),
			ConnectorVersion: c.GetVersion(),
		},
		s:         c.s,
		connector: c,
		logger:    aplog.NewNoopLogger(),
	}
}

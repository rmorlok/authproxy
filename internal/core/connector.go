package core

import (
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

// Connector object is returned from queries for connectors, with one record per id. It aggregates some information
// across all versions for a connector.
type Connector struct {
	ConnectorVersion
	TotalVersions int64
	States        database.ConnectorVersionStates
}

func (c *Connector) GetTotalVersions() int64 {
	return c.TotalVersions
}

func (c *Connector) GetStates() database.ConnectorVersionStates {
	return c.States
}

var _ iface.Connector = (*Connector)(nil)

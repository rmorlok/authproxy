package core

import (
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

// GetConnectorVersionIdsForConnections returns a list of unique connector version ids for the given set of
// connections. The purpose of this is to support loading connector versions in bulk.
func GetConnectorVersionIdsForConnections(connections []database.Connection) []iface.ConnectorVersionId {
	ids := make(map[iface.ConnectorVersionId]struct{}, len(connections))
	for _, c := range connections {
		ids[iface.ConnectorVersionId{Id: c.ConnectorId, Version: c.ConnectorVersion}] = struct{}{}
	}

	result := make([]iface.ConnectorVersionId, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}

	return result
}

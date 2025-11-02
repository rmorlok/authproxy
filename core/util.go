package core

import (
	"github.com/rmorlok/authproxy/core/iface"
	"github.com/rmorlok/authproxy/database"
)

func GetConnectorVersionIdsForConnections(connections []database.Connection) []iface.ConnectorVersionId {
	ids := make(map[iface.ConnectorVersionId]struct{}, len(connections))
	for _, c := range connections {
		ids[iface.ConnectorVersionId{c.ConnectorId, c.ConnectorVersion}] = struct{}{}
	}

	result := make([]iface.ConnectorVersionId, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	return result
}

package connectors

import (
	"github.com/rmorlok/authproxy/connectors/interface"
	"github.com/rmorlok/authproxy/database"
)

func GetConnectorVersionIdsForConnections(connections []database.Connection) []_interface.ConnectorVersionId {
	ids := make(map[_interface.ConnectorVersionId]struct{}, len(connections))
	for _, c := range connections {
		ids[_interface.ConnectorVersionId{c.ConnectorId, c.ConnectorVersion}] = struct{}{}
	}

	result := make([]_interface.ConnectorVersionId, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	return result
}

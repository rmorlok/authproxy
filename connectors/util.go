package connectors

import "github.com/rmorlok/authproxy/database"

func GetConnectorVersionIdsForConnections(connections []database.Connection) []ConnectorVersionId {
	ids := make(map[ConnectorVersionId]struct{}, len(connections))
	for _, c := range connections {
		ids[ConnectorVersionId{c.ConnectorId, c.ConnectorVersion}] = struct{}{}
	}

	result := make([]ConnectorVersionId, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	return result
}

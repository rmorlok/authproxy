package core

import (
	"errors"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

// GetConnectorGenerationIdsForConnections returns a list of unique connector generation ids for the given set of
// connections. The purpose of this is to support loading connector generations in bulk.
func GetConnectorGenerationIdsForConnections(
	connections []database.Connection,
) []iface.ConnectorGenerationId {
	ids := make(map[iface.ConnectorGenerationId]struct{}, len(connections))
	for _, c := range connections {
		ids[iface.ConnectorGenerationId{Id: c.ConnectorId, Generation: c.ConnectorGeneration}] = struct{}{}
	}

	result := make([]iface.ConnectorGenerationId, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}

	return result
}

// mapDatabaseError maps a database error to a core error.
func mapDatabaseError(err error) error {
	if errors.Is(err, database.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

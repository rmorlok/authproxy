package connectors

import (
	"context"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/tasks"
)

// C is the interface for the connectors service
type C interface {
	/*
	 * Migrating connectors from config to db
	 */

	// MigrateConnectors migrates connectors from configuration to the database
	// It checks if the connector already exists in the database:
	// - If it doesn't exist, it creates a new one
	// - If it exists and the data matches, it does nothing
	// - If it exists and the data has changed, it creates a new version
	MigrateConnectors(ctx context.Context) error

	/*
	 * Get connector version
	 */
	// GetConnectorVersion returns the specified version of a connector.
	GetConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (*ConnectorVersion, error)

	// GetConnectorVersionForState returns the most recent version of the connector for the specified state.
	GetConnectorVersionForState(ctx context.Context, id uuid.UUID, state database.ConnectorVersionState) (*ConnectorVersion, error)

	/*
	 * List connectors
	 */

	// ListConnectorsBuilder returns a builder to allow the caller to list connectors matching certain criteria.
	ListConnectorsBuilder() ListConnectorsBuilder

	// ListConnectorsFromCursor continues listing connectors from a cursor to support pagination.
	ListConnectorsFromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error)

	/*
	 * Connection-specific operations
	 */

	// DisconnectConnection disconnects a connection. This is a state transition that queues work to do any cleanup
	// with the 3rd party.
	DisconnectConnection(ctx context.Context, id uuid.UUID) (taskInfo *tasks.TaskInfo, err error)

	/*
	 * Task manager interface functions.
	 */

	RegisterTasks(mux *asynq.ServeMux)
	GetCronTasks() []*asynq.PeriodicTaskConfig
}

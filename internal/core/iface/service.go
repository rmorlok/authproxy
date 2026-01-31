package iface

import (
	"context"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/tasks"
)

type ConnectorVersionId = database.ConnectorVersionId

// C is the interface for the core service that implements primary business logic and binds the system together.
type C interface {
	/*
	 *
	 * Migration
	 *
	 */

	// Migrate migrates all resources defined in config file into the databases within the system, invoking appropriate
	// events, lifecycle hooks, etc.
	Migrate(ctx context.Context) error

	// MigrateConnectors migrates connectors from configuration to the database
	// It checks if the connector already exists in the database:
	// - If it doesn't exist, it creates a new one
	// - If it exists and the data matches, it does nothing
	// - If it exists and the data has changed, it creates a new version
	MigrateConnectors(ctx context.Context) error

	/*
	 *
	 * Connectors
	 *
	 */

	// GetConnectorVersion returns the specified version of a connector.
	GetConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (ConnectorVersion, error)

	// GetConnectorVersions Retrieves multiple connector versions at once.
	GetConnectorVersions(ctx context.Context, requested []ConnectorVersionId) (map[ConnectorVersionId]ConnectorVersion, error)

	// GetConnectorVersionForState returns the most recent version of the connector for the specified state.
	GetConnectorVersionForState(ctx context.Context, id uuid.UUID, state database.ConnectorVersionState) (ConnectorVersion, error)

	// ListConnectorsBuilder returns a builder to allow the caller to list connectors matching certain criteria.
	ListConnectorsBuilder() ListConnectorsBuilder

	// ListConnectorsFromCursor continues listing connectors from a cursor to support pagination.
	ListConnectorsFromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error)

	// ListConnectorVersionsBuilder returns a builder to allow the caller to list connector versions matching certain criteria.
	ListConnectorVersionsBuilder() ListConnectorVersionsBuilder

	// ListConnectorVersionsFromCursor continues listing connector versions from a cursor to support pagination.
	ListConnectorVersionsFromCursor(ctx context.Context, cursor string) (ListConnectorVersionsExecutor, error)

	/*
	 *
	 * Connections
	 *
	 */

	// DisconnectConnection disconnects a connection. This is a state transition that queues work to do any cleanup
	// with the 3rd party.
	DisconnectConnection(ctx context.Context, id uuid.UUID) (taskInfo *tasks.TaskInfo, err error)

	// GetConnection returns a connection by ID. This connection has the full connection version details in it.
	GetConnection(ctx context.Context, id uuid.UUID) (Connection, error)

	// CreateConnection creates a new connection.
	CreateConnection(ctx context.Context, namespace string, cv ConnectorVersion) (Connection, error)

	// ListConnectionsBuilder returns a builder to allow the caller to list connections matching certain criteria.
	ListConnectionsBuilder() ListConnectionsBuilder

	// ListConnectionsFromCursor continues listing connections from a cursor to support pagination.
	ListConnectionsFromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error)

	InitiateConnection(ctx context.Context, req InitiateConnectionRequest) (InitiateConnectionResponse, error)

	/*
	 *
	 * Namespaces
	 *
	 */

	// GetNamespace returns a namespace by path.
	GetNamespace(ctx context.Context, path string) (Namespace, error)

	// CreateNamespace creates a new namespace.
	CreateNamespace(ctx context.Context, path string, labels map[string]string) (Namespace, error)

	// EnsureNamespaceAncestorPath ensures that the specified namespace path exists in the database.
	EnsureNamespaceAncestorPath(ctx context.Context, targetNamespace string, labels map[string]string) (Namespace, error)

	// ListNamespacesBuilder returns a builder to allow the caller to list namespaces matching certain criteria.
	ListNamespacesBuilder() ListNamespacesBuilder

	// ListNamespacesFromCursor continues listing namespaces from a cursor to support pagination.
	ListNamespacesFromCursor(ctx context.Context, cursor string) (ListNamespacesExecutor, error)

	/*
	 *
	 * Tasks
	 *
	 */

	RegisterTasks(mux *asynq.ServeMux)
	GetCronTasks() []*asynq.PeriodicTaskConfig
}

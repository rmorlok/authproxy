package iface

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	cfgschema "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
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
	GetConnectorVersion(ctx context.Context, id apid.ID, version uint64) (ConnectorVersion, error)

	// GetConnectorVersions Retrieves multiple connector versions at once.
	GetConnectorVersions(ctx context.Context, requested []ConnectorVersionId) (map[ConnectorVersionId]ConnectorVersion, error)

	// GetConnectorVersionForState returns the most recent version of the connector for the specified state.
	GetConnectorVersionForState(ctx context.Context, id apid.ID, state database.ConnectorVersionState) (ConnectorVersion, error)

	// ListConnectorsBuilder returns a builder to allow the caller to list connectors matching certain criteria.
	ListConnectorsBuilder() ListConnectorsBuilder

	// ListConnectorsFromCursor continues listing connectors from a cursor to support pagination.
	ListConnectorsFromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error)

	// ListConnectorVersionsBuilder returns a builder to allow the caller to list connector versions matching certain criteria.
	ListConnectorVersionsBuilder() ListConnectorVersionsBuilder

	// ListConnectorVersionsFromCursor continues listing connector versions from a cursor to support pagination.
	ListConnectorVersionsFromCursor(ctx context.Context, cursor string) (ListConnectorVersionsExecutor, error)

	// CreateConnectorVersion creates a new connector with version 1 in draft state.
	CreateConnectorVersion(ctx context.Context, namespace string, definition *cschema.Connector, labels map[string]string, annotations map[string]string) (ConnectorVersion, error)

	// CreateDraftConnectorVersion creates a new draft version for an existing connector.
	// Returns ErrDraftAlreadyExists if a draft version already exists.
	CreateDraftConnectorVersion(ctx context.Context, id apid.ID, definition *cschema.Connector, labels map[string]string, annotations map[string]string) (ConnectorVersion, error)

	// UpdateDraftConnectorVersion updates an existing draft version.
	// Returns ErrNotDraft if the version is not in draft state.
	UpdateDraftConnectorVersion(ctx context.Context, id apid.ID, version uint64, definition *cschema.Connector, labels map[string]string, annotations map[string]string) (ConnectorVersion, error)

	// GetOrCreateDraftConnectorVersion returns the existing draft version, or creates a new one by cloning the latest version.
	GetOrCreateDraftConnectorVersion(ctx context.Context, id apid.ID) (ConnectorVersion, error)

	/*
	 *
	 * Connections
	 *
	 */

	// DisconnectConnection disconnects a connection. This is a state transition that queues work to do any cleanup
	// with the 3rd party.
	DisconnectConnection(ctx context.Context, id apid.ID) (taskInfo *tasks.TaskInfo, err error)

	// GetConnection returns a connection by ID. This connection has the full connection version details in it.
	GetConnection(ctx context.Context, id apid.ID) (Connection, error)

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

	// UpdateNamespaceLabels replaces all labels on a namespace.
	UpdateNamespaceLabels(ctx context.Context, path string, labels map[string]string) (Namespace, error)

	// PutNamespaceLabels adds or updates the specified labels on a namespace.
	PutNamespaceLabels(ctx context.Context, path string, labels map[string]string) (Namespace, error)

	// DeleteNamespaceLabels removes the specified label keys from a namespace.
	DeleteNamespaceLabels(ctx context.Context, path string, keys []string) (Namespace, error)

	// UpdateNamespaceAnnotations replaces all annotations on a namespace.
	UpdateNamespaceAnnotations(ctx context.Context, path string, annotations map[string]string) (Namespace, error)

	// PutNamespaceAnnotations adds or updates the specified annotations on a namespace.
	PutNamespaceAnnotations(ctx context.Context, path string, annotations map[string]string) (Namespace, error)

	// DeleteNamespaceAnnotations removes the specified annotation keys from a namespace.
	DeleteNamespaceAnnotations(ctx context.Context, path string, keys []string) (Namespace, error)

	// EnsureNamespaceAncestorPath ensures that the specified namespace path exists in the database.
	EnsureNamespaceAncestorPath(ctx context.Context, targetNamespace string, labels map[string]string) (Namespace, error)

	// SetNamespaceEncryptionKey sets the encryption key for a namespace.
	SetNamespaceEncryptionKey(ctx context.Context, path string, ekId apid.ID) (Namespace, error)

	// ClearNamespaceEncryptionKey clears the encryption key for a namespace (falls back to parent).
	ClearNamespaceEncryptionKey(ctx context.Context, path string) (Namespace, error)

	// ListNamespacesBuilder returns a builder to allow the caller to list namespaces matching certain criteria.
	ListNamespacesBuilder() ListNamespacesBuilder

	// ListNamespacesFromCursor continues listing namespaces from a cursor to support pagination.
	ListNamespacesFromCursor(ctx context.Context, cursor string) (ListNamespacesExecutor, error)

	/*
	 *
	 * Encryption Keys
	 *
	 */

	// GetEncryptionKey returns an encryption key by ID.
	GetEncryptionKey(ctx context.Context, id apid.ID) (EncryptionKey, error)

	// CreateEncryptionKey creates a new encryption key.
	CreateEncryptionKey(ctx context.Context, namespace string, keyData *cfgschema.KeyData, labels map[string]string) (EncryptionKey, error)

	// DeleteEncryptionKey soft deletes an encryption key.
	DeleteEncryptionKey(ctx context.Context, id apid.ID) error

	// SetEncryptionKeyState sets the state of an encryption key.
	SetEncryptionKeyState(ctx context.Context, id apid.ID, state database.EncryptionKeyState) error

	// UpdateEncryptionKeyLabels replaces all labels on an encryption key.
	UpdateEncryptionKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (EncryptionKey, error)

	// PutEncryptionKeyLabels adds or updates the specified labels on an encryption key.
	PutEncryptionKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (EncryptionKey, error)

	// DeleteEncryptionKeyLabels removes the specified label keys from an encryption key.
	DeleteEncryptionKeyLabels(ctx context.Context, id apid.ID, keys []string) (EncryptionKey, error)

	// UpdateEncryptionKeyAnnotations replaces all annotations on an encryption key.
	UpdateEncryptionKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (EncryptionKey, error)

	// PutEncryptionKeyAnnotations adds or updates the specified annotations on an encryption key.
	PutEncryptionKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (EncryptionKey, error)

	// DeleteEncryptionKeyAnnotations removes the specified annotation keys from an encryption key.
	DeleteEncryptionKeyAnnotations(ctx context.Context, id apid.ID, keys []string) (EncryptionKey, error)

	// ListEncryptionKeysBuilder returns a builder to allow the caller to list encryption keys matching certain criteria.
	ListEncryptionKeysBuilder() ListEncryptionKeysBuilder

	// ListEncryptionKeysFromCursor continues listing encryption keys from a cursor to support pagination.
	ListEncryptionKeysFromCursor(ctx context.Context, cursor string) (ListEncryptionKeysExecutor, error)

	/*
	 *
	 * Tasks
	 *
	 */

	RegisterTasks(mux *asynq.ServeMux)
	GetCronTasks() []*asynq.PeriodicTaskConfig
}

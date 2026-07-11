package iface

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	cfgschema "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/rmorlok/authproxy/internal/tasks"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
)

type ConnectorVersionId = database.ConnectorVersionId

type ConnectorLifecycleOptions struct {
	Timeout time.Duration
}

type ConnectionDisconnectOptions struct {
	Timeout time.Duration
}

type ConnectionMigrationOptions struct {
	TargetVersion uint64
	Timeout       time.Duration
}

type ConnectionMigrationTask struct {
	TaskInfo      *tasks.TaskInfo
	ConnectionID  apid.ID
	SourceVersion uint64
	TargetVersion uint64
}

type ActorNotification struct {
	Notification database.Notification
	Viewed       bool
	CanAction    bool
}

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

	// DisconnectConnectorConnections starts a workflow that disconnects all connections for a connector.
	DisconnectConnectorConnections(ctx context.Context, id apid.ID, opts ConnectorLifecycleOptions) (taskInfo *tasks.TaskInfo, err error)

	// ArchiveConnector starts a workflow that archives a connector after disconnecting its connections.
	ArchiveConnector(ctx context.Context, id apid.ID, opts ConnectorLifecycleOptions) (taskInfo *tasks.TaskInfo, err error)

	/*
	 *
	 * Connections
	 *
	 */

	// DisconnectConnection disconnects a connection. This is a state transition that queues work to do any cleanup
	// with the 3rd party.
	DisconnectConnection(ctx context.Context, id apid.ID, opts ConnectionDisconnectOptions) (taskInfo *tasks.TaskInfo, err error)

	// MigrateConnectionVersion starts a durable workflow that migrates a single connection to another version of the
	// same connector.
	MigrateConnectionVersion(ctx context.Context, id apid.ID, opts ConnectionMigrationOptions) (*ConnectionMigrationTask, error)

	// AbortConnection aborts an in-progress connection setup, revoking any credentials and deleting the connection.
	// Only valid for connections with a non-null setup_step.
	AbortConnection(ctx context.Context, id apid.ID) error

	// GetConnection returns a connection by ID. This connection has the full connection version details in it.
	GetConnection(ctx context.Context, id apid.ID) (Connection, error)

	// CreateConnection creates a new connection.
	CreateConnection(ctx context.Context, namespace string, cv ConnectorVersion) (Connection, error)

	// ListConnectionsBuilder returns a builder to allow the caller to list connections matching certain criteria.
	ListConnectionsBuilder() ListConnectionsBuilder

	// ListConnectionsFromCursor continues listing connections from a cursor to support pagination.
	ListConnectionsFromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error)

	InitiateConnection(ctx context.Context, req InitiateConnectionRequest) (ConnectionSetupResponse, error)

	// EnqueueVerifyConnection enqueues a background task to run all probes for a connection as part of
	// the verify step of the setup flow. The task advances the connection's setup step based on the outcome.
	EnqueueVerifyConnection(ctx context.Context, id apid.ID) error

	// RunProbe synchronously invokes a single probe against a connection and records the outcome
	// against the connection's health-state counters — the same code path the periodic asynq probe
	// task takes, just inline. Returns the probe's invocation error (or nil on success). Used by
	// integration tests that need deterministic probe execution and by future callers that want to
	// nudge a probe sooner than its next scheduled tick.
	RunProbe(ctx context.Context, connectionId apid.ID, probeId string) error

	// RunVerifyConnection synchronously runs every probe declared on the connection's connector
	// and advances setup_step based on the outcome — the same code path the asynq verify task
	// takes, just inline. Used by integration tests that drive the setup lifecycle without a
	// background worker. Returns asynq.SkipRetry for non-recoverable shapes (connection not found,
	// no longer in verify phase); other errors propagate unwrapped.
	RunVerifyConnection(ctx context.Context, connectionId apid.ID) error

	// EnqueueProbeNow schedules an immediate one-shot probe run for every probe configured on the
	// connection. Used by the proxy's 401/403 detection path to cut detection lag from the
	// configured probe interval to ~immediate when an upstream signals a credential failure on a
	// user-initiated request. Per-(connection, probe) throttling caps the rate of enqueues so a
	// 401 storm does not pile up tasks. Best-effort: errors are logged but do not surface to the
	// caller, since the caller's response is already on its way.
	EnqueueProbeNow(ctx context.Context, connectionId apid.ID) error

	// RetryConnectionSetup resets a connection that is in the verify_failed terminal state so the user
	// can try setup again — either restarting preconnect forms, or re-initiating OAuth if the connector
	// has no preconnect steps. Returns the initial setup step response for the retry.
	RetryConnectionSetup(ctx context.Context, id apid.ID, returnToUrl string) (ConnectionSetupResponse, error)

	// ReauthConnection re-runs the credential-collection portion of setup against an existing Ready
	// connection. Used for user-driven credential rotation (manual "Re-authenticate") and for the
	// recovery path on an unhealthy connection. For api-key, returns the credentials form with no
	// prior values pre-filled; on submit, InsertApiKeyCredential rotates the row in-place (the
	// existing row is soft-deleted in the same transaction). For OAuth2, re-issues preconnect:0 if
	// defined, otherwise re-initiates the OAuth redirect. The connection's State remains Ready
	// throughout; only setup_step is reset and re-driven.
	ReauthConnection(ctx context.Context, id apid.ID, returnToUrl string) (ConnectionSetupResponse, error)

	/*
	 *
	 * Notifications
	 *
	 */

	// ListActorNotifications returns actor-visible notifications with actor-specific
	// viewed/action state. Results are cached by actor and permission fingerprint.
	ListActorNotifications(
		ctx context.Context,
		ra *authcore.RequestAuth,
		opts database.ListNotificationsOptions,
	) ([]ActorNotification, error)

	// MarkActorNotificationViewed records viewed state for the authenticated
	// actor after checking that the actor can see the notification.
	MarkActorNotificationViewed(ctx context.Context, ra *authcore.RequestAuth, id apid.ID) error

	// MarkActorNotificationsViewed records viewed state for multiple
	// authenticated-actor-visible notifications.
	MarkActorNotificationsViewed(ctx context.Context, ra *authcore.RequestAuth, ids []apid.ID) error

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

	// SetNamespaceKey sets the key for a namespace.
	SetNamespaceKey(ctx context.Context, path string, ekId apid.ID) (Namespace, error)

	// ClearNamespaceKey clears the key for a namespace (falls back to parent).
	ClearNamespaceKey(ctx context.Context, path string) (Namespace, error)

	// ListNamespacesBuilder returns a builder to allow the caller to list namespaces matching certain criteria.
	ListNamespacesBuilder() ListNamespacesBuilder

	// ListNamespacesFromCursor continues listing namespaces from a cursor to support pagination.
	ListNamespacesFromCursor(ctx context.Context, cursor string) (ListNamespacesExecutor, error)

	/*
	 *
	 * Keys
	 *
	 */

	// GetKey returns a key by ID.
	GetKey(ctx context.Context, id apid.ID) (Key, error)

	// CreateKey creates a new key.
	CreateKey(ctx context.Context, namespace string, keyData *cfgschema.KeyData, labels map[string]string) (Key, error)

	// GetKeyData returns the decrypted provider configuration for a key.
	GetKeyData(ctx context.Context, id apid.ID) (*cfgschema.KeyData, error)

	// UpdateKeyData replaces the provider configuration for a key.
	UpdateKeyData(ctx context.Context, id apid.ID, keyData *cfgschema.KeyData) (Key, error)

	// DeleteKey soft deletes a key.
	DeleteKey(ctx context.Context, id apid.ID) error

	// SetKeyState sets the state of a key.
	SetKeyState(ctx context.Context, id apid.ID, state database.KeyState) error

	// UpdateKeyLabels replaces all labels on an encryption key.
	UpdateKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (Key, error)

	// PutKeyLabels adds or updates the specified labels on an encryption key.
	PutKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (Key, error)

	// DeleteKeyLabels removes the specified label keys from an encryption key.
	DeleteKeyLabels(ctx context.Context, id apid.ID, keys []string) (Key, error)

	// UpdateKeyAnnotations replaces all annotations on an encryption key.
	UpdateKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (Key, error)

	// PutKeyAnnotations adds or updates the specified annotations on an encryption key.
	PutKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (Key, error)

	// DeleteKeyAnnotations removes the specified annotation keys from an encryption key.
	DeleteKeyAnnotations(ctx context.Context, id apid.ID, keys []string) (Key, error)

	// ListKeysBuilder returns a builder to allow the caller to list keys matching certain criteria.
	ListKeysBuilder() ListKeysBuilder

	// ListKeysFromCursor continues listing keys from a cursor to support pagination.
	ListKeysFromCursor(ctx context.Context, cursor string) (ListKeysExecutor, error)

	/*
	 *
	 * Rate Limits
	 *
	 */

	// GetRateLimit returns a rate limit by ID.
	GetRateLimit(ctx context.Context, id apid.ID) (RateLimit, error)

	// CreateRateLimit creates a new rate-limit resource. Definition is validated before insert.
	CreateRateLimit(ctx context.Context, namespace string, def rlschema.RateLimit, labels, annotations map[string]string) (RateLimit, error)

	// UpdateRateLimitDefinition replaces a rate limit's definition payload.
	UpdateRateLimitDefinition(ctx context.Context, id apid.ID, def rlschema.RateLimit) (RateLimit, error)

	// DeleteRateLimit soft deletes a rate limit.
	DeleteRateLimit(ctx context.Context, id apid.ID) error

	// UpdateRateLimitLabels replaces all user labels on a rate limit.
	UpdateRateLimitLabels(ctx context.Context, id apid.ID, labels map[string]string) (RateLimit, error)

	// PutRateLimitLabels merges the supplied labels into the existing set.
	PutRateLimitLabels(ctx context.Context, id apid.ID, labels map[string]string) (RateLimit, error)

	// DeleteRateLimitLabels removes the specified user-label keys.
	DeleteRateLimitLabels(ctx context.Context, id apid.ID, keys []string) (RateLimit, error)

	// UpdateRateLimitAnnotations replaces all annotations on a rate limit.
	UpdateRateLimitAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (RateLimit, error)

	// PutRateLimitAnnotations merges the supplied annotations into the existing set.
	PutRateLimitAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (RateLimit, error)

	// DeleteRateLimitAnnotations removes the specified annotation keys.
	DeleteRateLimitAnnotations(ctx context.Context, id apid.ID, keys []string) (RateLimit, error)

	// ListRateLimitsBuilder returns a builder for listing rate limits.
	ListRateLimitsBuilder() ListRateLimitsBuilder

	// ListRateLimitsFromCursor continues listing rate limits from a cursor.
	ListRateLimitsFromCursor(ctx context.Context, cursor string) (ListRateLimitsExecutor, error)

	// DryRunRateLimit answers "would this request be rate-limited?"
	// against the same in-memory rule cache the enforcer uses. Counters
	// are not incremented — Limiter.Peek inspects state without writing.
	// Returns the per-rule match + would-allow outcome plus the
	// post-hydration namespace and label snapshot.
	DryRunRateLimit(ctx context.Context, req DryRunRateLimitRequest) (DryRunRateLimitResult, error)

	/*
	 *
	 * Tasks
	 *
	 */

	RegisterTasks(mux *asynq.ServeMux)
	RegisterWorkflows(worker *apworkflows.Worker) error
	GetCronTasks() []*asynq.PeriodicTaskConfig
}

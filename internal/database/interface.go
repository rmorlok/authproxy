package database

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"golang.org/x/time/rate"
)

type DeletedHandling bool

const (
	// DeletedHandlingExclude will exclude deleted records from the result set
	DeletedHandlingExclude DeletedHandling = false

	// DeletedHandlingInclude will include deleted records in the result set
	DeletedHandlingInclude DeletedHandling = true
)

type IActorData interface {
	GetId() apid.ID
	GetExternalId() string
	GetPermissions() []aschema.Permission
	GetNamespace() string
	GetLabels() map[string]string
	// GetAnnotations returns the annotations to apply on upsert. A nil return means
	// annotations should be left unchanged on existing actors (PATCH semantics);
	// a non-nil map (including empty) is treated as a full replacement.
	GetAnnotations() map[string]string
}

// IActorDataExtended extends IActorData with additional fields for labels and encrypted key.
// This interface is used when creating or updating actors with extended data such as
// labels for tracking the source of admin syncs, or encrypted keys for admin authentication.
type IActorDataExtended interface {
	IActorData
	GetEncryptedKey() *encfield.EncryptedField
}

//go:generate mockgen -source=./interface.go -destination=./mock/db.go -package=mock
type DB interface {
	SetCursorEncryptor(e pagination.CursorEncryptor)
	Ping(ctx context.Context) bool
	SearchResources(ctx context.Context, params SearchResourcesParams) (SearchResourcesResult, error)

	/*
	 * Object references
	 */

	ResolveActorReference(ctx context.Context, reference meta.ObjectReference) (*Actor, error)
	ResolveConnectionReference(ctx context.Context, reference meta.ObjectReference) (*Connection, error)
	ResolveConnectorReference(ctx context.Context, reference meta.ObjectReference) (*Connector, error)
	ResolveKeyReference(ctx context.Context, reference meta.ObjectReference) (*Key, error)
	ResolveNamespaceReference(ctx context.Context, reference meta.ObjectReference) (*Namespace, error)
	ResolveRateLimitReference(ctx context.Context, reference meta.ObjectReference) (*RateLimit, error)

	/*
	 *  Namespaces
	 */

	GetNamespace(ctx context.Context, path string) (*Namespace, error)
	CreateNamespace(ctx context.Context, ns *Namespace) error
	EnsureNamespaceByPath(ctx context.Context, path string) error
	DeleteNamespace(ctx context.Context, path string) error
	SetNamespaceState(ctx context.Context, path string, state NamespaceState) error
	SetNamespaceKeyId(ctx context.Context, path string, ekId *apid.ID) (*Namespace, error)
	UpdateNamespaceLabels(ctx context.Context, path string, labels map[string]string) (*Namespace, error)
	PutNamespaceLabels(ctx context.Context, path string, labels map[string]string) (*Namespace, error)
	DeleteNamespaceLabels(ctx context.Context, path string, keys []string) (*Namespace, error)
	UpdateNamespaceAnnotations(ctx context.Context, path string, annotations map[string]string) (*Namespace, error)
	PutNamespaceAnnotations(ctx context.Context, path string, annotations map[string]string) (*Namespace, error)
	DeleteNamespaceAnnotations(ctx context.Context, path string, keys []string) (*Namespace, error)
	ListNamespacesBuilder() ListNamespacesBuilder
	ListNamespacesFromCursor(ctx context.Context, cursor string) (ListNamespacesExecutor, error)
	EnumerateNamespaceEncryptionTargets(
		ctx context.Context,
		callback func(targets []NamespaceEncryptionTarget, lastPage bool) (updates []NamespaceTargetDataEncryptionKeyUpdate, keepGoing pagination.KeepGoing, err error),
	) error

	/*
	 *  Actors
	 */

	GetActor(ctx context.Context, id apid.ID) (*Actor, error)
	GetActorByExternalId(ctx context.Context, namespace, externalId string) (*Actor, error)
	CreateActor(ctx context.Context, actor *Actor) error
	UpsertActor(ctx context.Context, actor IActorData) (*Actor, error)
	UpdateActorName(ctx context.Context, id apid.ID, name scommon.ResourceName) (*Actor, error)
	DeleteActor(ctx context.Context, id apid.ID) error
	PutActorLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Actor, error)
	DeleteActorLabels(ctx context.Context, id apid.ID, keys []string) (*Actor, error)
	UpdateActorAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Actor, error)
	PutActorAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Actor, error)
	DeleteActorAnnotations(ctx context.Context, id apid.ID, keys []string) (*Actor, error)
	ListActorsBuilder() ListActorsBuilder
	ListActorsFromCursor(ctx context.Context, cursor string) (ListActorsExecutor, error)

	/*
	 * Connectors
	 */

	GetConnectorGeneration(ctx context.Context, id apid.ID, generation uint64) (*ConnectorWithDefinition, error)
	GetConnectorGenerations(ctx context.Context, requested []ConnectorGenerationId) (map[ConnectorGenerationId]*ConnectorWithDefinition, error)
	GetConnectorGenerationForState(ctx context.Context, id apid.ID, state ConnectorGenerationState) (*ConnectorWithDefinition, error)
	NewestConnectorGenerationForId(ctx context.Context, id apid.ID) (*ConnectorWithDefinition, error)
	NewestPublishedConnectorGenerationForId(ctx context.Context, id apid.ID) (*ConnectorWithDefinition, error)
	UpsertConnectorGeneration(ctx context.Context, cv *ConnectorWithDefinition) error
	SetConnectorGenerationState(ctx context.Context, id apid.ID, generation uint64, state ConnectorGenerationState) error
	UpdateConnectorName(ctx context.Context, id apid.ID, name scommon.ResourceName) error
	UpdateConnectorLabels(
		ctx context.Context,
		id apid.ID,
		labels map[string]string,
	) (*Connector, error)
	UpdateConnectorAnnotations(
		ctx context.Context,
		id apid.ID,
		annotations map[string]string,
	) (*Connector, error)
	DeleteConnector(ctx context.Context, id apid.ID) error
	ListConnectorGenerationsBuilder() ListConnectorGenerationsBuilder
	ListConnectorGenerationsFromCursor(ctx context.Context, cursor string) (ListConnectorGenerationsExecutor, error)
	ListConnectorsBuilder() ListConnectorsBuilder
	ListConnectorsFromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error)

	/*
	 * ToolSets (logical identity, membership policy, and definition generations)
	 */

	CreateToolSet(ctx context.Context, toolSet *ToolSet) error
	GetToolSet(ctx context.Context, id apid.ID) (*ToolSet, error)
	GetToolSetByName(ctx context.Context, namespace string, name scommon.ResourceName) (*ToolSet, error)
	UpdateToolSetConnectionSelector(ctx context.Context, id apid.ID, expectedRevision uint64, selector toolsetschema.ConnectionSelector) (*ToolSet, error)
	DeleteToolSet(ctx context.Context, id apid.ID) error

	// UpdateToolSetMetadata replaces the logical name, user labels, and
	// annotations without touching the selector or any generation.
	UpdateToolSetMetadata(ctx context.Context, id apid.ID, name scommon.ResourceName, userLabels map[string]string, annotations map[string]string) (*ToolSet, error)

	// ListToolSetsBuilder pages through live logical ToolSets within a namespace scope.
	ListToolSetsBuilder() ListToolSetsBuilder
	ListToolSetsFromCursor(ctx context.Context, cursor string) (ListToolSetsExecutor, error)

	// ListToolSetGenerationsBuilder pages through one live ToolSet's generations, newest first.
	ListToolSetGenerationsBuilder() ListToolSetGenerationsBuilder
	ListToolSetGenerationsFromCursor(ctx context.Context, cursor string) (ListToolSetGenerationsExecutor, error)

	// CreateToolSetGeneration allocates a new draft under a live parent. Only one
	// live draft may exist, and historical generation numbers are never reused.
	CreateToolSetGeneration(ctx context.Context, id apid.ID, definition encfield.EncryptedField) (*ToolSetGeneration, error)

	// GetToolSetGeneration returns one generation if it and its parent are live.
	GetToolSetGeneration(ctx context.Context, id apid.ID, generation uint64) (*ToolSetGeneration, error)

	// GetToolSetGenerationByState finds the sole draft or primary generation.
	GetToolSetGenerationByState(ctx context.Context, id apid.ID, state toolsetschema.ToolSetReleaseState) (*ToolSetGeneration, error)

	// UpdateToolSetGenerationDraft replaces only the expected editable revision
	// and advances its revision. Published plaintext cannot be changed here.
	UpdateToolSetGenerationDraft(ctx context.Context, id apid.ID, generation uint64, expectedRevision uint64, definition encfield.EncryptedField) (*ToolSetGeneration, error)

	// PublishToolSetGeneration atomically promotes the expected draft revision and
	// demotes the former primary. The caller must validate the definition and any
	// imported source snapshot before publishing; bindings are not migrated here.
	PublishToolSetGeneration(ctx context.Context, id apid.ID, generation uint64, expectedRevision uint64) (*ToolSetGeneration, error)

	// CreateToolSetBinding creates a pending assignment from the current matched
	// selector snapshot. It does not install a generation or inventory.
	CreateToolSetBinding(ctx context.Context, toolSetId apid.ID, connectionId apid.ID, expectedSelectorRevision uint64) (*ToolSetBinding, error)

	// GetToolSetBinding returns a live assignment whose two parents are live.
	GetToolSetBinding(ctx context.Context, id apid.ID) (*ToolSetBinding, error)

	// GetToolSetBindingForConnection finds the current assignment for a parent pair.
	GetToolSetBindingForConnection(ctx context.Context, toolSetId apid.ID, connectionId apid.ID) (*ToolSetBinding, error)

	// ListToolSetBindingsBuilder starts a listing of one connection's live assignments.
	ListToolSetBindingsBuilder() ListToolSetBindingsBuilder

	// ListToolSetBindingsFromCursor continues a binding listing with its saved filters.
	ListToolSetBindingsFromCursor(ctx context.Context, cursor string) (ListToolSetBindingsExecutor, error)

	// InstallExplicitToolSetInventory atomically pins a pending binding to its
	// prepared primary generation and installs its complete first Tool inventory.
	InstallExplicitToolSetInventory(ctx context.Context, install ToolSetInventoryInstall) (*ToolSetBinding, error)

	// ReplaceExplicitToolSetInventory replaces an installed inventory at its
	// captured epoch/generation, preserving identities for retained source keys.
	ReplaceExplicitToolSetInventory(ctx context.Context, install ToolSetInventoryInstall, expectedGeneration uint64) (*ToolSetBinding, error)

	// DeleteToolSetBinding retires a no-longer-matching assignment using its epoch
	// and current selector snapshot. Deleted-parent cleanup ignores selector revision.
	DeleteToolSetBinding(ctx context.Context, id apid.ID, expectedEpoch uint64, expectedSelectorRevision uint64) error

	/*
	 *  Standalone Tool identity and revision storage
	 */

	// CreateStandaloneTool atomically stores an identity and its first active definition.
	CreateStandaloneTool(ctx context.Context, tool *Tool, definition encfield.EncryptedField, digest string) (*ToolRevision, error)

	// GetTool reads a live identity, including those whose connection was deleted.
	GetTool(ctx context.Context, id apid.ID) (*Tool, error)

	// ListToolCandidates scans bounded current metadata for later authorization;
	// its internal pages and cursors must not be exposed as discovery results.
	ListToolCandidates(ctx context.Context, query ToolCandidateQuery) (ToolCandidatePage, error)

	// ListToolCandidatesFromCursor resumes a candidate scan with its saved filters
	// and limit. Callers must still authorize each returned candidate.
	ListToolCandidatesFromCursor(ctx context.Context, cursor string) (ToolCandidatePage, error)

	// ListToolsBuilder pages live Tool identities for management, independent of
	// connection liveness. Callers must still check per-resource permissions.
	ListToolsBuilder() ListToolsBuilder

	// ListToolsFromCursor resumes a management listing with its saved filters.
	ListToolsFromCursor(ctx context.Context, cursor string) (ListToolsExecutor, error)

	// UpdateStandaloneTool appends a revision and switches the pointer only if the observed revision is current.
	UpdateStandaloneTool(ctx context.Context, tool *Tool, expectedRevision uint64, definition encfield.EncryptedField, digest string) (*ToolRevision, error)

	// DeleteStandaloneTool removes the observed identity while retaining its revision history.
	DeleteStandaloneTool(ctx context.Context, id apid.ID, expectedRevision uint64) error

	// GetCurrentToolRevision reads the active definition and its connection identity in one SQL snapshot.
	GetCurrentToolRevision(ctx context.Context, id apid.ID) (*ToolRevision, error)

	// GetToolExecutionSnapshot captures the active definition, connection configuration,
	// pinned connector definition, and ToolSet provenance together for invocation admission.
	// Callers still authorize the revision and check expected revision and readiness.
	GetToolExecutionSnapshot(ctx context.Context, id apid.ID) (*ToolExecutionSnapshot, error)

	// GetToolRevision inspects retained history; it does not select a definition for invocation.
	GetToolRevision(ctx context.Context, id apid.ID, revision uint64) (*ToolRevision, error)

	/*
	 *  Connections
	 */

	GetConnection(ctx context.Context, id apid.ID) (*Connection, error)
	CreateConnection(ctx context.Context, c *Connection) error
	UpdateConnectionName(ctx context.Context, id apid.ID, name scommon.ResourceName) (*Connection, error)
	DeleteConnection(ctx context.Context, id apid.ID) error
	SetConnectionState(ctx context.Context, id apid.ID, state ConnectionState) error
	SetConnectionHealthState(ctx context.Context, id apid.ID, state ConnectionHealthState) error
	SetConnectionSetupStep(ctx context.Context, id apid.ID, setupStep *cschema.SetupStep) error
	SetConnectionSetupError(ctx context.Context, id apid.ID, setupError *string) error
	SetConnectionEncryptedConfiguration(ctx context.Context, id apid.ID, encryptedConfig *encfield.EncryptedField) error
	UpdateConnectionLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Connection, error)
	PutConnectionLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Connection, error)
	DeleteConnectionLabels(ctx context.Context, id apid.ID, keys []string) (*Connection, error)
	UpdateConnectionAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Connection, error)
	PutConnectionAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Connection, error)
	DeleteConnectionAnnotations(ctx context.Context, id apid.ID, keys []string) (*Connection, error)
	UpdateConnectionForGenerationMigration(ctx context.Context, update ConnectionGenerationMigrationUpdate) (*Connection, error)
	ListConnectionsBuilder() ListConnectionsBuilder
	ListConnectionsFromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error)

	/*
	 * Notifications
	 */
	UpsertNotification(ctx context.Context, upsert NotificationUpsert) (*Notification, error)
	GetNotification(ctx context.Context, id apid.ID) (*Notification, error)
	ListNotifications(ctx context.Context, opts ListNotificationsOptions) ([]Notification, error)
	MarkNotificationViewed(ctx context.Context, notificationID apid.ID, actorID apid.ID) error
	MarkNotificationsViewed(ctx context.Context, notificationIDs []apid.ID, actorID apid.ID) error
	NotificationViewedMap(ctx context.Context, actorID apid.ID, ids []apid.ID) (map[apid.ID]time.Time, error)
	ResolveNotificationsForResourceKeys(ctx context.Context, resourceType string, resourceID apid.ID, keys []string) error

	/*
	 * OAuth2 tokens
	 */
	GetOAuth2Token(ctx context.Context, connectionId apid.ID) (*OAuth2Token, error)
	InsertOAuth2Token(
		ctx context.Context,
		connectionId apid.ID,
		refreshedFrom *apid.ID,
		encryptedRefreshToken encfield.EncryptedField,
		encryptedAccessToken encfield.EncryptedField,
		accessTokenExpiresAt *time.Time,
		refreshTokenExpiry OAuth2RefreshTokenExpiry,
		scopes string,
		requestedScopes string,
		createdByActorId *apid.ID,
	) (*OAuth2Token, error)
	DeleteOAuth2Token(ctx context.Context, tokenId apid.ID) error
	DeleteAllOAuth2TokensForConnection(ctx context.Context, connectionId apid.ID) error

	// EnumerateOAuth2TokensExpiringWithin enumerates OAuth2 tokens that are expiring within a specified time interval
	// of now. This includes tokens that are already expired. It also includes tokens whose refresh token is due for
	// a proactive refresh (refresh_token_refresh_at has passed) regardless of access-token expiry. Deleted tokens are
	// not considered, nor are tokens tied to a deleted connection.
	EnumerateOAuth2TokensExpiringWithin(
		ctx context.Context,
		duration time.Duration,
		callback func(tokens []*OAuth2TokenWithConnection, lastPage bool) (keepGoing pagination.KeepGoing, err error),
	) error

	/*
	 * API Key credentials
	 */
	GetActiveApiKeyCredential(ctx context.Context, connectionId apid.ID) (*ApiKeyCredential, error)
	InsertApiKeyCredential(
		ctx context.Context,
		connectionId apid.ID,
		encryptedCredentials encfield.EncryptedField,
		placement *cschema.ApiKeyPlacement,
		createdByActorId *apid.ID,
	) (*ApiKeyCredential, error)
	UpdateApiKeyCredentialLastValidated(ctx context.Context, credentialId apid.ID, at time.Time) error
	DeleteAllApiKeyCredentialsForConnection(ctx context.Context, connectionId apid.ID) error

	/*
	 * Connection probe outcomes — append-only event log that drives the
	 * probe-driven health-check signal. The runtime walks the most-recent
	 * rows for each (connection_id, probe_id) to compute consecutive-success
	 * or -failure counts. A daily cleanup task caps growth (see
	 * internal/core/task_probe_outcome_cleanup.go).
	 */
	InsertProbeOutcome(ctx context.Context, connectionId apid.ID, probeId string, outcome string, errorMessage string) (*ConnectionProbeOutcome, error)
	GetRecentProbeOutcomes(ctx context.Context, connectionId apid.ID, probeId string, limit int) ([]*ConnectionProbeOutcome, error)
	DeleteOldProbeOutcomes(ctx context.Context, connectionId apid.ID, probeId string, keepMinimum int, olderThan time.Time) (int64, error)
	DistinctProbeIdsForConnection(ctx context.Context, connectionId apid.ID) ([]string, error)
	CountProbeOutcomes(ctx context.Context, connectionId apid.ID, probeId string) (int, error)

	/*
	 * Keys
	 */

	GetKey(ctx context.Context, id apid.ID) (*Key, error)
	CreateKey(ctx context.Context, ek *Key) error
	UpdateKeyName(ctx context.Context, id apid.ID, name scommon.ResourceName) (*Key, error)
	UpdateKey(ctx context.Context, id apid.ID, updates map[string]interface{}) (*Key, error)
	DeleteKey(ctx context.Context, id apid.ID) error
	// DeleteUnusedKey refuses keys with namespace references or any DEK history.
	DeleteUnusedKey(ctx context.Context, id apid.ID) error
	SetKeyState(ctx context.Context, id apid.ID, state KeyState) error
	UpdateKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Key, error)
	PutKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Key, error)
	DeleteKeyLabels(ctx context.Context, id apid.ID, keys []string) (*Key, error)
	UpdateKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Key, error)
	PutKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Key, error)
	DeleteKeyAnnotations(ctx context.Context, id apid.ID, keys []string) (*Key, error)
	ListKeysBuilder() ListKeysBuilder
	ListKeysFromCursor(ctx context.Context, cursor string) (ListKeysExecutor, error)

	// EnumerateKeysInDependencyOrder loads all non-deleted keys and walks them
	// in breadth-first order starting from the root key (the one with nil EncryptedKeyData).
	// The callback receives one depth-level of keys at a time, with depth 0 being the root.
	// Returns a slice of orphaned keys whose parent key could not be resolved.
	EnumerateKeysInDependencyOrder(
		ctx context.Context,
		callback func(keys []*Key, depth int) (keepGoing pagination.KeepGoing, err error),
	) ([]*Key, error)

	/*
	 * Data Encryption Keys
	 */

	CreateDataEncryptionKey(ctx context.Context, dek *DataEncryptionKey) error
	GetDataEncryptionKey(ctx context.Context, id apid.ID) (*DataEncryptionKey, error)
	GetCurrentDataEncryptionKeyForKey(ctx context.Context, keyId apid.ID) (*DataEncryptionKey, error)
	UpdateDataEncryptionKeyWrapping(ctx context.Context, dek *DataEncryptionKey) error
	ClearCurrentDataEncryptionKeyFlagForKey(ctx context.Context, keyId apid.ID) error
	SetDataEncryptionKeyCurrentFlag(ctx context.Context, id apid.ID, isCurrent bool) error
	ListDataEncryptionKeysForKey(ctx context.Context, keyId apid.ID) ([]*DataEncryptionKey, error)
	EnumerateDataEncryptionKeysForKey(
		ctx context.Context,
		keyId apid.ID,
		callback func(deks []*DataEncryptionKey, lastPage bool) (keepGoing pagination.KeepGoing, err error),
	) error

	/*
	 * Rate Limits
	 */

	GetRateLimit(ctx context.Context, id apid.ID) (*RateLimit, error)
	CreateRateLimit(ctx context.Context, rl *RateLimit) error
	UpdateRateLimitName(ctx context.Context, id apid.ID, name scommon.ResourceName) (*RateLimit, error)
	UpdateRateLimitDefinition(ctx context.Context, id apid.ID, def rlschema.RateLimitSpec) (*RateLimit, error)
	DeleteRateLimit(ctx context.Context, id apid.ID) error
	UpdateRateLimitLabels(ctx context.Context, id apid.ID, labels map[string]string) (*RateLimit, error)
	PutRateLimitLabels(ctx context.Context, id apid.ID, labels map[string]string) (*RateLimit, error)
	DeleteRateLimitLabels(ctx context.Context, id apid.ID, keys []string) (*RateLimit, error)
	UpdateRateLimitAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*RateLimit, error)
	PutRateLimitAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*RateLimit, error)
	DeleteRateLimitAnnotations(ctx context.Context, id apid.ID, keys []string) (*RateLimit, error)
	ListRateLimitsBuilder() ListRateLimitsBuilder
	ListRateLimitsFromCursor(ctx context.Context, cursor string) (ListRateLimitsExecutor, error)

	/*
	 * Re-encryption
	 */

	// EnumerateFieldsRequiringReEncryption walks all registered encrypted fields across all tables,
	// finding rows whose encrypted field EKV ID does not match the namespace's target EKV ID.
	EnumerateFieldsRequiringReEncryption(
		ctx context.Context,
		callback func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error),
	) error

	// BatchUpdateReEncryptedFields sets the new value and encrypted_at only on live
	// rows whose encrypted field still matches ExpectedValue. Stale updates are
	// skipped without error; earlier updates may be applied if a later one fails.
	BatchUpdateReEncryptedFields(ctx context.Context, updates []ReEncryptedFieldUpdate) error

	/*
	 *  Purge
	 */

	// PurgeSoftDeletedRecords hard-deletes all soft-deleted records where deleted_at is before olderThan.
	// Returns the total number of records deleted across all tables.
	PurgeSoftDeletedRecords(ctx context.Context, olderThan time.Time) (int64, error)

	// RefreshNamespaceLabelsCarryForward re-derives the materialized apxy/
	// portion of every resource that inherits from nsPath, then walks each
	// direct child namespace, recomputes its labels, and recurses. Each
	// row's update runs in its own short transaction. Intended to be
	// invoked from a background asynq task — a label change on a deeply
	// nested namespace can fan out to many descendants.
	RefreshNamespaceLabelsCarryForward(ctx context.Context, nsPath string) error

	// RefreshConnectionsForConnector re-derives the materialized
	// apxy/ portion of every connection pointing at the given logical
	// connector. Each connection's update runs in its own short
	// transaction. Intended to be invoked from a background asynq task
	// after a connector's user labels change.
	RefreshConnectionsForConnector(ctx context.Context, id apid.ID) error

	// ReconcileCarryForwardLabels walks every labelled resource in batches
	// of `batchSize` and re-derives the materialized apxy/ portion of
	// each row. The optional `limiter` is consulted before each row is
	// processed, providing a per-row records/sec rate limit; nil means
	// unlimited. Drift is rare under normal operation, but this method
	// is the safety net for any propagation task that misfired or any
	// data path that bypassed the carry-forward triggers. Intended to be
	// invoked from a daily asynq cron task. Progress is checkpointed in the
	// database, so a call continues any unfinished sweep; cancel ctx to stop
	// a run early and let a later call pick up from there. Returns the number
	// of rows whose labels were corrected by this call.
	ReconcileCarryForwardLabels(ctx context.Context, batchSize int32, limiter *rate.Limiter) (corrected int64, err error)

	/*
	 *  Nonces
	 */

	HasNonceBeenUsed(ctx context.Context, nonce apid.ID) (hasBeenUsed bool, err error)
	CheckNonceValidAndMarkUsed(ctx context.Context, nonce apid.ID, retainRecordUntil time.Time) (wasValid bool, err error)
	DeleteExpiredNonces(ctx context.Context) (err error)
}

package database

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
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
	Migrate(ctx context.Context) error
	Ping(ctx context.Context) bool

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

	GetConnectorVersion(ctx context.Context, id apid.ID, version uint64) (*ConnectorVersion, error)
	GetConnectorVersions(ctx context.Context, requested []ConnectorVersionId) (map[ConnectorVersionId]*ConnectorVersion, error)
	GetConnectorVersionForLabels(ctx context.Context, labelSelector string) (*ConnectorVersion, error)
	GetConnectorVersionForLabelsAndVersion(ctx context.Context, labelSelector string, version uint64) (*ConnectorVersion, error)
	GetConnectorVersionForState(ctx context.Context, id apid.ID, state ConnectorVersionState) (*ConnectorVersion, error)
	NewestConnectorVersionForId(ctx context.Context, id apid.ID) (*ConnectorVersion, error)
	NewestPublishedConnectorVersionForId(ctx context.Context, id apid.ID) (*ConnectorVersion, error)
	UpsertConnectorVersion(ctx context.Context, cv *ConnectorVersion) error
	SetConnectorVersionState(ctx context.Context, id apid.ID, version uint64, state ConnectorVersionState) error
	DeleteConnector(ctx context.Context, id apid.ID) error
	ListConnectorVersionsBuilder() ListConnectorVersionsBuilder
	ListConnectorVersionsFromCursor(ctx context.Context, cursor string) (ListConnectorVersionsExecutor, error)
	ListConnectorsBuilder() ListConnectorsBuilder
	ListConnectorsFromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error)

	/*
	 *  Connections
	 */

	GetConnection(ctx context.Context, id apid.ID) (*Connection, error)
	CreateConnection(ctx context.Context, c *Connection) error
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
	UpdateConnectionForVersionMigration(ctx context.Context, update ConnectionVersionMigrationUpdate) (*Connection, error)
	ListConnectionsBuilder() ListConnectionsBuilder
	ListConnectionsFromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error)

	/*
	 * Notifications
	 */
	UpsertNotification(ctx context.Context, upsert NotificationUpsert) (*Notification, error)
	GetNotification(ctx context.Context, id apid.ID) (*Notification, error)
	ListNotifications(ctx context.Context, opts ListNotificationsOptions) ([]Notification, error)
	MarkNotificationViewed(ctx context.Context, notificationID apid.ID, actorID apid.ID) error
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
		scopes string,
		requestedScopes string,
		createdByActorId *apid.ID,
	) (*OAuth2Token, error)
	DeleteOAuth2Token(ctx context.Context, tokenId apid.ID) error
	DeleteAllOAuth2TokensForConnection(ctx context.Context, connectionId apid.ID) error

	// EnumerateOAuth2TokensExpiringWithin enumerates OAuth2 tokens that are expiring within a specified time interval
	// of now. This includes tokens that are already expired. Deleted tokens are not considered, nor are tokens tied
	// to a deleted connection.
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
	UpdateKey(ctx context.Context, id apid.ID, updates map[string]interface{}) (*Key, error)
	DeleteKey(ctx context.Context, id apid.ID) error
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
	UpdateRateLimitDefinition(ctx context.Context, id apid.ID, def rlschema.RateLimit) (*RateLimit, error)
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

	// BatchUpdateReEncryptedFields updates encrypted field values after re-encryption,
	// setting the new value and updating encrypted_at.
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

	// RefreshConnectionsForConnectorVersion re-derives the materialized
	// apxy/ portion of every connection pointing at the given (id,
	// version). Each connection's update runs in its own short
	// transaction. Intended to be invoked from a background asynq task
	// after a connector version's user labels change (only meaningful for
	// draft versions; primary and active are immutable).
	RefreshConnectionsForConnectorVersion(ctx context.Context, id apid.ID, version uint64) error

	// ReconcileCarryForwardLabels walks every labelled resource in batches
	// of `batchSize` and re-derives the materialized apxy/ portion of
	// each row. The optional `limiter` is consulted before each row is
	// processed, providing a per-row records/sec rate limit; nil means
	// unlimited. Drift is rare under normal operation, but this method
	// is the safety net for any propagation task that misfired or any
	// data path that bypassed the carry-forward triggers. Intended to be
	// invoked from a daily asynq cron task. Returns the total number of
	// rows whose labels were corrected.
	ReconcileCarryForwardLabels(ctx context.Context, batchSize int32, limiter *rate.Limiter) (corrected int64, err error)

	/*
	 *  Nonces
	 */

	HasNonceBeenUsed(ctx context.Context, nonce apid.ID) (hasBeenUsed bool, err error)
	CheckNonceValidAndMarkUsed(ctx context.Context, nonce apid.ID, retainRecordUntil time.Time) (wasValid bool, err error)
	DeleteExpiredNonces(ctx context.Context) (err error)
}

package database

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/util/pagination"
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
	SetNamespaceEncryptionKeyId(ctx context.Context, path string, ekId *apid.ID) (*Namespace, error)
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
		callback func(targets []NamespaceEncryptionTarget, lastPage bool) (updates []NamespaceTargetEncryptionKeyVersionUpdate, stop bool, err error),
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
	SetConnectionSetupStep(ctx context.Context, id apid.ID, setupStep *string) error
	SetConnectionEncryptedConfiguration(ctx context.Context, id apid.ID, encryptedConfig *encfield.EncryptedField) error
	UpdateConnectionLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Connection, error)
	PutConnectionLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Connection, error)
	DeleteConnectionLabels(ctx context.Context, id apid.ID, keys []string) (*Connection, error)
	UpdateConnectionAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Connection, error)
	PutConnectionAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Connection, error)
	DeleteConnectionAnnotations(ctx context.Context, id apid.ID, keys []string) (*Connection, error)
	ListConnectionsBuilder() ListConnectionsBuilder
	ListConnectionsFromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error)

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
	) (*OAuth2Token, error)
	DeleteOAuth2Token(ctx context.Context, tokenId apid.ID) error
	DeleteAllOAuth2TokensForConnection(ctx context.Context, connectionId apid.ID) error

	// EnumerateOAuth2TokensExpiringWithin enumerates OAuth2 tokens that are expiring within a specified time interval
	// of now. This includes tokens that are already expired. Deleted tokens are not considered, nor are tokens tied
	// to a deleted connection.
	EnumerateOAuth2TokensExpiringWithin(
		ctx context.Context,
		duration time.Duration,
		callback func(tokens []*OAuth2TokenWithConnection, lastPage bool) (stop bool, err error),
	) error

	/*
	 * Encryption Keys
	 */

	GetEncryptionKey(ctx context.Context, id apid.ID) (*EncryptionKey, error)
	CreateEncryptionKey(ctx context.Context, ek *EncryptionKey) error
	UpdateEncryptionKey(ctx context.Context, id apid.ID, updates map[string]interface{}) (*EncryptionKey, error)
	DeleteEncryptionKey(ctx context.Context, id apid.ID) error
	SetEncryptionKeyState(ctx context.Context, id apid.ID, state EncryptionKeyState) error
	UpdateEncryptionKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (*EncryptionKey, error)
	PutEncryptionKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (*EncryptionKey, error)
	DeleteEncryptionKeyLabels(ctx context.Context, id apid.ID, keys []string) (*EncryptionKey, error)
	UpdateEncryptionKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*EncryptionKey, error)
	PutEncryptionKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*EncryptionKey, error)
	DeleteEncryptionKeyAnnotations(ctx context.Context, id apid.ID, keys []string) (*EncryptionKey, error)
	ListEncryptionKeysBuilder() ListEncryptionKeysBuilder
	ListEncryptionKeysFromCursor(ctx context.Context, cursor string) (ListEncryptionKeysExecutor, error)

	// EnumerateEncryptionKeysInDependencyOrder loads all non-deleted encryption keys and walks them
	// in breadth-first order starting from the root key (the one with nil EncryptedKeyData).
	// The callback receives one depth-level of keys at a time, with depth 0 being the root.
	// Returns a slice of orphaned keys whose parent encryption key version could not be resolved.
	EnumerateEncryptionKeysInDependencyOrder(
		ctx context.Context,
		callback func(keys []*EncryptionKey, depth int) (stop bool, err error),
	) ([]*EncryptionKey, error)

	/*
	 * Encryption Key Versions
	 */

	CreateEncryptionKeyVersion(ctx context.Context, ekv *EncryptionKeyVersion) error
	GetEncryptionKeyVersion(ctx context.Context, id apid.ID) (*EncryptionKeyVersion, error)
	GetCurrentEncryptionKeyVersionForEncryptionKey(ctx context.Context, encryptionKeyId apid.ID) (*EncryptionKeyVersion, error)
	ListEncryptionKeyVersionsForEncryptionKey(ctx context.Context, encryptionKeyId apid.ID) ([]*EncryptionKeyVersion, error)
	GetMaxOrderedVersionForEncryptionKey(ctx context.Context, encryptionKeyId apid.ID) (int64, error)
	ClearCurrentFlagForEncryptionKey(ctx context.Context, encryptionKeyId apid.ID) error
	GetCurrentEncryptionKeyVersionForNamespace(ctx context.Context, namespacePath string) (*EncryptionKeyVersion, error)
	ListEncryptionKeyVersionsForNamespace(ctx context.Context, namespacePath string) ([]*EncryptionKeyVersion, error)
	GetMaxOrderedVersionForNamespace(ctx context.Context, namespacePath string) (int64, error)
	ClearCurrentFlagForNamespace(ctx context.Context, namespacePath string) error
	DeleteEncryptionKeyVersion(ctx context.Context, id apid.ID) error
	DeleteEncryptionKeyVersionsForEncryptionKey(ctx context.Context, encryptionKeyId apid.ID) error
	SetEncryptionKeyVersionCurrentFlag(ctx context.Context, id apid.ID, isCurrent bool) error

	// EnumerateEncryptionKeyVersionsForKey enumerates all non-deleted encryption key versions for a
	// specified key in batches.
	EnumerateEncryptionKeyVersionsForKey(
		ctx context.Context,
		ekId apid.ID,
		callback func(ekvs []*EncryptionKeyVersion, lastPage bool) (stop bool, err error),
	) error

	/*
	 * Re-encryption
	 */

	// EnumerateFieldsRequiringReEncryption walks all registered encrypted fields across all tables,
	// finding rows whose encrypted field EKV ID does not match the namespace's target EKV ID.
	EnumerateFieldsRequiringReEncryption(
		ctx context.Context,
		callback func(targets []ReEncryptionTarget, lastPage bool) (stop bool, err error),
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

	/*
	 *  Nonces
	 */

	HasNonceBeenUsed(ctx context.Context, nonce apid.ID) (hasBeenUsed bool, err error)
	CheckNonceValidAndMarkUsed(ctx context.Context, nonce apid.ID, retainRecordUntil time.Time) (wasValid bool, err error)
	DeleteExpiredNonces(ctx context.Context) (err error)
}

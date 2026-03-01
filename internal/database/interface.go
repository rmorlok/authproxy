package database

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
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
	GetEncryptedKey() *string
}

//go:generate mockgen -source=./interface.go -destination=./mock/db.go -package=mock
type DB interface {
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
	UpdateNamespaceLabels(ctx context.Context, path string, labels map[string]string) (*Namespace, error)
	PutNamespaceLabels(ctx context.Context, path string, labels map[string]string) (*Namespace, error)
	DeleteNamespaceLabels(ctx context.Context, path string, keys []string) (*Namespace, error)
	ListNamespacesBuilder() ListNamespacesBuilder
	ListNamespacesFromCursor(ctx context.Context, cursor string) (ListNamespacesExecutor, error)

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
	UpdateConnectionLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Connection, error)
	PutConnectionLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Connection, error)
	DeleteConnectionLabels(ctx context.Context, id apid.ID, keys []string) (*Connection, error)
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
		encryptedRefreshToken string,
		encryptedAccessToken string,
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
	 *  Nonces
	 */

	HasNonceBeenUsed(ctx context.Context, nonce apid.ID) (hasBeenUsed bool, err error)
	CheckNonceValidAndMarkUsed(ctx context.Context, nonce apid.ID, retainRecordUntil time.Time) (wasValid bool, err error)
	DeleteExpiredNonces(ctx context.Context) (err error)
}

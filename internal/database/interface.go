package database

import (
	"context"
	"time"

	"github.com/google/uuid"
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
	GetId() uuid.UUID
	GetExternalId() string
	GetPermissions() []aschema.Permission
	IsAdmin() bool
	IsSuperAdmin() bool
	GetEmail() string
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
	DeleteNamespace(ctx context.Context, path string) error
	SetNamespaceState(ctx context.Context, path string, state NamespaceState) error
	ListNamespacesBuilder() ListNamespacesBuilder
	ListNamespacesFromCursor(ctx context.Context, cursor string) (ListNamespacesExecutor, error)

	/*
	 *  Actors
	 */

	GetActor(ctx context.Context, id uuid.UUID) (*Actor, error)
	GetActorByExternalId(ctx context.Context, externalId string) (*Actor, error)
	CreateActor(ctx context.Context, actor *Actor) error
	UpsertActor(ctx context.Context, actor IActorData) (*Actor, error)
	DeleteActor(ctx context.Context, id uuid.UUID) error
	ListActorsBuilder() ListActorsBuilder
	ListActorsFromCursor(ctx context.Context, cursor string) (ListActorsExecutor, error)

	/*
	 * Connectors
	 */

	GetConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (*ConnectorVersion, error)
	GetConnectorVersions(ctx context.Context, requested []ConnectorVersionId) (map[ConnectorVersionId]*ConnectorVersion, error)
	GetConnectorVersionForTypeAndVersion(ctx context.Context, typ string, version uint64) (*ConnectorVersion, error)
	GetConnectorVersionForType(ctx context.Context, typ string) (*ConnectorVersion, error)
	GetConnectorVersionForState(ctx context.Context, id uuid.UUID, state ConnectorVersionState) (*ConnectorVersion, error)
	NewestConnectorVersionForId(ctx context.Context, id uuid.UUID) (*ConnectorVersion, error)
	NewestPublishedConnectorVersionForId(ctx context.Context, id uuid.UUID) (*ConnectorVersion, error)
	UpsertConnectorVersion(ctx context.Context, cv *ConnectorVersion) error
	ListConnectorVersionsBuilder() ListConnectorVersionsBuilder
	ListConnectorVersionsFromCursor(ctx context.Context, cursor string) (ListConnectorVersionsExecutor, error)
	ListConnectorsBuilder() ListConnectorsBuilder
	ListConnectorsFromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error)

	/*
	 *  Connections
	 */

	GetConnection(ctx context.Context, id uuid.UUID) (*Connection, error)
	CreateConnection(ctx context.Context, c *Connection) error
	DeleteConnection(ctx context.Context, id uuid.UUID) error
	SetConnectionState(ctx context.Context, id uuid.UUID, state ConnectionState) error
	ListConnectionsBuilder() ListConnectionsBuilder
	ListConnectionsFromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error)

	/*
	 * OAuth2 tokens
	 */
	GetOAuth2Token(ctx context.Context, connectionId uuid.UUID) (*OAuth2Token, error)
	InsertOAuth2Token(
		ctx context.Context,
		connectionId uuid.UUID,
		refreshedFrom *uuid.UUID,
		encryptedRefreshToken string,
		encryptedAccessToken string,
		accessTokenExpiresAt *time.Time,
		scopes string,
	) (*OAuth2Token, error)
	DeleteOAuth2Token(ctx context.Context, tokenId uuid.UUID) error
	DeleteAllOAuth2TokensForConnection(ctx context.Context, connectionId uuid.UUID) error

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

	HasNonceBeenUsed(ctx context.Context, nonce uuid.UUID) (hasBeenUsed bool, err error)
	CheckNonceValidAndMarkUsed(ctx context.Context, nonce uuid.UUID, retainRecordUntil time.Time) (wasValid bool, err error)
}

package database

import (
	"context"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/jwt"
	"time"
)

//go:generate mockgen -source=./interface.go -destination=./mock/db.go -package=mock
type DB interface {
	Migrate(ctx context.Context) error
	Ping(ctx context.Context) bool

	/*
	 *  Actors
	 */

	GetActor(ctx context.Context, id uuid.UUID) (*Actor, error)
	GetActorByExternalId(ctx context.Context, externalId string) (*Actor, error)
	CreateActor(ctx context.Context, actor *Actor) error
	UpsertActor(ctx context.Context, actor *jwt.Actor) (*Actor, error)
	ListActorsBuilder() ListActorsBuilder
	ListActorsFromCursor(ctx context.Context, cursor string) (ListActorsExecutor, error)

	/*
	 * Connectors
	 */

	GetConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (*ConnectorVersion, error)
	GetConnectorVersionForTypeAndVersion(ctx context.Context, typ string, version uint64) (*ConnectorVersion, error)
	GetConnectorVersionForType(ctx context.Context, typ string) (*ConnectorVersion, error)
	GetConnectorVersionForState(ctx context.Context, id uuid.UUID, state ConnectorVersionState) (*ConnectorVersion, error)
	NewestConnectorVersionForId(ctx context.Context, id uuid.UUID) (*ConnectorVersion, error)
	NewestPublishedConnectorVersionForId(ctx context.Context, id uuid.UUID) (*ConnectorVersion, error)
	UpsertConnectorVersion(ctx context.Context, cv *ConnectorVersion) error
	ListConnectorsBuilder() ListConnectorsBuilder
	ListConnectorsFromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error)

	/*
	 *  Connections
	 */

	GetConnection(ctx context.Context, id uuid.UUID) (*Connection, error)
	CreateConnection(ctx context.Context, c *Connection) error
	DeleteConnection(ctx context.Context, id uuid.UUID) error
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

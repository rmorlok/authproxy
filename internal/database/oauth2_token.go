package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

func init() {
	RegisterEncryptedField(EncryptedFieldRegistration{
		Table:            OAuth2TokensTable,
		PrimaryKeyCols:   []string{"id"},
		EncryptedCols:    []string{"encrypted_refresh_token", "encrypted_access_token"},
		JoinTable:        ConnectionsTable,
		JoinLocalCol:     "connection_id",
		JoinRemoteCol:    "id",
		JoinNamespaceCol: "namespace",
	})
}

const OAuth2TokensTable = "oauth2_tokens"

const OAuth2AccessTokenExpiryBuffer = 30 * time.Second

// OAuth2RefreshTokenExpiry carries what is known about a refresh token's
// lifetime when inserting a token row. The zero value means "unknown".
type OAuth2RefreshTokenExpiry struct {
	// ExpiresAt is when the refresh token stops working.
	ExpiresAt *time.Time
	// RefreshAt is when to proactively refresh to keep the connection alive.
	RefreshAt *time.Time
}

type OAuth2Token struct {
	Id                    apid.ID
	ConnectionId          apid.ID // Foreign key to Connection; not enforced by database
	RefreshedFromId       *apid.ID
	EncryptedRefreshToken encfield.EncryptedField
	EncryptedAccessToken  encfield.EncryptedField
	AccessTokenExpiresAt  *time.Time
	// RefreshTokenExpiresAt is when the provider says the refresh token
	// stops working, derived from refresh_token_expires_in and related
	// token-response fields. Nil when the provider did not say.
	RefreshTokenExpiresAt *time.Time
	// RefreshTokenRefreshAt is when the background refresh task should
	// proactively refresh this token so that a new (or renewed) refresh
	// token is obtained before RefreshTokenExpiresAt. Nil when refreshing
	// cannot extend the refresh token's life or background refresh is off.
	RefreshTokenRefreshAt *time.Time
	Scopes                string
	RequestedScopes       string
	CreatedByActorId      *apid.ID // Actor who initiated this token (carried forward on refresh)
	CreatedAt             time.Time
	EncryptedAt           *time.Time
	DeletedAt             *time.Time
}

func (t *OAuth2Token) cols() []string {
	return []string{
		"id",
		"connection_id",
		"refreshed_from_id",
		"encrypted_refresh_token",
		"encrypted_access_token",
		"access_token_expires_at",
		"refresh_token_expires_at",
		"refresh_token_refresh_at",
		"scopes",
		"requested_scopes",
		"created_by_actor_id",
		"created_at",
		"encrypted_at",
		"deleted_at",
	}
}

func (t *OAuth2Token) fields() []any {
	return []any{
		&t.Id,
		&t.ConnectionId,
		&t.RefreshedFromId,
		&t.EncryptedRefreshToken,
		&t.EncryptedAccessToken,
		&t.AccessTokenExpiresAt,
		&t.RefreshTokenExpiresAt,
		&t.RefreshTokenRefreshAt,
		&t.Scopes,
		&t.RequestedScopes,
		&t.CreatedByActorId,
		&t.CreatedAt,
		&t.EncryptedAt,
		&t.DeletedAt,
	}
}

func (t *OAuth2Token) values() []any {
	return []any{
		t.Id,
		t.ConnectionId,
		t.RefreshedFromId,
		t.EncryptedRefreshToken,
		t.EncryptedAccessToken,
		t.AccessTokenExpiresAt,
		t.RefreshTokenExpiresAt,
		t.RefreshTokenRefreshAt,
		t.Scopes,
		t.RequestedScopes,
		t.CreatedByActorId,
		t.CreatedAt,
		t.EncryptedAt,
		t.DeletedAt,
	}
}

func (t *OAuth2Token) IsAccessTokenExpired(ctx context.Context) bool {
	if t.AccessTokenExpiresAt == nil {
		return false
	}

	return !t.AccessTokenExpiresAt.After(apctx.GetClock(ctx).Now().Add(OAuth2AccessTokenExpiryBuffer))
}

func (t *OAuth2Token) Validate() error {
	result := &multierror.Error{}

	if t.Id == apid.Nil {
		result = multierror.Append(result, errors.New("oauth2 token id is required"))
	}

	if err := t.Id.ValidatePrefix(apid.PrefixOAuth2Token); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid oauth2 token id: %w", err))
	}

	if t.ConnectionId == apid.Nil {
		result = multierror.Append(result, errors.New("oauth2 token connection id is required"))
	}

	if err := t.ConnectionId.ValidatePrefix(apid.PrefixConnection); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid oauth2 token connection id: %w", err))
	}

	if t.RefreshedFromId != nil {
		if err := t.RefreshedFromId.ValidatePrefix(apid.PrefixOAuth2Token); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid oauth2 token refreshed from id: %w", err))
		}
	}

	if t.CreatedByActorId != nil {
		if err := t.CreatedByActorId.ValidatePrefix(apid.PrefixActor); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid oauth2 token created_by_actor_id: %w", err))
		}
	}

	return result.ErrorOrNil()
}

func (s *service) GetOAuth2Token(
	ctx context.Context,
	connectionId apid.ID,
) (*OAuth2Token, error) {
	var result OAuth2Token
	err := s.sq.
		Select(result.cols()...).
		From(OAuth2TokensTable).
		Where(sq.Eq{
			"connection_id": connectionId,
			"deleted_at":    nil,
		}).
		OrderBy("created_at DESC").
		Limit(1).
		RunWith(s.db).
		QueryRow().
		Scan(result.fields()...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("no OAuth2 token found for connection Id: %w", ErrNotFound)
		}

		return nil, err
	}

	return &result, nil
}

func (s *service) DeleteOAuth2Token(
	ctx context.Context,
	tokenId apid.ID,
) error {
	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(OAuth2TokensTable).
		Set("deleted_at", now).
		Where(sq.Eq{"id": tokenId}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to soft delete oauth token: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to soft delete oauth token: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	if affected > 1 {
		return fmt.Errorf("multiple oauth tokens were soft deleted: %w", ErrViolation)
	}

	return nil
}

func (s *service) DeleteAllOAuth2TokensForConnection(
	ctx context.Context,
	connectionId apid.ID,
) error {
	logger := aplog.NewBuilder(s.logger).
		WithCtx(ctx).
		WithConnectionId(connectionId).
		Build()
	logger.Debug("deleting all oauth tokens for connection")

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(OAuth2TokensTable).
		Set("deleted_at", now).
		Where(sq.Eq{"connection_id": connectionId}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to soft delete oauth tokens for connection: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to soft delete oauth tokens for connection: %w", err)
	}

	logger.Info("deleted oauth tokens for connection", "affected", affected)

	return nil
}

func (s *service) InsertOAuth2Token(
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
) (*OAuth2Token, error) {
	logger := aplog.NewBuilder(s.logger).
		WithCtx(ctx).
		WithConnectionId(connectionId).
		Build()
	logger.Debug("inserting new oauth token")

	now := apctx.GetClock(ctx).Now()
	var newToken *OAuth2Token

	err := s.transaction(func(tx *sql.Tx) error {
		// Check if a token exists for refreshedFrom
		dbResult, err := s.sq.Update(OAuth2TokensTable).
			Set("deleted_at", now).
			Where(sq.Eq{
				"connection_id": connectionId,
				"deleted_at":    nil,
			}).
			RunWith(tx).
			Exec()
		if err != nil {
			return fmt.Errorf("failed to soft delete old oauth tokens as part of inserting new token: %w", err)
		}

		affected, err := dbResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to soft delete old oauth tokens as part of inserting new token: %w", err)
		}

		logger.Info("deleted previous oauth tokens for connection as part of inserting new", "affected", affected)

		// Create a new token
		newToken = &OAuth2Token{
			Id:                    apctx.GetIdGenerator(ctx).New(apid.PrefixOAuth2Token),
			ConnectionId:          connectionId,
			RefreshedFromId:       refreshedFrom,
			EncryptedRefreshToken: encryptedRefreshToken,
			EncryptedAccessToken:  encryptedAccessToken,
			AccessTokenExpiresAt:  accessTokenExpiresAt,
			RefreshTokenExpiresAt: refreshTokenExpiry.ExpiresAt,
			RefreshTokenRefreshAt: refreshTokenExpiry.RefreshAt,
			Scopes:                scopes,
			RequestedScopes:       requestedScopes,
			CreatedByActorId:      createdByActorId,
			CreatedAt:             now,
		}

		if err := newToken.Validate(); err != nil {
			return err
		}

		result, err := s.sq.
			Insert(OAuth2TokensTable).
			Columns(newToken.cols()...).
			Values(newToken.values()...).
			RunWith(tx).
			Exec()
		if err != nil {
			return fmt.Errorf("failed to create oauth token: %w", err)
		}

		affected, err = result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to create oauth token: %w", err)
		}

		if affected == 0 {
			return errors.New("failed to create oauth token; no rows inserted")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return newToken, nil
}

type OAuth2TokenWithConnection struct {
	Token      OAuth2Token
	Connection Connection
}

func (s *service) EnumerateOAuth2TokensExpiringWithin(
	ctx context.Context,
	duration time.Duration,
	callback func(tokens []*OAuth2TokenWithConnection, lastPage bool) (keepGoing pagination.KeepGoing, err error),
) error {
	const pageSize = 100
	now := apctx.GetClock(ctx).Now()
	expirationThreshold := now.Add(duration)

	offset := uint64(0)

	for {
		rows, err := s.sq.
			Select(
				append(
					util.PrependAll("t.", util.ToPtr(OAuth2Token{}).cols()),
					util.PrependAll("c.", util.ToPtr(Connection{}).cols())...,
				)...,
			).
			From(OAuth2TokensTable + " AS t").
			InnerJoin(ConnectionsTable + " AS c ON c.id = t.connection_id").
			Where(sq.Eq{
				"t.deleted_at": nil,
				"c.deleted_at": nil,
				"c.state":      ConnectionStateConfigured,
			}).
			// Either the access token is about to expire, or the refresh
			// token is approaching its own expiry and is due for a
			// proactive refresh to keep the connection alive.
			Where(sq.Or{
				sq.LtOrEq{"t.access_token_expires_at": expirationThreshold},
				sq.LtOrEq{"t.refresh_token_refresh_at": now},
			}).
			OrderBy("t.created_at DESC").
			Limit(pageSize + 1).
			Offset(offset).
			RunWith(s.db).
			Query()
		if err != nil {
			return err
		}

		var results []*OAuth2TokenWithConnection
		for rows.Next() {
			var r OAuth2TokenWithConnection
			err := rows.Scan(append(r.Token.fields(), r.Connection.fields()...)...)
			if err != nil {
				return err
			}
			results = append(results, &r)
		}

		lastPage := len(results) < pageSize+1

		if len(results) > pageSize {
			results = results[:pageSize]
		}

		keepGoing, err := callback(results, lastPage)
		if err != nil {
			return err
		}

		if keepGoing == pagination.Stop || lastPage {
			break
		}

		offset += pageSize
	}

	return nil
}

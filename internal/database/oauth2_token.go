package database

import (
	"context"
	"database/sql"
	"time"

	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/util"
)

const OAuth2TokensTable = "oauth2_tokens"

type OAuth2Token struct {
	Id                    apid.ID
	ConnectionId          apid.ID // Foreign key to Connection; not enforced by database
	RefreshedFromId       *apid.ID
	EncryptedRefreshToken string
	EncryptedAccessToken  string
	AccessTokenExpiresAt  *time.Time
	Scopes                string
	CreatedAt             time.Time
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
		"scopes",
		"created_at",
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
		&t.Scopes,
		&t.CreatedAt,
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
		t.Scopes,
		t.CreatedAt,
		t.DeletedAt,
	}
}

func (t *OAuth2Token) IsAccessTokenExpired(ctx context.Context) bool {
	if t.AccessTokenExpiresAt == nil {
		return false
	}

	return t.AccessTokenExpiresAt.Before(apctx.GetClock(ctx).Now())
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
			return nil, errors.Wrap(ErrNotFound, "no OAuth2 token found for connection Id")
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
		return errors.Wrap(err, "failed to soft delete oauth token")
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to soft delete oauth token")
	}

	if affected == 0 {
		return ErrNotFound
	}

	if affected > 1 {
		return errors.Wrap(ErrViolation, "multiple oauth tokens were soft deleted")
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
		return errors.Wrap(err, "failed to soft delete oauth tokens for connection")
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to soft delete oauth tokens for connection")
	}

	logger.Info("deleted oauth tokens for connection", "affected", affected)

	return nil
}

func (s *service) InsertOAuth2Token(
	ctx context.Context,
	connectionId apid.ID,
	refreshedFrom *apid.ID,
	encryptedRefreshToken string,
	encryptedAccessToken string,
	accessTokenExpiresAt *time.Time,
	scopes string,
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
			return errors.Wrap(err, "failed to soft delete old oauth tokens as part of inserting new token")
		}

		affected, err := dbResult.RowsAffected()
		if err != nil {
			return errors.Wrap(err, "failed to soft delete old oauth tokens as part of inserting new token")
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
			Scopes:                scopes,
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
			return errors.Wrap(err, "failed to create oauth token")
		}

		affected, err = result.RowsAffected()
		if err != nil {
			return errors.Wrap(err, "failed to create oauth token")
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

func (s *service) EnumerateOAuth2Tokens(
	ctx context.Context,
	callback func(tokens []*OAuth2Token, lastPage bool) (stop bool, err error),
) error {
	const pageSize = 100
	offset := uint64(0)

	for {
		rows, err := s.sq.
			Select(util.ToPtr(OAuth2Token{}).cols()...).
			From(OAuth2TokensTable).
			Where(sq.Eq{"deleted_at": nil}).
			OrderBy("id").
			Limit(pageSize + 1).
			Offset(offset).
			RunWith(s.db).
			Query()
		if err != nil {
			return err
		}

		var results []*OAuth2Token
		for rows.Next() {
			var r OAuth2Token
			if err := rows.Scan(r.fields()...); err != nil {
				rows.Close()
				return err
			}
			results = append(results, &r)
		}
		rows.Close()

		lastPage := len(results) <= pageSize
		if len(results) > pageSize {
			results = results[:pageSize]
		}

		stop, err := callback(results, lastPage)
		if err != nil {
			return err
		}

		if stop || lastPage {
			break
		}

		offset += pageSize
	}

	return nil
}

func (s *service) BatchUpdateOAuth2TokenEncryptedFields(ctx context.Context, updates []OAuth2TokenEncryptedFieldsUpdate) error {
	for _, u := range updates {
		_, err := s.sq.
			Update(OAuth2TokensTable).
			Set("encrypted_access_token", u.EncryptedAccessToken).
			Set("encrypted_refresh_token", u.EncryptedRefreshToken).
			Where(sq.Eq{"id": u.Id}).
			RunWith(s.db).
			Exec()
		if err != nil {
			return errors.Wrapf(err, "failed to update oauth2 token %s", u.Id)
		}
	}
	return nil
}

type OAuth2TokenWithConnection struct {
	Token      OAuth2Token
	Connection Connection
}

func (s *service) EnumerateOAuth2TokensExpiringWithin(
	ctx context.Context,
	duration time.Duration,
	callback func(tokens []*OAuth2TokenWithConnection, lastPage bool) (stop bool, err error),
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
			From(OAuth2TokensTable+" AS t").
			InnerJoin(ConnectionsTable+" AS c ON c.id = t.connection_id").
			Where(sq.Eq{
				"t.deleted_at": nil,
				"c.deleted_at": nil,
				"c.state":      ConnectionStateReady,
			}).
			Where("t.access_token_expires_at <= ?", expirationThreshold).
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

		stop, err := callback(results, lastPage)
		if err != nil {
			return err
		}

		if stop || lastPage {
			break
		}

		offset += pageSize
	}

	return nil
}

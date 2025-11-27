package database

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"gorm.io/gorm"
)

type OAuth2Token struct {
	ID                    uuid.UUID      `gorm:"column:id;primarykey"`
	ConnectionID          uuid.UUID      `gorm:"column:connection_id;not null"` // Foreign key to Connection
	RefreshedFromID       *uuid.UUID     `gorm:"column:refreshed_from_id"`
	EncryptedRefreshToken string         `gorm:"column:encrypted_refresh_token"`
	EncryptedAccessToken  string         `gorm:"column:encrypted_access_token"`
	AccessTokenExpiresAt  *time.Time     `gorm:"column:access_token_expires_at"`
	Scopes                string         `gorm:"column:scopes"`
	CreatedAt             time.Time      `gorm:"column:created_at"`
	DeletedAt             gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (*OAuth2Token) TableName() string {
	return "oauth2_tokens"
}

func (t *OAuth2Token) IsAccessTokenExpired(ctx context.Context) bool {
	if t.AccessTokenExpiresAt == nil {
		return false
	}

	return t.AccessTokenExpiresAt.Before(apctx.GetClock(ctx).Now())
}

func (s *service) GetOAuth2Token(
	ctx context.Context,
	connectionId uuid.UUID,
) (*OAuth2Token, error) {
	sess := s.session(ctx)

	var t OAuth2Token

	// Query the database for the newest token matching the specified connection ID
	result := sess.
		Where("connection_id = ?", connectionId).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		First(&t)

	if result.Error != nil {
		if errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}

	return &t, nil
}

func (s *service) DeleteOAuth2Token(
	ctx context.Context,
	tokenId uuid.UUID,
) error {
	sess := s.session(ctx)
	result := sess.Delete(&OAuth2Token{}, tokenId)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *service) DeleteAllOAuth2TokensForConnection(
	ctx context.Context,
	connectionId uuid.UUID,
) error {
	sqlDb, err := s.gorm.DB()
	if err != nil {
		return err
	}

	sqb := sq.StatementBuilder.RunWith(sqlDb)
	now := apctx.GetClock(ctx).Now()

	_, err = sqb.
		Update("oauth2_tokens").
		Set("deleted_at", now).
		Where("connection_id = ?", connectionId).
		ExecContext(ctx)

	if err != nil {
		return err
	}
	return nil
}

func (s *service) InsertOAuth2Token(
	ctx context.Context,
	connectionId uuid.UUID,
	refreshedFrom *uuid.UUID,
	encryptedRefreshToken string,
	encryptedAccessToken string,
	accessTokenExpiresAt *time.Time,
	scopes string,
) (*OAuth2Token, error) {
	sess := s.session(ctx)

	var newToken *OAuth2Token

	err := sess.Transaction(func(tx *gorm.DB) error {
		// Check if a token exists for refreshedFrom
		result := tx.
			Model(&OAuth2Token{}).
			Where("connection_id = ?", connectionId).
			Where("deleted_at IS NULL").
			Update("deleted_at", apctx.GetClock(ctx).Now())

		if result.Error != nil {
			return result.Error
		}

		// Create a new token
		newToken = &OAuth2Token{
			ID:                    apctx.GetUuidGenerator(ctx).New(),
			ConnectionID:          connectionId,
			RefreshedFromID:       refreshedFrom,
			EncryptedRefreshToken: encryptedRefreshToken,
			EncryptedAccessToken:  encryptedAccessToken,
			AccessTokenExpiresAt:  accessTokenExpiresAt,
			Scopes:                scopes,
			CreatedAt:             apctx.GetClock(ctx).Now(),
		}

		if err := tx.Create(newToken).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return newToken, nil
}

type OAuth2TokenWithConnection struct {
	OAuth2Token
	Connection Connection `gorm:"embedded"`
}

func (s *service) EnumerateOAuth2TokensExpiringWithin(
	ctx context.Context,
	duration time.Duration,
	callback func(tokens []*OAuth2TokenWithConnection, lastPage bool) (stop bool, err error),
) error {
	const pageSize = 100
	now := apctx.GetClock(ctx).Now()
	expirationThreshold := now.Add(duration)

	sess := s.session(ctx)
	offset := 0

	for {
		var tokensWithConnections []*OAuth2TokenWithConnection

		// Fetch tokens that are expiring within the given duration or already expired
		result := sess.
			Table("oauth2_tokens t").
			Select("t.*, c.*").
			Joins("INNER JOIN connections c ON c.id = t.connection_id").
			Where("t.access_token_expires_at <= ?", expirationThreshold).
			Where("t.deleted_at IS NULL").
			Where("c.deleted_at IS NULL").
			Where("c.state = ?", ConnectionStateReady).
			Order("t.created_at DESC").
			Limit(pageSize + 1).
			Offset(offset).
			Find(&tokensWithConnections)

		if result.Error != nil {
			return result.Error
		}

		lastPage := len(tokensWithConnections) < pageSize+1

		if len(tokensWithConnections) > pageSize {
			tokensWithConnections = tokensWithConnections[:pageSize]
		}

		stop, err := callback(tokensWithConnections, lastPage)
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

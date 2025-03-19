package database

import (
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/context"
	"gorm.io/gorm"
	"time"
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

func (t *OAuth2Token) IsAccessTokenExpired(ctx context.Context) bool {
	if t.AccessTokenExpiresAt == nil {
		return false
	}

	return t.AccessTokenExpiresAt.Before(ctx.Clock().Now())
}

func (db *gormDB) GetOAuth2Token(
	ctx context.Context,
	connectionId uuid.UUID,
) (*OAuth2Token, error) {
	sess := db.session(ctx)

	var t OAuth2Token

	// Query the database for the newest token matching the specified connection ID
	result := sess.
		Where("connection_id = ?", connectionId).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		First(&t)

	if result.Error != nil {
		if errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &t, nil
}

func (db *gormDB) InsertOAuth2Token(
	ctx context.Context,
	connectionId uuid.UUID,
	refreshedFrom *uuid.UUID,
	encryptedRefreshToken string,
	encryptedAccessToken string,
	accessTokenExpiresAt *time.Time,
	scopes string,
) (*OAuth2Token, error) {
	sess := db.session(ctx)

	var newToken *OAuth2Token

	err := sess.Transaction(func(tx *gorm.DB) error {
		// Check if a token exists for refreshedFrom
		result := tx.
			Model(&OAuth2Token{}).
			Where("connection_id = ?", connectionId).
			Where("deleted_at IS NULL").
			Update("deleted_at", ctx.Clock().Now())

		if result.Error != nil {
			return result.Error
		}

		// Create a new token
		newToken = &OAuth2Token{
			ID:                    ctx.UuidGenerator().New(),
			ConnectionID:          connectionId,
			RefreshedFromID:       refreshedFrom,
			EncryptedRefreshToken: encryptedRefreshToken,
			EncryptedAccessToken:  encryptedAccessToken,
			AccessTokenExpiresAt:  accessTokenExpiresAt,
			Scopes:                scopes,
			CreatedAt:             ctx.Clock().Now(),
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

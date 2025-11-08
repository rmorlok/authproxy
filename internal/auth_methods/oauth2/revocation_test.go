package oauth2

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	cfg "github.com/rmorlok/authproxy/internal/config/connectors"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	mockH "github.com/rmorlok/authproxy/internal/httpf/mock"
	"github.com/stretchr/testify/require"
	genmock "gopkg.in/h2non/gentleman-mock.v2"
)

func TestSupportsRevokeRefreshToken(t *testing.T) {
	o2 := oAuth2Connection{}
	require.False(t, o2.SupportsRevokeTokens())

	o2.auth = &cfg.AuthOAuth2{
		Type: cfg.AuthTypeOAuth2,
	}

	require.False(t, o2.SupportsRevokeTokens())

	o2.auth.Revocation = &cfg.AuthOauth2Revocation{}

	require.False(t, o2.SupportsRevokeTokens())

	o2.auth.Revocation.Endpoint = "https://example.com/revoke"

	require.True(t, o2.SupportsRevokeTokens())
}

func TestRevokeRefreshToken(t *testing.T) {
	connectionId := uuid.New()
	tokenId := uuid.New()

	setupWithMocks := func(t *testing.T) (*oAuth2Connection, *mockDb.MockDB, *mockEncrypt.MockE, *gomock.Controller) {
		ctrl := gomock.NewController(t)
		h := mockH.NewFactoryWithMockingClient(ctrl)
		db := mockDb.NewMockDB(ctrl)
		encrypt := mockEncrypt.NewMockE(ctrl)
		logger, _ := mockLog.NewTestLogger(t)

		return &oAuth2Connection{
			cfg:        nil,
			db:         db,
			httpf:      h,
			r:          nil,
			connectors: nil,
			encrypt:    encrypt,
			logger:     logger,
			connection: database.Connection{
				ID: connectionId,
			},
			auth: &cfg.AuthOAuth2{
				Type: cfg.AuthTypeOAuth2,
				Revocation: &cfg.AuthOauth2Revocation{
					Endpoint: "http://example.com/revoke",
				},
			},
		}, db, encrypt, ctrl
	}

	t.Run("it works with base settings", func(t *testing.T) {
		o2, db, encrypt, ctrl := setupWithMocks(t)
		defer ctrl.Finish()

		MockOAuthTokenForConnection(context.Background(), db, encrypt, database.OAuth2Token{
			ID:                    tokenId,
			ConnectionID:          connectionId,
			EncryptedAccessToken:  "some-access-token",
			EncryptedRefreshToken: "some-refresh-token",
		})

		db.
			EXPECT().
			DeleteOAuth2Token(gomock.Any(), tokenId).
			Return(nil)

		genmock.
			New("http://example.com").
			Post("/revoke").
			MatchType("application/x-www-form-urlencoded").
			BodyString("token=some-refresh-token&token_type_hint=refresh_token").
			Reply(200)

		genmock.
			New("http://example.com").
			Post("/revoke").
			MatchType("application/x-www-form-urlencoded").
			BodyString("token=some-access-token&token_type_hint=access_token").
			Reply(200)

		err := o2.RevokeTokens(context.Background())
		require.NoError(t, err)
	})
}

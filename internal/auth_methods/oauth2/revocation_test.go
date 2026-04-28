package oauth2

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	mockH "github.com/rmorlok/authproxy/internal/httpf/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/require"
	genmock "gopkg.in/h2non/gentleman-mock.v2"
	gock "gopkg.in/h2non/gock.v1"
)

// noAuthorizationHeader is a gock matcher that fails if the request includes
// an Authorization header. RFC 7009 token revocation does not use Bearer
// authorization on the token itself, and Google's revoke endpoint rejects
// requests that include one.
func noAuthorizationHeader(req *http.Request, _ *gock.Request) (bool, error) {
	if v := req.Header.Get("Authorization"); v != "" {
		return false, fmt.Errorf("revoke request must not include Authorization header, got %q", v)
	}
	return true, nil
}

func TestSupportsRevokeRefreshToken(t *testing.T) {
	o2 := oAuth2Connection{}
	require.False(t, o2.SupportsRevokeTokens())

	o2.auth = &cschema.AuthOAuth2{
		Type: cschema.AuthTypeOAuth2,
	}

	require.False(t, o2.SupportsRevokeTokens())

	o2.auth.Revocation = &cschema.AuthOauth2Revocation{}

	require.False(t, o2.SupportsRevokeTokens())

	o2.auth.Revocation.Endpoint = "https://example.com/revoke"

	require.True(t, o2.SupportsRevokeTokens())
}

func TestRevokeRefreshToken(t *testing.T) {
	connectionId := apid.New(apid.PrefixActor)
	tokenId := apid.New(apid.PrefixOAuth2Token)

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
			connection: &mockCore.Connection{
				Id: connectionId,
			},
			auth: &cschema.AuthOAuth2{
				Type: cschema.AuthTypeOAuth2,
				Revocation: &cschema.AuthOauth2Revocation{
					Endpoint: "http://example.com/revoke",
				},
			},
		}, db, encrypt, ctrl
	}

	t.Run("it works with base settings", func(t *testing.T) {
		o2, db, encrypt, ctrl := setupWithMocks(t)
		defer ctrl.Finish()

		MockOAuthTokenForConnection(context.Background(), db, encrypt, database.OAuth2Token{
			Id:                    tokenId,
			ConnectionId:          connectionId,
			EncryptedAccessToken:  encfield.EncryptedField{ID: "ekv_test", Data: "some-access-token"},
			EncryptedRefreshToken: encfield.EncryptedField{ID: "ekv_test", Data: "some-refresh-token"},
		})

		db.
			EXPECT().
			DeleteOAuth2Token(gomock.Any(), tokenId).
			Return(nil)

		genmock.
			New("http://example.com").
			Post("/revoke").
			MatchType("application/x-www-form-urlencoded").
			AddMatcher(noAuthorizationHeader).
			BodyString("token=some-refresh-token&token_type_hint=refresh_token").
			Reply(200)

		genmock.
			New("http://example.com").
			Post("/revoke").
			MatchType("application/x-www-form-urlencoded").
			AddMatcher(noAuthorizationHeader).
			BodyString("token=some-access-token&token_type_hint=access_token").
			Reply(200)

		err := o2.RevokeTokens(context.Background())
		require.NoError(t, err)
	})

	t.Run("only revokes refresh token when supported_tokens=refresh_token", func(t *testing.T) {
		o2, db, encrypt, ctrl := setupWithMocks(t)
		defer ctrl.Finish()

		supported := cschema.AuthOAuth2RevocationSupportedTypeRefreshToken
		o2.auth.Revocation.SupportedTokens = &supported

		MockOAuthTokenForConnection(context.Background(), db, encrypt, database.OAuth2Token{
			Id:                    tokenId,
			ConnectionId:          connectionId,
			EncryptedAccessToken:  encfield.EncryptedField{ID: "ekv_test", Data: "some-access-token"},
			EncryptedRefreshToken: encfield.EncryptedField{ID: "ekv_test", Data: "some-refresh-token"},
		})

		db.
			EXPECT().
			DeleteOAuth2Token(gomock.Any(), tokenId).
			Return(nil)

		genmock.
			New("http://example.com").
			Post("/revoke").
			MatchType("application/x-www-form-urlencoded").
			AddMatcher(noAuthorizationHeader).
			BodyString("token=some-refresh-token&token_type_hint=refresh_token").
			Reply(200)

		err := o2.RevokeTokens(context.Background())
		require.NoError(t, err)
	})
}

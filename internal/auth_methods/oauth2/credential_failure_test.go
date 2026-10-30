package oauth2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

// credentialFailureConnection isolates credential origins from token-endpoint
// I/O; unexpected database or decryption calls fail their mock expectations.
func credentialFailureConnection(t *testing.T) (*oAuth2Connection, *mockDb.MockDB, *mockEncrypt.MockE) {
	t.Helper()
	ctrl := gomock.NewController(t)
	db, encrypt := mockDb.NewMockDB(ctrl), mockEncrypt.NewMockE(ctrl)
	logger, _ := bufLogger(t)
	return &oAuth2Connection{
		db: db, encrypt: encrypt, logger: logger,
		auth:       &sconfig.AuthOAuth2{},
		connection: &mockCore.Connection{Id: apid.New(apid.PrefixConnection)},
	}, db, encrypt
}

// TestCredentialFailureTokenLookup marks missing rows wherever token loading
// occurs, without reclassifying dependency failures or changing HTTP errors.
func TestCredentialFailureTokenLookup(t *testing.T) {
	for _, origin := range []string{"resolve", "refresh", "locked refresh"} {
		for _, test := range []struct {
			name        string
			cause       error
			unavailable bool
		}{
			{"missing", database.ErrNotFound, true},
			{"wrapped missing", fmt.Errorf("token row absent: %w", database.ErrNotFound), true},
			{"database failure", errors.New("database unavailable"), false},
			{"cancellation", context.Canceled, false},
		} {
			t.Run(origin+"/"+test.name, func(t *testing.T) {
				// All three origins fail before decryption or token-endpoint I/O.
				o, db, _ := credentialFailureConnection(t)
				db.EXPECT().GetOAuth2Token(gomock.Any(), o.connection.GetId()).Return(nil, test.cause)
				var err error
				switch origin {
				case "resolve":
					_, err = o.Resolve(context.Background())
				case "refresh":
					err = o.Refresh(context.Background())
				case "locked refresh":
					_, err = o.refreshAccessTokenLocked(context.Background(), nil, refreshModeAlways)
				}

				// Internal classification preserves both the original cause and
				// the legacy direct-sentinel versus wrapped-error HTTP behavior.
				require.ErrorIs(t, err, test.cause)
				require.Equal(t, test.unavailable, errors.Is(err, iface.ErrCredentialsUnavailable))
				require.Equal(t, test.cause.Error(), err.Error())
				public := httperr.FromError(err)
				if origin == "resolve" && test.cause == database.ErrNotFound {
					require.Equal(t, http.StatusUnprocessableEntity, public.Status)
					require.Equal(t, "no valid oauth token found", public.ResponseMsgOrDefault())
				} else {
					require.Equal(t, http.StatusInternalServerError, public.Status)
					require.Equal(t, "Internal Server Error", public.ResponseMsgOrDefault())
				}
			})
		}
	}
}

// TestCredentialFailureRefreshClassification separates unusable credentials
// from categories that merely make a connection unhealthy. Recovery callers
// still decide whether to return this error or preserve an upstream 401.
func TestCredentialFailureRefreshClassification(t *testing.T) {
	for _, test := range []struct {
		category    tokenRefreshCategory
		unavailable bool
	}{
		{tokenRefreshNoRefreshToken, true},
		{tokenRefreshInvalidGrant, true},
		{tokenRefreshInvalidGrantAfterAmbiguousAttempt, true},
		{tokenRefreshInvalidClient, false},
		{tokenRefreshProvider4xxOther, false},
		{tokenRefreshMalformedResponse, false},
		{tokenRefreshProvider5xx, false},
		{tokenRefreshNetworkError, false},
		{tokenRefreshInternalError, false},
	} {
		t.Run(string(test.category), func(t *testing.T) {
			// Classification adds no provider text and preserves existing causes.
			o, _, _ := credentialFailureConnection(t)
			cause := errors.New("refresh failed with status 400")
			err := o.classifyAndRecordRefreshFailure(context.Background(), test.category, 400, "", 1, cause)
			require.ErrorIs(t, err, cause)
			require.Equal(t, cause.Error(), err.Error())
			require.Equal(t, test.unavailable, errors.Is(err, iface.ErrCredentialsUnavailable))
		})
	}
}

// TestCredentialFailureResolveDecryption does not mistake missing encryption
// keys or other decrypt failures for a missing OAuth credential.
func TestCredentialFailureResolveDecryption(t *testing.T) {
	for _, cause := range []error{database.ErrNotFound, context.Canceled, errors.New("decrypt failed")} {
		t.Run(cause.Error(), func(t *testing.T) {
			// A present token reaches decryption; even a not-found cause here
			// refers to encryption infrastructure rather than the token row.
			o, db, encrypt := credentialFailureConnection(t)
			token := &database.OAuth2Token{EncryptedAccessToken: encfield.EncryptedField{Data: "ciphertext"}}
			db.EXPECT().GetOAuth2Token(gomock.Any(), o.connection.GetId()).Return(token, nil)
			encrypt.EXPECT().DecryptString(gomock.Any(), token.EncryptedAccessToken).Return("", cause)
			_, err := o.Resolve(context.Background())
			require.ErrorIs(t, err, cause)
			require.NotErrorIs(t, err, iface.ErrCredentialsUnavailable)
		})
	}
}

// TestCredentialFailureRefreshBeforeRequest distinguishes absent refresh tokens
// from connector configuration failures before making any token-endpoint call.
func TestCredentialFailureRefreshBeforeRequest(t *testing.T) {
	for _, missingRefresh := range []bool{true, false} {
		t.Run(fmt.Sprintf("missing refresh token %t", missingRefresh), func(t *testing.T) {
			o, db, _ := credentialFailureConnection(t)
			token := &database.OAuth2Token{}
			if !missingRefresh {
				// A stored refresh token gets as far as an invalid client-id
				// configuration, which cannot justify reconnect advice.
				token.EncryptedRefreshToken = encfield.EncryptedField{Data: "ciphertext"}
				o.auth.ClientId = &common.StringValue{}
			}
			db.EXPECT().GetOAuth2Token(gomock.Any(), o.connection.GetId()).Return(token, nil)
			_, err := o.refreshAccessTokenLocked(context.Background(), nil, refreshModeAlways)
			require.Error(t, err)
			require.Equal(t, missingRefresh, errors.Is(err, iface.ErrCredentialsUnavailable))
			if missingRefresh {
				require.ErrorIs(t, err, errNoRefreshToken)
				require.Equal(t, errNoRefreshToken.Error(), err.Error())
			} else {
				require.ErrorContains(t, err, "failed to get client id for connector")
			}
		})
	}
}

// TestCredentialFailureStoredClientCredentials only marks missing persisted
// client grants; unavailable stores and damaged payloads remain internal errors.
func TestCredentialFailureStoredClientCredentials(t *testing.T) {
	for _, test := range []struct {
		name, plaintext string
		loadErr, decErr error
		unavailable     bool
	}{
		{name: "missing", loadErr: database.ErrNotFound, unavailable: true},
		{name: "wrapped missing", loadErr: fmt.Errorf("no active grant: %w", database.ErrNotFound), unavailable: true},
		{name: "database failure", loadErr: errors.New("database unavailable")},
		{name: "cancellation", loadErr: context.Canceled},
		{name: "decryption key absent", decErr: database.ErrNotFound},
		{name: "malformed payload", plaintext: "not json"},
		{name: "missing client id", plaintext: `{"clientSecret":"secret"}`},
		{name: "missing client secret", plaintext: `{"clientId":"id"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Grant type selects connection-owned client credentials instead
			// of connector configuration; all work stops before endpoint I/O.
			o, db, encrypt := credentialFailureConnection(t)
			grant := sconfig.OAuth2GrantClientCredentials
			o.auth.GrantType = &grant
			credential := &database.ApiKeyCredential{EncryptedCredentials: encfield.EncryptedField{Data: "ciphertext"}}
			db.EXPECT().GetActiveApiKeyCredential(gomock.Any(), o.connection.GetId()).Return(credential, test.loadErr)
			if test.loadErr == nil {
				encrypt.EXPECT().DecryptString(gomock.Any(), credential.EncryptedCredentials).Return(test.plaintext, test.decErr)
			}
			_, _, err := o.resolveClientCredentials(context.Background())
			require.Error(t, err)
			require.Equal(t, test.unavailable, errors.Is(err, iface.ErrCredentialsUnavailable))
			if test.loadErr != nil {
				require.ErrorIs(t, err, test.loadErr)
				require.Equal(t, "failed to load OAuth2 client credentials: "+test.loadErr.Error(), err.Error())
			} else if test.decErr != nil {
				require.ErrorIs(t, err, test.decErr)
			}
		})
	}
}

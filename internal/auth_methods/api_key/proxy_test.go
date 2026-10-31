package api_key

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	"github.com/rmorlok/authproxy/internal/httperr"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestApiKeyConn builds an apiKeyConnection with mock DB + fake encrypt
// for resolveAuth tests. The fake encrypt service stores plaintext in Data
// (no actual encryption), which lets resolveAuth's json.Unmarshal succeed.
func newTestApiKeyConn(
	t *testing.T, ctrl *gomock.Controller,
	conn *mockCore.Connection,
) (*apiKeyConnection, *mockDb.MockDB) {
	t.Helper()
	db := mockDb.NewMockDB(ctrl)
	return &apiKeyConnection{
		db:         db,
		encrypt:    encrypt.NewFakeEncryptService(false),
		logger:     aplog.NewNoopLogger(),
		connection: conn,
	}, db
}

func TestResolveAuth_UsesPlacementSnapshotWhenPresent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	connectionId := apid.New(apid.PrefixConnection)
	conn, db := newTestApiKeyConn(t, ctrl, &mockCore.Connection{Id: connectionId})

	// The credential was inserted when the connector's placement was "header
	// with X-API-Key"; the snapshot stored on the row should be used even if
	// the live connector definition has since been updated.
	db.EXPECT().GetActiveApiKeyCredential(gomock.Any(), connectionId).Return(
		&database.ApiKeyCredential{
			Id:                   apid.New(apid.PrefixApiKeyCredential),
			ConnectionId:         connectionId,
			EncryptedCredentials: encfield.EncryptedField{ID: "dek_fake", Data: `{"apiKey":"sk-snapshot"}`},
			PlacementSnapshot: &cschema.ApiKeyPlacement{
				Type:       cschema.ApiKeyPlacementHeader,
				HeaderName: "X-API-Key",
			},
		}, nil,
	)

	app, err := conn.resolveAuth(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "X-API-Key", app.HeaderName)
	assert.Equal(t, "sk-snapshot", app.HeaderValue)
}

func TestResolveAuth_FailsWhenPlacementSnapshotMissing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	connectionId := apid.New(apid.PrefixConnection)
	conn, db := newTestApiKeyConn(t, ctrl, &mockCore.Connection{Id: connectionId})

	// Every credential row inserted by the connection-initiate path carries a
	// placement snapshot. A row without one indicates corruption, so the proxy
	// errors rather than guessing.
	db.EXPECT().GetActiveApiKeyCredential(gomock.Any(), connectionId).Return(
		&database.ApiKeyCredential{
			Id:                   apid.New(apid.PrefixApiKeyCredential),
			ConnectionId:         connectionId,
			EncryptedCredentials: encfield.EncryptedField{ID: "dek_fake", Data: `{"apiKey":"sk-live"}`},
			// PlacementSnapshot: nil
		}, nil,
	)

	_, err := conn.resolveAuth(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "placement snapshot")
	assert.NotErrorIs(t, err, auth_methods.ErrCredentialsUnavailable)
}

// TestResolveAuth_ClassifiesCredentialLookupErrors recognizes a missing row
// without treating unrelated failures or HTTP status codes as credential loss.
func TestResolveAuth_ClassifiesCredentialLookupErrors(t *testing.T) {
	for _, test := range []struct {
		name        string
		cause       error
		unavailable bool
	}{
		{"missing row", database.ErrNotFound, true},
		{"wrapped missing row", fmt.Errorf("no active credential: %w", database.ErrNotFound), true},
		{"HTTP-wrapped missing row", httperr.NotFound("credential absent", httperr.WithInternalErr(database.ErrNotFound)), true},
		{"database failure", errors.New("db boom"), false},
		{"HTTP not found without missing row", httperr.NotFound("database service absent"), false},
		{"canceled lookup", context.Canceled, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Fail the active-credential query before any decryption or injection.
			ctrl := gomock.NewController(t)
			connectionId := apid.New(apid.PrefixConnection)
			conn, db := newTestApiKeyConn(t, ctrl, &mockCore.Connection{Id: connectionId})
			db.EXPECT().GetActiveApiKeyCredential(gomock.Any(), connectionId).Return(nil, test.cause)
			_, err := conn.Resolve(context.Background())

			// The authenticator adds only the marker; callers retain the cause,
			// diagnostic text, and public HTTP response they received before.
			wantErr := fmt.Errorf("failed to load api-key credential: %w", test.cause)
			require.EqualError(t, err, wantErr.Error())
			require.ErrorIs(t, err, test.cause)
			require.Equal(t, test.unavailable, errors.Is(err, auth_methods.ErrCredentialsUnavailable))
			wantHTTP, gotHTTP := httperr.FromError(wantErr), httperr.FromError(err)
			require.Equal(t, wantHTTP.Status, gotHTTP.Status)
			require.Equal(t, wantHTTP.ResponseMsgOrDefault(), gotHTTP.ResponseMsgOrDefault())
		})
	}
}

func TestResolveAuth_FailsOnMalformedPlaintext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	connectionId := apid.New(apid.PrefixConnection)
	conn, db := newTestApiKeyConn(t, ctrl, &mockCore.Connection{Id: connectionId})

	// Decrypt returns the raw Data — if a row's encrypted bytes ever fail to
	// decrypt-to-JSON, resolveAuth surfaces the parse error rather than
	// proxying with junk.
	db.EXPECT().GetActiveApiKeyCredential(gomock.Any(), connectionId).Return(
		&database.ApiKeyCredential{
			ConnectionId:         connectionId,
			EncryptedCredentials: encfield.EncryptedField{ID: "dek_fake", Data: "not-json"},
			PlacementSnapshot:    &cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer},
		}, nil,
	)

	_, err := conn.resolveAuth(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
	assert.NotErrorIs(t, err, auth_methods.ErrCredentialsUnavailable)
}

// TestResolveAuth_DecryptNotFoundIsNotCredentialLoss keeps missing encryption
// infrastructure distinct from an absent active credential row.
func TestResolveAuth_DecryptNotFoundIsNotCredentialLoss(t *testing.T) {
	// The active credential exists, but its referenced encryption key does not.
	ctrl := gomock.NewController(t)
	connectionId := apid.New(apid.PrefixConnection)
	conn, db := newTestApiKeyConn(t, ctrl, &mockCore.Connection{Id: connectionId})
	cred := &database.ApiKeyCredential{
		ConnectionId: connectionId,
		EncryptedCredentials: encfield.EncryptedField{
			ID:   "dek_missing",
			Data: "private-ciphertext",
		},
		PlacementSnapshot: &cschema.ApiKeyPlacement{
			Type: cschema.ApiKeyPlacementBearer,
		},
	}
	db.EXPECT().
		GetActiveApiKeyCredential(gomock.Any(), connectionId).
		Return(cred, nil)
	encryption := mockEncrypt.NewMockE(ctrl)
	encryption.EXPECT().
		DecryptString(gomock.Any(), cred.EncryptedCredentials).
		Return("", database.ErrNotFound)
	conn.encrypt = encryption
	_, err := conn.Resolve(context.Background())

	// A missing decryption dependency preserves its cause without advising
	// credential replacement, which would not fix the underlying problem.
	require.ErrorContains(t, err, "failed to decrypt")
	require.ErrorIs(t, err, database.ErrNotFound)
	require.NotErrorIs(t, err, auth_methods.ErrCredentialsUnavailable)
}

// TestResolveAuth_InvalidStoredCredentialIsNotMissing leaves corrupt plaintext
// and placement configuration unclassified instead of inferring credential loss.
func TestResolveAuth_InvalidStoredCredentialIsNotMissing(t *testing.T) {
	for _, test := range []struct {
		name      string
		plaintext string
		placement cschema.ApiKeyPlacement
	}{
		{"empty key", `{}`, cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer}},
		{"missing basic username", `{"apiKey":"key"}`, cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBasic}},
		{"missing header name", `{"apiKey":"key"}`, cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementHeader}},
		{"missing query name", `{"apiKey":"key"}`, cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementQuery}},
		{"unknown placement", `{"apiKey":"key"}`, cschema.ApiKeyPlacement{Type: "unknown"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Return an existing row whose stored fields violate setup invariants.
			ctrl := gomock.NewController(t)
			connectionId := apid.New(apid.PrefixConnection)
			conn, db := newTestApiKeyConn(t, ctrl, &mockCore.Connection{Id: connectionId})
			db.EXPECT().GetActiveApiKeyCredential(gomock.Any(), connectionId).Return(&database.ApiKeyCredential{
				ConnectionId:         connectionId,
				EncryptedCredentials: encfield.EncryptedField{ID: "dek_fake", Data: test.plaintext},
				PlacementSnapshot:    &test.placement,
			}, nil)
			_, err := conn.Resolve(context.Background())

			// Invalid existing data is not evidence that the credential is absent.
			require.Error(t, err)
			require.NotErrorIs(t, err, auth_methods.ErrCredentialsUnavailable)
		})
	}
}

func TestResolveAuth_BasicPlacementResolvesUsername(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	connectionId := apid.New(apid.PrefixConnection)
	conn, db := newTestApiKeyConn(t, ctrl, &mockCore.Connection{Id: connectionId})

	db.EXPECT().GetActiveApiKeyCredential(gomock.Any(), connectionId).Return(
		&database.ApiKeyCredential{
			ConnectionId: connectionId,
			EncryptedCredentials: encfield.EncryptedField{
				ID:   "dek_fake",
				Data: `{"apiKey":"pw","username":"u"}`,
			},
			PlacementSnapshot: &cschema.ApiKeyPlacement{
				Type:          cschema.ApiKeyPlacementBasic,
				UsernameField: "account_id",
			},
		}, nil,
	)

	app, err := conn.resolveAuth(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Authorization", app.HeaderName)
	// "u:pw" base64-encoded — RFC 7617.
	assert.Equal(t, "Basic dTpwdw==", app.HeaderValue)
}

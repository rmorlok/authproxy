package api_key

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestApiKeyConn builds an apiKeyConnection with mock DB + fake encrypt
// for resolveAuth tests. The fake encrypt service stores plaintext in Data
// (no actual encryption), which lets resolveAuth's json.Unmarshal succeed.
func newTestApiKeyConn(t *testing.T, ctrl *gomock.Controller, conn *mockCore.Connection) (*apiKeyConnection, *mockDb.MockDB) {
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
			EncryptedCredentials: encfield.EncryptedField{ID: "dek_fake", Data: `{"api_key":"sk-snapshot"}`},
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
			EncryptedCredentials: encfield.EncryptedField{ID: "dek_fake", Data: `{"api_key":"sk-live"}`},
			// PlacementSnapshot: nil
		}, nil,
	)

	_, err := conn.resolveAuth(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "placement snapshot")
}

func TestResolveAuth_PropagatesDBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	connectionId := apid.New(apid.PrefixConnection)
	conn, db := newTestApiKeyConn(t, ctrl, &mockCore.Connection{Id: connectionId})

	db.EXPECT().GetActiveApiKeyCredential(gomock.Any(), connectionId).
		Return(nil, errors.New("db boom"))

	_, err := conn.resolveAuth(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db boom")
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
				Data: `{"api_key":"pw","username":"u"}`,
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

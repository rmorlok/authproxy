package api_key

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// newTestFactory builds an api_key.factory with a mock DB + fake encrypt
// service for PersistCredentials tests. The fake encrypt stores plaintext
// directly in EncryptedField.Data so tests can introspect the persisted
// credential blob.
func newTestFactory(t *testing.T, ctrl *gomock.Controller) (*factory, *mockDb.MockDB) {
	t.Helper()
	db := mockDb.NewMockDB(ctrl)
	return &factory{
		db:      db,
		encrypt: encrypt.NewFakeEncryptService(false),
		logger:  aplog.NewNoopLogger(),
	}, db
}

// ctxWithActor returns a context with an authenticated actor — required
// because PersistCredentials records the actor as the credential's creator.
func ctxWithActor(t *testing.T) (context.Context, apid.ID) {
	t.Helper()
	actorId := apid.MustParse("act_test1111111111aa")
	ra := apauthcore.NewAuthenticatedRequestAuth(&apauthcore.Actor{
		Id:        actorId,
		Namespace: "root",
	})
	return ra.ContextWith(context.Background()), actorId
}

// captureEncField captures the encfield.EncryptedField argument so tests can
// assert on the persisted (fake-encrypted, i.e. plaintext) JSON blob.
type captureEncField struct{ field encfield.EncryptedField }

func (c *captureEncField) Matches(x any) bool {
	v, ok := x.(encfield.EncryptedField)
	if !ok {
		return false
	}
	c.field = v
	return true
}
func (c *captureEncField) String() string { return "captured encfield.EncryptedField" }

// TestPersistCredentials_Bearer — the canonical happy path. Insert receives
// the placement reference and the actor id; the blob round-trips as a JSON
// payload with just the api_key field (no username).
func TestPersistCredentials_Bearer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	f, db := newTestFactory(t, ctrl)
	conn := &mockCore.Connection{
		Id:        apid.New(apid.PrefixConnection),
		Namespace: "root",
	}
	placement := &cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer}

	cap := &captureEncField{}
	ctx, actorId := ctxWithActor(t)
	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, cap, placement, &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)

	require.NoError(t, f.PersistCredentials(ctx, conn, placement, map[string]any{
		"api_key": "sk-bearer-xyz",
	}))

	var plaintext database.ApiKeyCredentialPlaintext
	require.NoError(t, json.Unmarshal([]byte(cap.field.Data), &plaintext))
	assert.Equal(t, "sk-bearer-xyz", plaintext.ApiKey)
	assert.Empty(t, plaintext.Username, "bearer placement must not include username")
}

// TestPersistCredentials_Header — header placement is treated like bearer
// for persistence (api_key only).
func TestPersistCredentials_Header(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	f, db := newTestFactory(t, ctrl)
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root"}
	placement := &cschema.ApiKeyPlacement{
		Type:       cschema.ApiKeyPlacementHeader,
		HeaderName: "X-API-Key",
	}

	cap := &captureEncField{}
	ctx, actorId := ctxWithActor(t)
	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, cap, placement, &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)

	require.NoError(t, f.PersistCredentials(ctx, conn, placement, map[string]any{
		"api_key": "key-1",
	}))

	var plaintext database.ApiKeyCredentialPlaintext
	require.NoError(t, json.Unmarshal([]byte(cap.field.Data), &plaintext))
	assert.Equal(t, "key-1", plaintext.ApiKey)
}

// TestPersistCredentials_Query — query placement is treated like bearer for
// persistence too. The placement type only affects how the credential is
// applied at request time, not how it's stored.
func TestPersistCredentials_Query(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	f, db := newTestFactory(t, ctrl)
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root"}
	placement := &cschema.ApiKeyPlacement{
		Type:      cschema.ApiKeyPlacementQuery,
		ParamName: "api_key",
	}

	ctx, actorId := ctxWithActor(t)
	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, gomock.Any(), placement, &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)

	require.NoError(t, f.PersistCredentials(ctx, conn, placement, map[string]any{
		"api_key": "q-1",
	}))
}

// TestPersistCredentials_Basic — basic placement requires both api_key and
// the configured username field. The username lands in the blob as Username.
func TestPersistCredentials_Basic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	f, db := newTestFactory(t, ctrl)
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root"}
	placement := &cschema.ApiKeyPlacement{
		Type:          cschema.ApiKeyPlacementBasic,
		UsernameField: "account_id",
	}

	cap := &captureEncField{}
	ctx, actorId := ctxWithActor(t)
	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, cap, placement, &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)

	require.NoError(t, f.PersistCredentials(ctx, conn, placement, map[string]any{
		"api_key":    "secret-token",
		"account_id": "alice@example.com",
	}))

	var plaintext database.ApiKeyCredentialPlaintext
	require.NoError(t, json.Unmarshal([]byte(cap.field.Data), &plaintext))
	assert.Equal(t, "secret-token", plaintext.ApiKey)
	assert.Equal(t, "alice@example.com", plaintext.Username)
}

// TestPersistCredentials_RejectsMissingApiKey — every placement requires the
// api_key field. A missing or empty value surfaces as a user-visible
// BadRequest before the DB is touched.
func TestPersistCredentials_RejectsMissingApiKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	f, _ := newTestFactory(t, ctrl)
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root"}
	placement := &cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer}

	ctx, _ := ctxWithActor(t)
	err := f.PersistCredentials(ctx, conn, placement, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_key is required")
}

// TestPersistCredentials_RejectsMissingUsernameForBasic — basic placement
// without the configured username field is rejected before persistence.
func TestPersistCredentials_RejectsMissingUsernameForBasic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	f, _ := newTestFactory(t, ctrl)
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root"}
	placement := &cschema.ApiKeyPlacement{
		Type:          cschema.ApiKeyPlacementBasic,
		UsernameField: "account_id",
	}

	ctx, _ := ctxWithActor(t)
	err := f.PersistCredentials(ctx, conn, placement, map[string]any{
		"api_key": "secret",
		// account_id intentionally omitted
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"account_id" is required`)
}

// TestPersistCredentials_RejectsNilPlacement — programmer error. Surfaces
// as an internal error rather than a panic.
func TestPersistCredentials_RejectsNilPlacement(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	f, _ := newTestFactory(t, ctrl)
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root"}

	ctx, _ := ctxWithActor(t)
	err := f.PersistCredentials(ctx, conn, nil, map[string]any{"api_key": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "placement")
}

// TestPersistCredentials_PropagatesDBError — DB failures surface as the
// wrapped internal error so the HTTP layer renders them as 500s with the
// original error preserved in the stack trace.
func TestPersistCredentials_PropagatesDBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	f, db := newTestFactory(t, ctrl)
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root"}
	placement := &cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer}

	sentinel := errors.New("connection refused")
	ctx, _ := ctxWithActor(t)
	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, gomock.Any(), placement, gomock.Any()).
		Return(nil, sentinel)

	err := f.PersistCredentials(ctx, conn, placement, map[string]any{"api_key": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to persist api-key credentials")
}

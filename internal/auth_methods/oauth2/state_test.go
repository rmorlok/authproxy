package oauth2

import (
	"context"
	"encoding/base64"
	"log/slog"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stateTestActor is a minimal IActorData stub for state tests.
type stateTestActor struct {
	id        apid.ID
	namespace string
}

func (a stateTestActor) GetId() apid.ID                      { return a.id }
func (a stateTestActor) GetExternalId() string               { return "" }
func (a stateTestActor) GetLabels() map[string]string        { return nil }
func (a stateTestActor) GetPermissions() []aschema.Permission { return nil }
func (a stateTestActor) GetNamespace() string                { return a.namespace }

// stateTestCore is a minimal coreIface.C that only implements GetConnection.
// Other method calls will nil-panic, making accidental coupling obvious.
type stateTestCore struct {
	coreIface.C
	conn coreIface.Connection
	err  error
}

func (c *stateTestCore) GetConnection(_ context.Context, _ apid.ID) (coreIface.Connection, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.conn, nil
}

func newStateTestEncrypt(t *testing.T) encrypt.E {
	t.Helper()
	cfg := config.FromRoot(&sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			GlobalAESKey: sconfig.NewKeyDataRandomBytes(),
		},
	})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	_, e := encrypt.NewTestEncryptService(cfg, db)
	return e
}

func TestState_RoundTripPreservesNamespace(t *testing.T) {
	ctx := context.Background()
	_, r := apredis.MustApplyTestConfig(nil)
	e := encrypt.NewFakeEncryptService(false)

	original := &state{
		Id:           apid.New(apid.PrefixOauth2State),
		Namespace:    "root.tenant-a",
		ActorId:      apid.New(apid.PrefixActor),
		ConnectorId:  apid.New(apid.PrefixConnectorVersion),
		ConnectionId: apid.New(apid.PrefixConnection),
		ExpiresAt:    time.Now().Add(time.Minute).UTC(),
		ReturnToUrl:  "https://example.com/return",
	}
	require.NoError(t, writeStateToRedis(ctx, r, e, original, time.Minute))

	loaded, err := readStateFromRedis(ctx, r, e, original.Id)
	require.NoError(t, err)
	assert.Equal(t, original.Namespace, loaded.Namespace)
	assert.Equal(t, original.ActorId, loaded.ActorId)
	assert.Equal(t, original.ConnectionId, loaded.ConnectionId)
}

func TestState_RedisValueIsCiphertext(t *testing.T) {
	ctx := context.Background()
	_, r := apredis.MustApplyTestConfig(nil)
	e := newStateTestEncrypt(t)

	s := &state{
		Id:           apid.New(apid.PrefixOauth2State),
		Namespace:    "root.tenant-a",
		ActorId:      apid.New(apid.PrefixActor),
		ConnectorId:  apid.New(apid.PrefixConnectorVersion),
		ConnectionId: apid.New(apid.PrefixConnection),
		ExpiresAt:    time.Now().Add(time.Minute).UTC(),
	}
	require.NoError(t, writeStateToRedis(ctx, r, e, s, time.Minute))

	raw, err := r.Get(ctx, getStateRedisKey(s.Id)).Result()
	require.NoError(t, err)
	// Sanity: the raw blob must NOT contain the namespace string in plaintext.
	assert.NotContains(t, raw, s.Namespace, "namespace must not appear in cleartext in redis")
	assert.NotContains(t, raw, s.ActorId.String(), "actor id must not appear in cleartext in redis")
}

func TestState_TamperedCiphertextFailsToDecrypt(t *testing.T) {
	ctx := context.Background()
	_, r := apredis.MustApplyTestConfig(nil)
	e := newStateTestEncrypt(t)

	s := &state{
		Id:           apid.New(apid.PrefixOauth2State),
		Namespace:    "root.tenant-a",
		ActorId:      apid.New(apid.PrefixActor),
		ConnectorId:  apid.New(apid.PrefixConnectorVersion),
		ConnectionId: apid.New(apid.PrefixConnection),
		ExpiresAt:    time.Now().Add(time.Minute).UTC(),
	}
	require.NoError(t, writeStateToRedis(ctx, r, e, s, time.Minute))

	// Read the stored envelope, flip a byte in the ciphertext body, and write it back.
	// AES-GCM is AEAD, so decryption must fail with an authentication error.
	raw, err := r.Get(ctx, getStateRedisKey(s.Id)).Result()
	require.NoError(t, err)

	ef, err := encfield.ParseInlineString(raw)
	require.NoError(t, err)

	ciphertext, err := base64.StdEncoding.DecodeString(ef.Data)
	require.NoError(t, err)
	require.NotEmpty(t, ciphertext)
	ciphertext[len(ciphertext)/2] ^= 0xff
	tampered := encfield.EncryptedField{ID: ef.ID, Data: base64.StdEncoding.EncodeToString(ciphertext)}
	require.NoError(t, r.Set(ctx, getStateRedisKey(s.Id), tampered.ToInlineString(), time.Minute).Err())

	_, err = readStateFromRedis(ctx, r, e, s.Id)
	require.Error(t, err, "tampered ciphertext must not decrypt")
	assert.Contains(t, err.Error(), "decrypt")
}

func TestGetOAuth2State_RejectsNamespaceMismatchOnActor(t *testing.T) {
	ctx := context.Background()
	cfg, r := apredis.MustApplyTestConfig(nil)
	e := encrypt.NewFakeEncryptService(false)
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	stateId := apid.New(apid.PrefixOauth2State)
	actorId := apid.New(apid.PrefixActor)
	s := &state{
		Id:           stateId,
		Namespace:    "root.tenant-a",
		ActorId:      actorId,
		ConnectorId:  apid.New(apid.PrefixConnectorVersion),
		ConnectionId: apid.New(apid.PrefixConnection),
		ExpiresAt:    time.Now().Add(time.Minute).UTC(),
	}
	require.NoError(t, writeStateToRedis(ctx, r, e, s, time.Minute))

	// The state was created against tenant-a but the inbound actor claims tenant-b.
	// Even with a matching actor id, the namespace check must reject.
	actor := stateTestActor{id: actorId, namespace: "root.tenant-b"}
	core := &stateTestCore{conn: &mockCore.Connection{Namespace: "root.tenant-b"}}

	_, err := getOAuth2State(ctx, cfg, nil, r, core, nil, e, logger, actor, stateId)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "actor namespace")
}

func TestGetOAuth2State_RejectsNamespaceMismatchOnConnection(t *testing.T) {
	ctx := context.Background()
	cfg, r := apredis.MustApplyTestConfig(nil)
	e := encrypt.NewFakeEncryptService(false)
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	stateId := apid.New(apid.PrefixOauth2State)
	actorId := apid.New(apid.PrefixActor)
	s := &state{
		Id:           stateId,
		Namespace:    "root.tenant-a",
		ActorId:      actorId,
		ConnectorId:  apid.New(apid.PrefixConnectorVersion),
		ConnectionId: apid.New(apid.PrefixConnection),
		ExpiresAt:    time.Now().Add(time.Minute).UTC(),
	}
	require.NoError(t, writeStateToRedis(ctx, r, e, s, time.Minute))

	// Actor namespace matches the state, but the connection on file is in a different
	// namespace — likely because the state was crafted against another tenant's
	// connection id. Reject before we reach the auth-method dispatch.
	actor := stateTestActor{id: actorId, namespace: "root.tenant-a"}
	core := &stateTestCore{conn: &mockCore.Connection{Namespace: "root.tenant-b"}}

	_, err := getOAuth2State(ctx, cfg, nil, r, core, nil, e, logger, actor, stateId)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection namespace")
}

// testWriter routes slog output through t.Log so it shows up under -v.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}

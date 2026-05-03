package encrypt

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

type reencryptTestEnv struct {
	ctx         context.Context
	cfg         config.C
	db          database.DB
	rawDb       *sql.DB
	enc         E
	logger      *slog.Logger
	globalEKVID apid.ID
	globalKeyV1 []byte
}

func setupReencryptTest(t *testing.T) reencryptTestEnv {
	t.Helper()

	sconfig.ResetKeyDataMockRegistry()
	t.Cleanup(sconfig.ResetKeyDataMockRegistry)

	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()
	logger := slog.Default()

	globalKeyBytes := util.MustGenerateSecureRandomKey(32)
	globalKD := sconfig.NewKeyDataMock("global")
	sconfig.KeyDataMockAddVersion("global", "global-key", "v1", globalKeyBytes)

	cfg := config.FromRoot(&sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			GlobalAESKey: globalKD,
		},
	})

	cfg, db, rawDb := database.MustApplyBlankTestDbConfigRaw(t, cfg)
	cfg, enc := NewTestEncryptService(cfg, db)

	globalVersions, err := db.ListEncryptionKeyVersionsForEncryptionKey(ctx, globalEncryptionKeyID)
	require.NoError(t, err)
	require.Len(t, globalVersions, 1)

	return reencryptTestEnv{
		ctx:         ctx,
		cfg:         cfg,
		db:          db,
		rawDb:       rawDb,
		enc:         enc,
		logger:      logger,
		globalEKVID: globalVersions[0].Id,
		globalKeyV1: globalKeyBytes,
	}
}

// addGlobalV2 adds a second version to the global key, syncs, and returns the new EKV ID.
func (env *reencryptTestEnv) addGlobalV2(t *testing.T) (apid.ID, []byte) {
	t.Helper()

	v2Bytes := util.MustGenerateSecureRandomKey(32)
	sconfig.KeyDataMockAddVersion("global", "global-key", "v2", v2Bytes)
	syncKeysVersionsToDatabase(env.ctx, env.cfg, env.db, env.logger, nil)
	require.NoError(t, env.enc.SyncKeysFromDbToMemory(env.ctx))

	versions, err := env.db.ListEncryptionKeyVersionsForEncryptionKey(env.ctx, globalEncryptionKeyID)
	require.NoError(t, err)
	for _, v := range versions {
		if v.ProviderVersion == "v2" {
			return v.Id, v2Bytes
		}
	}
	t.Fatal("v2 not found after sync")
	return "", nil
}

// setNamespaceTarget sets the target_encryption_key_version_id for a namespace.
func (env *reencryptTestEnv) setNamespaceTarget(t *testing.T, namespacePath string, ekvId apid.ID) {
	t.Helper()
	_, err := env.rawDb.Exec(
		fmt.Sprintf(`UPDATE namespaces SET target_encryption_key_version_id = '%s' WHERE path = '%s'`, string(ekvId), namespacePath),
	)
	require.NoError(t, err)
}

// encryptWithV1 encrypts data using the v1 global key and returns an EncryptedField tagged with the v1 EKV ID.
func (env *reencryptTestEnv) encryptWithV1(t *testing.T, plaintext []byte) encfield.EncryptedField {
	t.Helper()
	encrypted, err := encryptWithKey(env.globalKeyV1, plaintext)
	require.NoError(t, err)
	return encfield.EncryptedField{
		ID:   env.globalEKVID,
		Data: base64.StdEncoding.EncodeToString(encrypted),
	}
}

// createConnection inserts a connection directly via raw SQL.
func (env *reencryptTestEnv) createConnection(t *testing.T, namespace string) apid.ID {
	t.Helper()
	connId := apid.New(apid.PrefixConnection)
	connectorId := apid.New(apid.PrefixConnectorVersion)
	now := apctx.GetClock(env.ctx).Now().Format(time.RFC3339)
	_, err := env.rawDb.Exec(fmt.Sprintf(
		`INSERT INTO connections (id, namespace, state, connector_id, connector_version, created_at, updated_at) VALUES ('%s', '%s', 'ready', '%s', 1, '%s', '%s')`,
		string(connId), namespace, string(connectorId), now, now,
	))
	require.NoError(t, err)
	return connId
}

func (env *reencryptTestEnv) runReencrypt(t *testing.T) error {
	t.Helper()
	handler := NewEncryptServiceTaskHandler(env.cfg, env.db, env.enc, nil, env.logger)
	return handler.handleReencryptAll(env.ctx, asynq.NewTask(TaskTypeReencryptAll, nil))
}

func TestHandleReencryptAll(t *testing.T) {
	t.Run("re-encrypts actor with mismatched key version", func(t *testing.T) {
		env := setupReencryptTest(t)
		v2EKVID, _ := env.addGlobalV2(t)

		// Create actor encrypted with v1
		actorId := apid.New(apid.PrefixActor)
		ef := env.encryptWithV1(t, []byte("my-secret-key"))
		err := env.db.CreateActor(env.ctx, &database.Actor{
			Id:           actorId,
			Namespace:    "root",
			ExternalId:   "user1",
			EncryptedKey: &ef,
		})
		require.NoError(t, err)

		env.setNamespaceTarget(t, "root", v2EKVID)

		err = env.runReencrypt(t)
		require.NoError(t, err)

		// Verify actor is now encrypted with v2
		actor, err := env.db.GetActor(env.ctx, actorId)
		require.NoError(t, err)
		require.NotNil(t, actor.EncryptedKey)
		require.Equal(t, v2EKVID, actor.EncryptedKey.ID)

		decrypted, err := env.enc.DecryptString(env.ctx, *actor.EncryptedKey)
		require.NoError(t, err)
		require.Equal(t, "my-secret-key", decrypted)
	})

	t.Run("skips actor already at target", func(t *testing.T) {
		env := setupReencryptTest(t)

		// Create actor encrypted with current (only) version
		actorId := apid.New(apid.PrefixActor)
		ef, err := env.enc.EncryptStringForKey(env.ctx, globalEncryptionKeyID, "my-key")
		require.NoError(t, err)
		require.Equal(t, env.globalEKVID, ef.ID)

		err = env.db.CreateActor(env.ctx, &database.Actor{
			Id:           actorId,
			Namespace:    "root",
			ExternalId:   "user1",
			EncryptedKey: &ef,
		})
		require.NoError(t, err)

		env.setNamespaceTarget(t, "root", env.globalEKVID)

		err = env.runReencrypt(t)
		require.NoError(t, err)

		actor, err := env.db.GetActor(env.ctx, actorId)
		require.NoError(t, err)
		require.NotNil(t, actor.EncryptedKey)
		require.Equal(t, env.globalEKVID, actor.EncryptedKey.ID)
	})

	t.Run("re-encrypts oauth2 token via connection JOIN", func(t *testing.T) {
		env := setupReencryptTest(t)
		v2EKVID, _ := env.addGlobalV2(t)

		connId := env.createConnection(t, "root")

		accessEF := env.encryptWithV1(t, []byte("access-token"))
		refreshEF := env.encryptWithV1(t, []byte("refresh-token"))
		_, err := env.db.InsertOAuth2Token(env.ctx, connId, nil, refreshEF, accessEF, nil, "scope1", "scope1")
		require.NoError(t, err)

		env.setNamespaceTarget(t, "root", v2EKVID)

		err = env.runReencrypt(t)
		require.NoError(t, err)

		updatedToken, err := env.db.GetOAuth2Token(env.ctx, connId)
		require.NoError(t, err)
		require.Equal(t, v2EKVID, updatedToken.EncryptedAccessToken.ID)
		require.Equal(t, v2EKVID, updatedToken.EncryptedRefreshToken.ID)

		accessPlain, err := env.enc.DecryptString(env.ctx, updatedToken.EncryptedAccessToken)
		require.NoError(t, err)
		require.Equal(t, "access-token", accessPlain)

		refreshPlain, err := env.enc.DecryptString(env.ctx, updatedToken.EncryptedRefreshToken)
		require.NoError(t, err)
		require.Equal(t, "refresh-token", refreshPlain)
	})

	t.Run("empty no rows need re-encryption", func(t *testing.T) {
		env := setupReencryptTest(t)

		err := env.runReencrypt(t)
		require.NoError(t, err)
	})

	t.Run("re-encryption is idempotent", func(t *testing.T) {
		env := setupReencryptTest(t)
		v2EKVID, _ := env.addGlobalV2(t)

		actorId := apid.New(apid.PrefixActor)
		ef := env.encryptWithV1(t, []byte("secret"))
		err := env.db.CreateActor(env.ctx, &database.Actor{
			Id:           actorId,
			Namespace:    "root",
			ExternalId:   "user1",
			EncryptedKey: &ef,
		})
		require.NoError(t, err)

		env.setNamespaceTarget(t, "root", v2EKVID)

		// First run
		err = env.runReencrypt(t)
		require.NoError(t, err)

		actor, err := env.db.GetActor(env.ctx, actorId)
		require.NoError(t, err)
		require.Equal(t, v2EKVID, actor.EncryptedKey.ID)

		// Second run — should find nothing to re-encrypt
		err = env.runReencrypt(t)
		require.NoError(t, err)

		actor, err = env.db.GetActor(env.ctx, actorId)
		require.NoError(t, err)
		require.Equal(t, v2EKVID, actor.EncryptedKey.ID)

		decrypted, err := env.enc.DecryptString(env.ctx, *actor.EncryptedKey)
		require.NoError(t, err)
		require.Equal(t, "secret", decrypted)
	})

	t.Run("multiple fields on same row re-encrypted", func(t *testing.T) {
		env := setupReencryptTest(t)
		v2EKVID, _ := env.addGlobalV2(t)

		connId := env.createConnection(t, "root")

		// Both fields encrypted with v1
		accessEF := env.encryptWithV1(t, []byte("access"))
		refreshEF := env.encryptWithV1(t, []byte("refresh"))
		_, err := env.db.InsertOAuth2Token(env.ctx, connId, nil, refreshEF, accessEF, nil, "", "")
		require.NoError(t, err)

		env.setNamespaceTarget(t, "root", v2EKVID)

		err = env.runReencrypt(t)
		require.NoError(t, err)

		token, err := env.db.GetOAuth2Token(env.ctx, connId)
		require.NoError(t, err)
		require.Equal(t, v2EKVID, token.EncryptedAccessToken.ID, "access token should be re-encrypted")
		require.Equal(t, v2EKVID, token.EncryptedRefreshToken.ID, "refresh token should be re-encrypted")
	})
}

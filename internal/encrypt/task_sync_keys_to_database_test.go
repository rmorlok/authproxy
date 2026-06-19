package encrypt

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

// encryptKeyDataForTest encrypts a child KeyData config using the given wrapping material,
// returning the EncryptedField that should be stored on the child Key.
func encryptKeyDataForTest(
	t *testing.T,
	wrappingMaterialID apid.ID,
	wrappingKeyBytes []byte,
	childKeyData *sconfig.KeyData,
) *encfield.EncryptedField {
	t.Helper()

	jsonData, err := json.Marshal(childKeyData)
	require.NoError(t, err)

	encrypted, err := encryptWithKey(wrappingKeyBytes, jsonData)
	require.NoError(t, err)

	return &encfield.EncryptedField{
		ID:   wrappingMaterialID,
		Data: base64.StdEncoding.EncodeToString(encrypted),
	}
}

func createDataEncryptionKeyForTest(
	t *testing.T,
	ctx context.Context,
	db database.DB,
	keyID apid.ID,
	kd *sconfig.KeyData,
) (*database.DataEncryptionKey, []byte) {
	t.Helper()

	generated, err := kd.GenerateDataEncryptionKey(ctx)
	require.NoError(t, err)

	dek := &database.DataEncryptionKey{
		KeyId:           keyID,
		Provider:        string(generated.Provider),
		ProviderID:      generated.ProviderID,
		ProviderVersion: generated.ProviderVersion,
		ProtectedData:   &generated.ProtectedData,
		IsCurrent:       true,
	}
	require.NoError(t, db.CreateDataEncryptionKey(ctx, dek))

	return dek, append([]byte(nil), generated.Data...)
}

type mockKMSSyncTestEnv struct {
	ctx       context.Context
	cfg       config.C
	db        database.DB
	enc       E
	logger    *slog.Logger
	namespace string
	ekID      apid.ID
	dekV1ID   apid.ID
}

func setupMockKMSKeySyncTest(t *testing.T) mockKMSSyncTestEnv {
	t.Helper()

	sconfig.ResetKeyDataMockRegistry()
	sconfig.ResetKeyDataMockKMSRegistry()
	t.Cleanup(sconfig.ResetKeyDataMockRegistry)
	t.Cleanup(sconfig.ResetKeyDataMockKMSRegistry)

	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()
	logger := slog.Default()

	globalKeyBytes := util.MustGenerateSecureRandomKey(32)
	globalKD := sconfig.NewKeyDataMock("global-kms-parent")
	sconfig.KeyDataMockAddVersion("global-kms-parent", "global-key", "v1", globalKeyBytes)

	cfg := config.FromRoot(&sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			GlobalAESKey: globalKD,
		},
	})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	globalDEK, globalDEKBytes := createDataEncryptionKeyForTest(t, ctx, db, globalEncryptionKeyID, globalKD)

	require.NoError(t, syncKeysToDatabase(ctx, cfg, db, logger, nil))

	kmsKeyData := sconfig.NewKeyDataMockKMS("namespace-kms")
	sconfig.KeyDataMockKMSAddVersion("namespace-kms", "mock-kms-key", "v1", util.MustGenerateSecureRandomKey(32))

	ekID := apid.New(apid.PrefixKey)
	namespace := "root.kms"
	require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{
		Path: namespace,
	}))

	require.NoError(t, db.CreateKey(ctx, &database.Key{
		Id:               ekID,
		Namespace:        namespace,
		State:            database.KeyStateActive,
		EncryptedKeyData: encryptKeyDataForTest(t, globalDEK.Id, globalDEKBytes, kmsKeyData),
	}))
	_, err := db.SetNamespaceKeyId(ctx, namespace, &ekID)
	require.NoError(t, err)

	dekV1ID := createMockKMSDataEncryptionKey(t, ctx, db, ekID, "v1", true)
	require.NoError(t, syncKeysToDatabase(ctx, cfg, db, logger, nil))
	enc := newTestService(cfg, db)

	return mockKMSSyncTestEnv{
		ctx:       ctx,
		cfg:       cfg,
		db:        db,
		enc:       enc,
		logger:    logger,
		namespace: namespace,
		ekID:      ekID,
		dekV1ID:   dekV1ID,
	}
}

func createMockKMSDataEncryptionKey(
	t *testing.T,
	ctx context.Context,
	db database.DB,
	ekID apid.ID,
	providerVersion string,
	isCurrent bool,
) apid.ID {
	t.Helper()

	dekBytes := util.MustGenerateSecureRandomKey(32)
	protected, err := sconfig.KeyDataMockKMSWrap("namespace-kms", "mock-kms-key", providerVersion, dekBytes)
	require.NoError(t, err)

	dek := &database.DataEncryptionKey{
		KeyId:           ekID,
		Provider:        string(sconfig.ProviderTypeMockKMS),
		ProviderID:      "mock-kms-key",
		ProviderVersion: providerVersion,
		ProtectedData:   &protected,
		IsCurrent:       isCurrent,
	}
	require.NoError(t, db.CreateDataEncryptionKey(ctx, dek))
	return dek.Id
}

func TestMockKMSKeySync(t *testing.T) {
	t.Run("persists wrapped dek and decrypts after memory restart", func(t *testing.T) {
		env := setupMockKMSKeySyncTest(t)

		deks, err := env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Len(t, deks, 1)
		require.Equal(t, env.dekV1ID, deks[0].Id)
		require.NotNil(t, deks[0].ProtectedData)
		require.Equal(t, string(sconfig.ProviderTypeMockKMS), deks[0].ProtectedData.Type)
		require.NotEmpty(t, deks[0].ProtectedData.WrappedData)

		encrypted, err := env.enc.EncryptStringForNamespace(env.ctx, env.namespace, "kms plaintext")
		require.NoError(t, err)
		require.Equal(t, env.dekV1ID, encrypted.ID)

		restarted := newTestService(env.cfg, env.db)
		decrypted, err := restarted.DecryptString(env.ctx, encrypted)
		require.NoError(t, err)
		require.Equal(t, "kms plaintext", decrypted)

		require.NoError(t, syncKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))
		afterDEKs, err := env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Len(t, afterDEKs, 1, "resync with existing protected data should not generate duplicate DEKs")
		require.Equal(t, env.dekV1ID, afterDEKs[0].Id)
	})

	t.Run("sync does not create versions before dek generation", func(t *testing.T) {
		env := setupMockKMSKeySyncTest(t)

		ekID := apid.New(apid.PrefixKey)
		namespace := "root.kms.nodek"
		require.NoError(t, env.db.CreateNamespace(env.ctx, &database.Namespace{
			Path: namespace,
		}))

		keyData := sconfig.NewKeyDataMockKMS("namespace-kms")
		keyDataJSON, err := json.Marshal(keyData)
		require.NoError(t, err)
		encKeyData, err := env.enc.EncryptGlobal(env.ctx, keyDataJSON)
		require.NoError(t, err)
		require.NoError(t, env.db.CreateKey(env.ctx, &database.Key{
			Id:               ekID,
			Namespace:        namespace,
			State:            database.KeyStateActive,
			EncryptedKeyData: &encKeyData,
		}))
		_, err = env.db.SetNamespaceKeyId(env.ctx, namespace, &ekID)
		require.NoError(t, err)

		require.NoError(t, syncKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))
		deks, err := env.db.ListDataEncryptionKeysForKey(env.ctx, ekID)
		require.NoError(t, err)
		require.Empty(t, deks)
	})

	t.Run("rotation creates new protected dek and re-encrypts fields", func(t *testing.T) {
		env := setupMockKMSKeySyncTest(t)

		currentV1, err := env.db.GetCurrentDataEncryptionKeyForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Equal(t, env.dekV1ID, currentV1.Id)

		actorID := apid.New(apid.PrefixActor)
		encrypted, err := env.enc.EncryptStringForNamespace(env.ctx, env.namespace, "rotate me")
		require.NoError(t, err)
		require.Equal(t, env.dekV1ID, encrypted.ID)
		require.NoError(t, env.db.CreateActor(env.ctx, &database.Actor{
			Id:           actorID,
			Namespace:    env.namespace,
			ExternalId:   "kms-actor",
			EncryptedKey: &encrypted,
		}))

		sconfig.KeyDataMockKMSAddVersion("namespace-kms", "mock-kms-key", "v2", util.MustGenerateSecureRandomKey(32))
		dekV2ID := createMockKMSDataEncryptionKey(t, env.ctx, env.db, env.ekID, "v2", true)
		require.NoError(t, syncKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))
		require.NoError(t, env.enc.SyncKeysFromDbToMemory(env.ctx))

		currentV2, err := env.db.GetCurrentDataEncryptionKeyForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.NotEqual(t, currentV1.Id, currentV2.Id)
		require.Equal(t, dekV2ID, currentV2.Id)
		require.Equal(t, "v2", currentV2.ProviderVersion)
		requireNamespaceTarget(t, env.ctx, env.db, env.namespace, dekV2ID)

		handler := NewEncryptServiceTaskHandler(env.cfg, env.db, env.enc, nil, env.logger)
		require.NoError(t, handler.handleReencryptAll(env.ctx, asynq.NewTask(TaskTypeReencryptAll, nil)))

		actor, err := env.db.GetActor(env.ctx, actorID)
		require.NoError(t, err)
		require.NotNil(t, actor.EncryptedKey)
		require.Equal(t, dekV2ID, actor.EncryptedKey.ID)

		decrypted, err := env.enc.DecryptString(env.ctx, *actor.EncryptedKey)
		require.NoError(t, err)
		require.Equal(t, "rotate me", decrypted)
	})
}

type rewrapSyncTestEnv struct {
	ctx       context.Context
	cfg       config.C
	db        database.DB
	logger    *slog.Logger
	namespace string
	keyID     apid.ID
	keyData   *sconfig.KeyData
	dekID     apid.ID
	dekBytes  []byte
}

func setupRewrapSyncTest(t *testing.T) rewrapSyncTestEnv {
	t.Helper()

	sconfig.ResetKeyDataMockRegistry()
	sconfig.ResetKeyDataMockKMSRegistry()
	t.Cleanup(sconfig.ResetKeyDataMockRegistry)
	t.Cleanup(sconfig.ResetKeyDataMockKMSRegistry)

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
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	globalDEK, globalDEKBytes := createDataEncryptionKeyForTest(t, ctx, db, globalEncryptionKeyID, globalKD)

	keyBytes := util.MustGenerateSecureRandomKey(32)
	keyData := sconfig.NewKeyDataMock("namespace-secret")
	sconfig.KeyDataMockAddVersion("namespace-secret", "mock-secret-key", "v1", keyBytes)

	keyID := apid.New(apid.PrefixKey)
	namespace := "root.secret"
	require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{
		Path: namespace,
	}))

	require.NoError(t, db.CreateKey(ctx, &database.Key{
		Id:               keyID,
		Namespace:        namespace,
		State:            database.KeyStateActive,
		EncryptedKeyData: encryptKeyDataForTest(t, globalDEK.Id, globalDEKBytes, keyData),
	}))
	_, err := db.SetNamespaceKeyId(ctx, namespace, &keyID)
	require.NoError(t, err)

	dek, dekBytes := createDataEncryptionKeyForTest(t, ctx, db, keyID, keyData)
	require.NoError(t, syncKeysToDatabase(ctx, cfg, db, logger, nil))

	return rewrapSyncTestEnv{
		ctx:       ctx,
		cfg:       cfg,
		db:        db,
		logger:    logger,
		namespace: namespace,
		keyID:     keyID,
		keyData:   keyData,
		dekID:     dek.Id,
		dekBytes:  dekBytes,
	}
}

func unwrapDataEncryptionKeyForTest(
	t *testing.T,
	ctx context.Context,
	kd *sconfig.KeyData,
	dek *database.DataEncryptionKey,
) []byte {
	t.Helper()

	infos := dataEncryptionKeyInfos([]*database.DataEncryptionKey{dek})
	require.Len(t, infos, 1)
	plaintext, err := kd.UnwrapDataEncryptionKey(ctx, infos[0])
	require.NoError(t, err)
	return plaintext
}

func requireNamespaceTarget(
	t *testing.T,
	ctx context.Context,
	db database.DB,
	namespacePath string,
	expected apid.ID,
) {
	t.Helper()

	ns, err := db.GetNamespace(ctx, namespacePath)
	require.NoError(t, err)
	require.NotNil(t, ns.TargetDataEncryptionKeyId)
	require.Equal(t, expected, *ns.TargetDataEncryptionKeyId)
}

func TestSyncKeysToDatabase(t *testing.T) {
	t.Run("does not create or duplicate deks", func(t *testing.T) {
		env := setupRewrapSyncTest(t)

		globalDEKs, err := env.db.ListDataEncryptionKeysForKey(env.ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, globalDEKs, 1)

		keyDEKs, err := env.db.ListDataEncryptionKeysForKey(env.ctx, env.keyID)
		require.NoError(t, err)
		require.Len(t, keyDEKs, 1)

		require.NoError(t, syncKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))

		globalDEKs, err = env.db.ListDataEncryptionKeysForKey(env.ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, globalDEKs, 1)

		keyDEKs, err = env.db.ListDataEncryptionKeysForKey(env.ctx, env.keyID)
		require.NoError(t, err)
		require.Len(t, keyDEKs, 1)
	})

	t.Run("rewraps stale dek with current provider material", func(t *testing.T) {
		env := setupRewrapSyncTest(t)

		before, err := env.db.GetDataEncryptionKey(env.ctx, env.dekID)
		require.NoError(t, err)
		beforePlaintext := unwrapDataEncryptionKeyForTest(t, env.ctx, env.keyData, before)

		sconfig.KeyDataMockAddVersion("namespace-secret", "mock-secret-key", "v2", util.MustGenerateSecureRandomKey(32))

		require.NoError(t, syncKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))

		after, err := env.db.GetDataEncryptionKey(env.ctx, env.dekID)
		require.NoError(t, err)
		require.Equal(t, env.dekID, after.Id)
		require.Equal(t, before.KeyId, after.KeyId)
		require.Equal(t, before.IsCurrent, after.IsCurrent)
		require.True(t, before.CreatedAt.Equal(after.CreatedAt))
		require.Equal(t, "v2", after.ProviderVersion)
		require.NotEqual(t, before.ProtectedData.WrappedData, after.ProtectedData.WrappedData)
		require.Equal(t, beforePlaintext, unwrapDataEncryptionKeyForTest(t, env.ctx, env.keyData, after))
		requireNamespaceTarget(t, env.ctx, env.db, env.namespace, env.dekID)
	})

	t.Run("does not rewrite encrypted application fields during rewrap", func(t *testing.T) {
		env := setupRewrapSyncTest(t)
		enc := newTestService(env.cfg, env.db)

		actorID := apid.New(apid.PrefixActor)
		encrypted, err := enc.EncryptStringForNamespace(env.ctx, env.namespace, "leave me alone")
		require.NoError(t, err)
		require.Equal(t, env.dekID, encrypted.ID)
		require.NoError(t, env.db.CreateActor(env.ctx, &database.Actor{
			Id:           actorID,
			Namespace:    env.namespace,
			ExternalId:   "sync-does-not-reencrypt",
			EncryptedKey: &encrypted,
		}))

		actorBefore, err := env.db.GetActor(env.ctx, actorID)
		require.NoError(t, err)
		require.NotNil(t, actorBefore.EncryptedKey)

		sconfig.KeyDataMockAddVersion("namespace-secret", "mock-secret-key", "v2", util.MustGenerateSecureRandomKey(32))

		require.NoError(t, syncKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))

		afterDEK, err := env.db.GetDataEncryptionKey(env.ctx, env.dekID)
		require.NoError(t, err)
		require.Equal(t, "v2", afterDEK.ProviderVersion)

		actorAfter, err := env.db.GetActor(env.ctx, actorID)
		require.NoError(t, err)
		require.NotNil(t, actorAfter.EncryptedKey)
		require.Equal(t, actorBefore.EncryptedKey.ID, actorAfter.EncryptedKey.ID)
		require.Equal(t, actorBefore.EncryptedKey.Data, actorAfter.EncryptedKey.Data)
	})

	t.Run("no-op when wrapping material is current", func(t *testing.T) {
		env := setupRewrapSyncTest(t)

		before, err := env.db.GetDataEncryptionKey(env.ctx, env.dekID)
		require.NoError(t, err)

		require.NoError(t, syncKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))

		after, err := env.db.GetDataEncryptionKey(env.ctx, env.dekID)
		require.NoError(t, err)
		require.Equal(t, before.Provider, after.Provider)
		require.Equal(t, before.ProviderID, after.ProviderID)
		require.Equal(t, before.ProviderVersion, after.ProviderVersion)
		require.Equal(t, before.ProtectedData, after.ProtectedData)
		require.True(t, before.UpdatedAt.Equal(after.UpdatedAt))
	})

	t.Run("unwrap failure leaves dek unchanged", func(t *testing.T) {
		env := setupRewrapSyncTest(t)
		before, err := env.db.GetDataEncryptionKey(env.ctx, env.dekID)
		require.NoError(t, err)

		sconfig.KeyDataMockAddVersion("namespace-secret", "mock-secret-key", "v2", util.MustGenerateSecureRandomKey(32))
		sconfig.KeyDataMockRemoveVersion("namespace-secret", "v1")

		err = syncKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil)
		require.ErrorContains(t, err, "failed to unwrap data encryption key")

		after, getErr := env.db.GetDataEncryptionKey(env.ctx, env.dekID)
		require.NoError(t, getErr)
		require.Equal(t, before.ProviderVersion, after.ProviderVersion)
		require.Equal(t, before.ProtectedData, after.ProtectedData)
	})

	t.Run("wrap failure leaves dek unchanged", func(t *testing.T) {
		env := setupRewrapSyncTest(t)
		before, err := env.db.GetDataEncryptionKey(env.ctx, env.dekID)
		require.NoError(t, err)

		sconfig.KeyDataMockAddVersion("namespace-secret", "mock-secret-key", "v2", []byte("bad"))

		err = syncKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil)
		require.ErrorContains(t, err, "failed to rewrap data encryption key")

		after, getErr := env.db.GetDataEncryptionKey(env.ctx, env.dekID)
		require.NoError(t, getErr)
		require.Equal(t, before.ProviderVersion, after.ProviderVersion)
		require.Equal(t, before.ProtectedData, after.ProtectedData)
	})

	t.Run("rewraps mock kms protected metadata", func(t *testing.T) {
		env := setupMockKMSKeySyncTest(t)
		keyData := sconfig.NewKeyDataMockKMS("namespace-kms")

		before, err := env.db.GetDataEncryptionKey(env.ctx, env.dekV1ID)
		require.NoError(t, err)
		beforePlaintext := unwrapDataEncryptionKeyForTest(t, env.ctx, keyData, before)

		sconfig.KeyDataMockKMSAddVersion("namespace-kms", "mock-kms-key", "v2", util.MustGenerateSecureRandomKey(32))

		require.NoError(t, syncKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))

		after, err := env.db.GetDataEncryptionKey(env.ctx, env.dekV1ID)
		require.NoError(t, err)
		require.Equal(t, env.dekV1ID, after.Id)
		require.Equal(t, "v2", after.ProviderVersion)
		require.Equal(t, "v2", after.ProtectedData.Metadata["kek_version"])
		require.Equal(t, beforePlaintext, unwrapDataEncryptionKeyForTest(t, env.ctx, keyData, after))
	})

	t.Run("namespace targets resolve direct inherited and global deks idempotently", func(t *testing.T) {
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC))).Build()
		logger := slog.Default()

		globalKeyBytes := util.MustGenerateSecureRandomKey(32)
		globalKD := sconfig.NewKeyDataMock("global-targets")
		sconfig.KeyDataMockAddVersion("global-targets", "global-key", "v1", globalKeyBytes)

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
		globalDEK, globalDEKBytes := createDataEncryptionKeyForTest(t, ctx, db, globalEncryptionKeyID, globalKD)

		parentKeyBytes := util.MustGenerateSecureRandomKey(32)
		parentKeyData := sconfig.NewKeyDataMock("parent-targets")
		sconfig.KeyDataMockAddVersion("parent-targets", "parent-key", "v1", parentKeyBytes)

		parentKeyID := apid.New(apid.PrefixKey)
		require.NoError(t, db.CreateKey(ctx, &database.Key{
			Id:               parentKeyID,
			Namespace:        "root",
			State:            database.KeyStateActive,
			EncryptedKeyData: encryptKeyDataForTest(t, globalDEK.Id, globalDEKBytes, parentKeyData),
		}))
		parentDEK, _ := createDataEncryptionKeyForTest(t, ctx, db, parentKeyID, parentKeyData)

		require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{
			Path:  "root.parent",
			KeyId: &parentKeyID,
		}))
		require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{Path: "root.parent.child"}))
		require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{Path: "root.global"}))

		require.NoError(t, syncKeysToDatabase(ctx, cfg, db, logger, nil))
		requireNamespaceTarget(t, ctx, db, "root.parent", parentDEK.Id)
		requireNamespaceTarget(t, ctx, db, "root.parent.child", parentDEK.Id)
		requireNamespaceTarget(t, ctx, db, "root.global", globalDEK.Id)

		parentBefore, err := db.GetNamespace(ctx, "root.parent")
		require.NoError(t, err)

		require.NoError(t, syncKeysToDatabase(ctx, cfg, db, logger, nil))
		parentAfter, err := db.GetNamespace(ctx, "root.parent")
		require.NoError(t, err)
		require.Equal(t, parentBefore.TargetDataEncryptionKeyId, parentAfter.TargetDataEncryptionKeyId)
		require.True(t, parentBefore.UpdatedAt.Equal(parentAfter.UpdatedAt))
	})
}

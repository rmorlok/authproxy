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
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

// encryptKeyDataForTest encrypts a child KeyData config using the given parent key version,
// returning the EncryptedField that should be stored on the child Key.
func encryptKeyDataForTest(
	t *testing.T,
	parentEKVID apid.ID,
	parentKeyBytes []byte,
	childKeyData *sconfig.KeyData,
) *encfield.EncryptedField {
	t.Helper()

	jsonData, err := json.Marshal(childKeyData)
	require.NoError(t, err)

	encrypted, err := encryptWithKey(parentKeyBytes, jsonData)
	require.NoError(t, err)

	return &encfield.EncryptedField{
		ID:   parentEKVID,
		Data: base64.StdEncoding.EncodeToString(encrypted),
	}
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

	require.NoError(t, syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil))

	globalVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
	require.NoError(t, err)
	require.Len(t, globalVersions, 1)

	kmsKeyData := sconfig.NewKeyDataMockKMS("namespace-kms")
	sconfig.KeyDataMockKMSAddVersion("namespace-kms", "mock-kms-key", "v1", util.MustGenerateSecureRandomKey(32))

	ekID := apid.New(apid.PrefixKey)
	namespace := "root.kms"
	require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{
		Path:  namespace,
		KeyId: &ekID,
	}))

	require.NoError(t, db.CreateKey(ctx, &database.Key{
		Id:               ekID,
		Namespace:        namespace,
		State:            database.KeyStateActive,
		EncryptedKeyData: encryptKeyDataForTest(t, globalVersions[0].Id, globalKeyBytes, kmsKeyData),
	}))

	dekV1ID := createMockKMSDataEncryptionKey(t, ctx, db, ekID, "v1", true)
	require.NoError(t, syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil))
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

		versions, err := env.db.ListEncryptionKeyVersionsForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Len(t, versions, 1)
		require.Equal(t, string(sconfig.ProviderTypeMockKMS), versions[0].Provider)
		require.Equal(t, string(env.dekV1ID), versions[0].ProviderID)
		require.Equal(t, "v1", versions[0].ProviderVersion)

		encrypted, err := env.enc.EncryptStringForNamespace(env.ctx, env.namespace, "kms plaintext")
		require.NoError(t, err)
		require.Equal(t, versions[0].Id, encrypted.ID)

		restarted := newTestService(env.cfg, env.db)
		decrypted, err := restarted.DecryptString(env.ctx, encrypted)
		require.NoError(t, err)
		require.Equal(t, "kms plaintext", decrypted)

		require.NoError(t, syncKeysVersionsToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))
		afterResync, err := env.db.ListEncryptionKeyVersionsForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Len(t, afterResync, 1, "resync with existing protected data should not generate a duplicate version")
		require.Equal(t, versions[0].Id, afterResync[0].Id)
	})

	t.Run("sync does not create versions before dek generation", func(t *testing.T) {
		env := setupMockKMSKeySyncTest(t)

		ekID := apid.New(apid.PrefixKey)
		namespace := "root.kms.nodek"
		require.NoError(t, env.db.CreateNamespace(env.ctx, &database.Namespace{
			Path:  namespace,
			KeyId: &ekID,
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

		require.NoError(t, syncKeysVersionsToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))
		versions, err := env.db.ListEncryptionKeyVersionsForKey(env.ctx, ekID)
		require.NoError(t, err)
		require.Empty(t, versions)
	})

	t.Run("rotation creates new protected dek and re-encrypts fields", func(t *testing.T) {
		env := setupMockKMSKeySyncTest(t)

		currentV1, err := env.db.GetCurrentEncryptionKeyVersionForNamespace(env.ctx, env.namespace)
		require.NoError(t, err)

		actorID := apid.New(apid.PrefixActor)
		encrypted, err := env.enc.EncryptStringForNamespace(env.ctx, env.namespace, "rotate me")
		require.NoError(t, err)
		require.Equal(t, currentV1.Id, encrypted.ID)
		require.NoError(t, env.db.CreateActor(env.ctx, &database.Actor{
			Id:           actorID,
			Namespace:    env.namespace,
			ExternalId:   "kms-actor",
			EncryptedKey: &encrypted,
		}))

		sconfig.KeyDataMockKMSAddVersion("namespace-kms", "mock-kms-key", "v2", util.MustGenerateSecureRandomKey(32))
		dekV2ID := createMockKMSDataEncryptionKey(t, env.ctx, env.db, env.ekID, "v2", true)
		require.NoError(t, syncKeysVersionsToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))
		require.NoError(t, env.enc.SyncKeysFromDbToMemory(env.ctx))

		currentV2, err := env.db.GetCurrentEncryptionKeyVersionForNamespace(env.ctx, env.namespace)
		require.NoError(t, err)
		require.NotEqual(t, currentV1.Id, currentV2.Id)
		require.Equal(t, string(dekV2ID), currentV2.ProviderID)
		require.Equal(t, "v2", currentV2.ProviderVersion)

		handler := NewEncryptServiceTaskHandler(env.cfg, env.db, env.enc, nil, env.logger)
		require.NoError(t, handler.handleReencryptAll(env.ctx, asynq.NewTask(TaskTypeReencryptAll, nil)))

		actor, err := env.db.GetActor(env.ctx, actorID)
		require.NoError(t, err)
		require.NotNil(t, actor.EncryptedKey)
		require.Equal(t, currentV2.Id, actor.EncryptedKey.ID)

		decrypted, err := env.enc.DecryptString(env.ctx, *actor.EncryptedKey)
		require.NoError(t, err)
		require.Equal(t, "rotate me", decrypted)
	})
}

func TestSyncKeysVersionsToDatabase(t *testing.T) {
	logger := slog.Default()

	t.Run("global key only then add versions", func(t *testing.T) {
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		globalKeyBytes := util.MustGenerateSecureRandomKey(32)
		globalKD := sconfig.NewKeyDataMock("global")
		sconfig.KeyDataMockAddVersion("global", "global-key", "v1", globalKeyBytes)

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)

		// --- Iteration 1: initial sync with single global key version ---
		err := syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		versions, err := db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, versions, 1)
		require.Equal(t, string(sconfig.ProviderTypeMock), versions[0].Provider)
		require.Equal(t, "global-key", versions[0].ProviderID)
		require.Equal(t, "v1", versions[0].ProviderVersion)
		require.True(t, versions[0].IsCurrent)
		require.Equal(t, int64(1), versions[0].OrderedVersion)

		// --- Iteration 2: add a second version to the global key ---
		globalKeyBytesV2 := util.MustGenerateSecureRandomKey(32)
		sconfig.KeyDataMockAddVersion("global", "global-key", "v2", globalKeyBytesV2)

		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		versions, err = db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, versions, 2)

		var v1, v2 *database.EncryptionKeyVersion
		for _, v := range versions {
			switch v.ProviderVersion {
			case "v1":
				v1 = v
			case "v2":
				v2 = v
			}
		}
		require.NotNil(t, v1)
		require.NotNil(t, v2)
		require.False(t, v1.IsCurrent, "v1 should no longer be current")
		require.True(t, v2.IsCurrent, "v2 should be current")
		require.Equal(t, int64(1), v1.OrderedVersion)
		require.Equal(t, int64(2), v2.OrderedVersion)

		// --- Iteration 3: re-sync is idempotent ---
		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		versions, err = db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, versions, 2, "idempotent sync should not create duplicates")
	})

	t.Run("three level key hierarchy", func(t *testing.T) {
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		globalKeyBytes := util.MustGenerateSecureRandomKey(32)
		globalKD := sconfig.NewKeyDataMock("global")
		sconfig.KeyDataMockAddVersion("global", "global-key", "v1", globalKeyBytes)

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)

		// --- Iteration 1: sync global key only ---
		err := syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		globalVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, globalVersions, 1)
		globalEKVID := globalVersions[0].Id

		// --- Iteration 2: add a child key (depth 1) encrypted by the global key ---
		childKeyBytes := util.MustGenerateSecureRandomKey(32)
		childMockID := "child"
		sconfig.NewKeyDataMock(childMockID)
		sconfig.KeyDataMockAddVersion(childMockID, "child-key", "cv1", childKeyBytes)
		childKeyData := sconfig.NewKeyDataMock(childMockID)

		childEKID := apid.New(apid.PrefixKey)
		childEF := encryptKeyDataForTest(t, globalEKVID, globalKeyBytes, childKeyData)

		childEK := &database.Key{
			Id:               childEKID,
			Namespace:        "root",
			State:            database.KeyStateActive,
			EncryptedKeyData: childEF,
		}
		require.NoError(t, db.CreateKey(ctx, childEK))

		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		childVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, childEKID)
		require.NoError(t, err)
		require.Len(t, childVersions, 1)
		require.True(t, childVersions[0].IsCurrent)
		require.Equal(t, string(sconfig.ProviderTypeMock), childVersions[0].Provider)
		childEKVID := childVersions[0].Id

		// --- Iteration 3: add a grandchild key (depth 2) encrypted by the child key ---
		grandchildKeyBytes := util.MustGenerateSecureRandomKey(32)
		grandchildMockID := "grandchild"
		sconfig.NewKeyDataMock(grandchildMockID)
		sconfig.KeyDataMockAddVersion(grandchildMockID, "grandchild-key", "gv1", grandchildKeyBytes)
		grandchildKeyData := sconfig.NewKeyDataMock(grandchildMockID)

		grandchildEKID := apid.New(apid.PrefixKey)
		grandchildEF := encryptKeyDataForTest(t, childEKVID, childKeyBytes, grandchildKeyData)

		grandchildEK := &database.Key{
			Id:               grandchildEKID,
			Namespace:        "root",
			State:            database.KeyStateActive,
			EncryptedKeyData: grandchildEF,
		}
		require.NoError(t, db.CreateKey(ctx, grandchildEK))

		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		grandchildVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, grandchildEKID)
		require.NoError(t, err)
		require.Len(t, grandchildVersions, 1)
		require.True(t, grandchildVersions[0].IsCurrent)

		// Verify all three keys still have exactly one version
		globalVersions, err = db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, globalVersions, 1)

		childVersions, err = db.ListEncryptionKeyVersionsForKey(ctx, childEKID)
		require.NoError(t, err)
		require.Len(t, childVersions, 1)

		// --- Iteration 4: idempotent re-sync ---
		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		globalVersions, err = db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, globalVersions, 1)

		childVersions, err = db.ListEncryptionKeyVersionsForKey(ctx, childEKID)
		require.NoError(t, err)
		require.Len(t, childVersions, 1)

		grandchildVersions, err = db.ListEncryptionKeyVersionsForKey(ctx, grandchildEKID)
		require.NoError(t, err)
		require.Len(t, grandchildVersions, 1)
	})

	t.Run("child key gets new version added", func(t *testing.T) {
		// Scenario: a child key backed by a mock provider gains a new version between syncs.
		// Because the mock registry is shared, the deserialized KeyDataMock sees the new version.
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		globalKeyBytes := util.MustGenerateSecureRandomKey(32)
		globalKD := sconfig.NewKeyDataMock("global")
		sconfig.KeyDataMockAddVersion("global", "global-key", "v1", globalKeyBytes)

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)

		// Sync global key
		err := syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		globalVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		globalEKVID := globalVersions[0].Id

		// Create child key with mock provider starting with cv1
		childKeyBytesV1 := util.MustGenerateSecureRandomKey(32)
		childMockID := "child"
		sconfig.NewKeyDataMock(childMockID)
		sconfig.KeyDataMockAddVersion(childMockID, "child-key", "cv1", childKeyBytesV1)
		childKeyData := sconfig.NewKeyDataMock(childMockID)

		childEKID := apid.New(apid.PrefixKey)
		childEF := encryptKeyDataForTest(t, globalEKVID, globalKeyBytes, childKeyData)

		childEK := &database.Key{
			Id:               childEKID,
			Namespace:        "root",
			State:            database.KeyStateActive,
			EncryptedKeyData: childEF,
		}
		require.NoError(t, db.CreateKey(ctx, childEK))

		// Sync: child should have 1 version
		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		childVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, childEKID)
		require.NoError(t, err)
		require.Len(t, childVersions, 1)
		require.True(t, childVersions[0].IsCurrent)
		require.Equal(t, "cv1", childVersions[0].ProviderVersion)

		// Add a second version to the child mock — no need to re-encrypt, the mock registry
		// is shared and the deserialized KeyDataMock will see the new version.
		childKeyBytesV2 := util.MustGenerateSecureRandomKey(32)
		sconfig.KeyDataMockAddVersion(childMockID, "child-key", "cv2", childKeyBytesV2)

		// Sync again: child should now have 2 versions
		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		childVersions, err = db.ListEncryptionKeyVersionsForKey(ctx, childEKID)
		require.NoError(t, err)
		require.Len(t, childVersions, 2)

		var cv1, cv2 *database.EncryptionKeyVersion
		for _, v := range childVersions {
			switch v.ProviderVersion {
			case "cv1":
				cv1 = v
			case "cv2":
				cv2 = v
			}
		}
		require.NotNil(t, cv1)
		require.NotNil(t, cv2)
		require.False(t, cv1.IsCurrent, "cv1 should no longer be current")
		require.True(t, cv2.IsCurrent, "cv2 should be current")
		require.Equal(t, int64(1), cv1.OrderedVersion)
		require.Equal(t, int64(2), cv2.OrderedVersion)
	})

	t.Run("child key version removed", func(t *testing.T) {
		// Scenario: a child key's old version is removed from the mock provider.
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		globalKeyBytes := util.MustGenerateSecureRandomKey(32)
		globalKD := sconfig.NewKeyDataMock("global")
		sconfig.KeyDataMockAddVersion("global", "global-key", "v1", globalKeyBytes)

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)

		err := syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		globalVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		globalEKVID := globalVersions[0].Id

		// Create child with two versions
		childKeyBytesV1 := util.MustGenerateSecureRandomKey(32)
		childKeyBytesV2 := util.MustGenerateSecureRandomKey(32)
		childMockID := "child"
		sconfig.NewKeyDataMock(childMockID)
		sconfig.KeyDataMockAddVersion(childMockID, "child-key", "cv1", childKeyBytesV1)
		sconfig.KeyDataMockAddVersion(childMockID, "child-key", "cv2", childKeyBytesV2)
		childKeyData := sconfig.NewKeyDataMock(childMockID)

		childEKID := apid.New(apid.PrefixKey)
		childEF := encryptKeyDataForTest(t, globalEKVID, globalKeyBytes, childKeyData)

		childEK := &database.Key{
			Id:               childEKID,
			Namespace:        "root",
			State:            database.KeyStateActive,
			EncryptedKeyData: childEF,
		}
		require.NoError(t, db.CreateKey(ctx, childEK))

		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		childVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, childEKID)
		require.NoError(t, err)
		require.Len(t, childVersions, 2)

		// Remove cv1
		sconfig.KeyDataMockRemoveVersion(childMockID, "cv1")

		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		childVersions, err = db.ListEncryptionKeyVersionsForKey(ctx, childEKID)
		require.NoError(t, err)
		require.Len(t, childVersions, 1)
		require.Equal(t, "cv2", childVersions[0].ProviderVersion)
		require.True(t, childVersions[0].IsCurrent)
	})

	t.Run("version removed from global provider gets deleted", func(t *testing.T) {
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		globalKD := sconfig.NewKeyDataMock("global")
		globalKeyBytesV1 := util.MustGenerateSecureRandomKey(32)
		sconfig.KeyDataMockAddVersion("global", "global-key", "v1", globalKeyBytesV1)

		// Add second version before first sync
		globalKeyBytesV2 := util.MustGenerateSecureRandomKey(32)
		sconfig.KeyDataMockAddVersion("global", "global-key", "v2", globalKeyBytesV2)

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)

		err := syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		versions, err := db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, versions, 2)

		// Remove v1 from the provider
		sconfig.KeyDataMockRemoveVersion("global", "v1")

		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		versions, err = db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, versions, 1)
		require.Equal(t, "v2", versions[0].ProviderVersion)
		require.True(t, versions[0].IsCurrent)
	})

	t.Run("global key rotated then child added using new version", func(t *testing.T) {
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		globalKeyBytesV1 := util.MustGenerateSecureRandomKey(32)
		globalKD := sconfig.NewKeyDataMock("global")
		sconfig.KeyDataMockAddVersion("global", "global-key", "v1", globalKeyBytesV1)

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)

		// Sync v1
		err := syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		// Add v2 to global and sync
		globalKeyBytesV2 := util.MustGenerateSecureRandomKey(32)
		sconfig.KeyDataMockAddVersion("global", "global-key", "v2", globalKeyBytesV2)

		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		// Get v2's EKV ID
		globalVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		var globalV2EKVID apid.ID
		for _, v := range globalVersions {
			if v.ProviderVersion == "v2" {
				globalV2EKVID = v.Id
			}
		}
		require.False(t, globalV2EKVID.IsNil())

		// Create a child key encrypted with global v2
		childKeyBytes := util.MustGenerateSecureRandomKey(32)
		childMockID := "child"
		sconfig.NewKeyDataMock(childMockID)
		sconfig.KeyDataMockAddVersion(childMockID, "child-key", "cv1", childKeyBytes)
		childKeyData := sconfig.NewKeyDataMock(childMockID)

		childEKID := apid.New(apid.PrefixKey)
		childEF := encryptKeyDataForTest(t, globalV2EKVID, globalKeyBytesV2, childKeyData)

		childEK := &database.Key{
			Id:               childEKID,
			Namespace:        "root",
			State:            database.KeyStateActive,
			EncryptedKeyData: childEF,
		}
		require.NoError(t, db.CreateKey(ctx, childEK))

		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		childVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, childEKID)
		require.NoError(t, err)
		require.Len(t, childVersions, 1)
		require.True(t, childVersions[0].IsCurrent)
	})

	t.Run("global key with multiple versions and child encrypted with non-current", func(t *testing.T) {
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		globalKeyBytesV1 := util.MustGenerateSecureRandomKey(32)
		globalKD := sconfig.NewKeyDataMock("global")
		sconfig.KeyDataMockAddVersion("global", "global-key", "v1", globalKeyBytesV1)

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)

		// Sync v1
		err := syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		// Get v1's EKV ID before adding v2
		globalVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		globalV1EKVID := globalVersions[0].Id

		// Create a child encrypted with v1
		childKeyBytes := util.MustGenerateSecureRandomKey(32)
		childMockID := "child"
		sconfig.NewKeyDataMock(childMockID)
		sconfig.KeyDataMockAddVersion(childMockID, "child-key", "cv1", childKeyBytes)
		childKeyData := sconfig.NewKeyDataMock(childMockID)

		childEKID := apid.New(apid.PrefixKey)
		childEF := encryptKeyDataForTest(t, globalV1EKVID, globalKeyBytesV1, childKeyData)

		childEK := &database.Key{
			Id:               childEKID,
			Namespace:        "root",
			State:            database.KeyStateActive,
			EncryptedKeyData: childEF,
		}
		require.NoError(t, db.CreateKey(ctx, childEK))

		// Now add v2 to global, making v1 non-current
		globalKeyBytesV2 := util.MustGenerateSecureRandomKey(32)
		sconfig.KeyDataMockAddVersion("global", "global-key", "v2", globalKeyBytesV2)

		// Sync: should handle child encrypted with the now-non-current v1
		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		childVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, childEKID)
		require.NoError(t, err)
		require.Len(t, childVersions, 1)
		require.True(t, childVersions[0].IsCurrent)
	})

	t.Run("namespace with encryption_key_id gets target_encryption_key_version_id set", func(t *testing.T) {
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		globalKeyBytes := util.MustGenerateSecureRandomKey(32)
		globalKD := sconfig.NewKeyDataMock("global")
		sconfig.KeyDataMockAddVersion("global", "global-key", "v1", globalKeyBytes)

		childKeyBytes := util.MustGenerateSecureRandomKey(32)
		sconfig.NewKeyDataMock("child")
		sconfig.KeyDataMockAddVersion("child", "child-key", "cv1", childKeyBytes)
		childKeyData := sconfig.NewKeyDataMock("child")

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)

		// Sync global key first
		err := syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		globalVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		globalEKVID := globalVersions[0].Id

		// Create child encryption key
		childEKID := apid.New(apid.PrefixKey)
		childEF := encryptKeyDataForTest(t, globalEKVID, globalKeyBytes, childKeyData)
		childEK := &database.Key{
			Id:               childEKID,
			Namespace:        "root",
			State:            database.KeyStateActive,
			EncryptedKeyData: childEF,
		}
		require.NoError(t, db.CreateKey(ctx, childEK))

		// Create namespace that uses the child encryption key
		require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{
			Path:  "root.withkey",
			KeyId: &childEKID,
		}))

		// Sync: should set target_encryption_key_version_id on the namespace
		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		childVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, childEKID)
		require.NoError(t, err)
		require.Len(t, childVersions, 1)
		childEKVID := childVersions[0].Id

		// Verify the namespace got the correct target
		var collected []database.NamespaceEncryptionTarget
		err = db.EnumerateNamespaceEncryptionTargets(ctx,
			func(targets []database.NamespaceEncryptionTarget, lastPage bool) ([]database.NamespaceTargetDataEncryptionKeyUpdate, pagination.KeepGoing, error) {
				collected = append(collected, targets...)
				return nil, pagination.Continue, nil
			},
		)
		require.NoError(t, err)

		for _, target := range collected {
			if target.Path == "root.withkey" {
				require.NotNil(t, target.TargetDataEncryptionKeyId)
				require.Equal(t, childEKVID, *target.TargetDataEncryptionKeyId)
			}
		}
	})

	t.Run("namespace without encryption_key_id inherits from ancestor", func(t *testing.T) {
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		globalKeyBytes := util.MustGenerateSecureRandomKey(32)
		globalKD := sconfig.NewKeyDataMock("global")
		sconfig.KeyDataMockAddVersion("global", "global-key", "v1", globalKeyBytes)

		parentKeyBytes := util.MustGenerateSecureRandomKey(32)
		sconfig.NewKeyDataMock("parent")
		sconfig.KeyDataMockAddVersion("parent", "parent-key", "pv1", parentKeyBytes)
		parentKeyData := sconfig.NewKeyDataMock("parent")

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)

		// Sync global key
		err := syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		globalVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		globalEKVID := globalVersions[0].Id

		// Create parent encryption key
		parentEKID := apid.New(apid.PrefixKey)
		parentEF := encryptKeyDataForTest(t, globalEKVID, globalKeyBytes, parentKeyData)
		parentEK := &database.Key{
			Id:               parentEKID,
			Namespace:        "root",
			State:            database.KeyStateActive,
			EncryptedKeyData: parentEF,
		}
		require.NoError(t, db.CreateKey(ctx, parentEK))

		// Create parent namespace with encryption key
		require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{
			Path:  "root.parent",
			KeyId: &parentEKID,
		}))

		// Create child namespace WITHOUT encryption key — should inherit from parent
		require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{
			Path: "root.parent.child",
		}))

		// Sync
		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		parentVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, parentEKID)
		require.NoError(t, err)
		require.Len(t, parentVersions, 1)
		parentEKVID := parentVersions[0].Id

		// Verify child inherited from parent
		var collected []database.NamespaceEncryptionTarget
		err = db.EnumerateNamespaceEncryptionTargets(ctx,
			func(targets []database.NamespaceEncryptionTarget, lastPage bool) ([]database.NamespaceTargetDataEncryptionKeyUpdate, pagination.KeepGoing, error) {
				collected = append(collected, targets...)
				return nil, pagination.Continue, nil
			},
		)
		require.NoError(t, err)

		for _, target := range collected {
			if target.Path == "root.parent.child" {
				require.NotNil(t, target.TargetDataEncryptionKeyId)
				require.Equal(t, parentEKVID, *target.TargetDataEncryptionKeyId)
			}
			if target.Path == "root.parent" {
				require.NotNil(t, target.TargetDataEncryptionKeyId)
				require.Equal(t, parentEKVID, *target.TargetDataEncryptionKeyId)
			}
		}
	})

	t.Run("namespace falls back to global key when no ancestor has encryption key", func(t *testing.T) {
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		globalKeyBytes := util.MustGenerateSecureRandomKey(32)
		globalKD := sconfig.NewKeyDataMock("global")
		sconfig.KeyDataMockAddVersion("global", "global-key", "v1", globalKeyBytes)

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)

		// Create namespace without encryption key (root already exists from db init)
		require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{
			Path: "root.nokey",
		}))

		// Sync
		err := syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		globalVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, globalVersions, 1)
		globalEKVID := globalVersions[0].Id

		// Verify namespace uses global key version
		var collected []database.NamespaceEncryptionTarget
		err = db.EnumerateNamespaceEncryptionTargets(ctx,
			func(targets []database.NamespaceEncryptionTarget, lastPage bool) ([]database.NamespaceTargetDataEncryptionKeyUpdate, pagination.KeepGoing, error) {
				collected = append(collected, targets...)
				return nil, pagination.Continue, nil
			},
		)
		require.NoError(t, err)

		for _, target := range collected {
			if target.Path == "root.nokey" {
				require.NotNil(t, target.TargetDataEncryptionKeyId)
				require.Equal(t, globalEKVID, *target.TargetDataEncryptionKeyId)
			}
		}
	})

	t.Run("idempotent re-sync produces no spurious updates", func(t *testing.T) {
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		globalKeyBytes := util.MustGenerateSecureRandomKey(32)
		globalKD := sconfig.NewKeyDataMock("global")
		sconfig.KeyDataMockAddVersion("global", "global-key", "v1", globalKeyBytes)

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)

		require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{
			Path: "root.stable",
		}))

		// First sync sets everything
		err := syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		// Capture state after first sync
		var firstCollected []database.NamespaceEncryptionTarget
		err = db.EnumerateNamespaceEncryptionTargets(ctx,
			func(targets []database.NamespaceEncryptionTarget, lastPage bool) ([]database.NamespaceTargetDataEncryptionKeyUpdate, pagination.KeepGoing, error) {
				firstCollected = append(firstCollected, targets...)
				return nil, pagination.Continue, nil
			},
		)
		require.NoError(t, err)

		// Second sync should be idempotent
		err = syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil)
		require.NoError(t, err)

		var secondCollected []database.NamespaceEncryptionTarget
		err = db.EnumerateNamespaceEncryptionTargets(ctx,
			func(targets []database.NamespaceEncryptionTarget, lastPage bool) ([]database.NamespaceTargetDataEncryptionKeyUpdate, pagination.KeepGoing, error) {
				secondCollected = append(secondCollected, targets...)
				return nil, pagination.Continue, nil
			},
		)
		require.NoError(t, err)

		require.Equal(t, len(firstCollected), len(secondCollected))
		for i := range firstCollected {
			require.Equal(t, firstCollected[i].Path, secondCollected[i].Path)
			require.Equal(t, firstCollected[i].TargetDataEncryptionKeyId, secondCollected[i].TargetDataEncryptionKeyId)
		}
	})
}

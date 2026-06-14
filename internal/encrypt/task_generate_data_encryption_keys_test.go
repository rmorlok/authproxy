package encrypt

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

type mockKMSGenerateTestEnv struct {
	ctx       context.Context
	clock     *clock.FakeClock
	cfg       config.C
	db        database.DB
	logger    *slog.Logger
	namespace string
	ekID      apid.ID
}

func setupMockKMSGenerateTest(t *testing.T, kmsVersions bool) mockKMSGenerateTestEnv {
	t.Helper()

	sconfig.ResetKeyDataMockRegistry()
	sconfig.ResetKeyDataMockKMSRegistry()
	t.Cleanup(sconfig.ResetKeyDataMockRegistry)
	t.Cleanup(sconfig.ResetKeyDataMockKMSRegistry)

	now := time.Date(2026, time.June, 13, 10, 0, 0, 0, time.UTC)
	fakeClock := clock.NewFakeClock(now)
	ctx := apctx.NewBuilderBackground().WithClock(fakeClock).Build()
	logger := slog.Default()

	globalKeyBytes := util.MustGenerateSecureRandomKey(32)
	globalKD := sconfig.NewKeyDataMock("global-kms-parent")
	sconfig.KeyDataMockAddVersion("global-kms-parent", "global-key", "v1", globalKeyBytes)

	cfg := config.FromRoot(&sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			GlobalAESKey: globalKD,
			DataEncryptionKeys: &sconfig.DataEncryptionKeys{
				RotationInterval: &sconfig.HumanDuration{Duration: time.Hour},
			},
		},
	})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)

	require.NoError(t, syncKeysVersionsToDatabase(ctx, cfg, db, logger, nil))

	globalVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, globalEncryptionKeyID)
	require.NoError(t, err)
	require.Len(t, globalVersions, 1)

	kmsKeyData := sconfig.NewKeyDataMockKMS("namespace-kms")
	if kmsVersions {
		sconfig.KeyDataMockKMSAddVersion("namespace-kms", "mock-kms-key", "v1", util.MustGenerateSecureRandomKey(32))
	}

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

	return mockKMSGenerateTestEnv{
		ctx:       ctx,
		clock:     fakeClock,
		cfg:       cfg,
		db:        db,
		logger:    logger,
		namespace: namespace,
		ekID:      ekID,
	}
}

func TestGenerateDataEncryptionKeysToDatabase(t *testing.T) {
	t.Run("creates first dek and synced key version", func(t *testing.T) {
		env := setupMockKMSGenerateTest(t, true)

		require.NoError(t, generateDataEncryptionKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))

		deks, err := env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Len(t, deks, 1)
		require.True(t, deks[0].Id.HasPrefix(apid.PrefixDataEncryptionKey))
		require.True(t, deks[0].IsCurrent)
		require.Equal(t, string(sconfig.ProviderTypeMockKMS), deks[0].Provider)
		require.Equal(t, "mock-kms-key", deks[0].ProviderID)
		require.Equal(t, "v1", deks[0].ProviderVersion)
		require.NotEmpty(t, deks[0].ProtectedData.WrappedData)

		versions, err := env.db.ListEncryptionKeyVersionsForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Len(t, versions, 1)
		require.Equal(t, string(deks[0].Id), versions[0].ProviderID)
		require.True(t, versions[0].IsCurrent)

		enc := newTestService(env.cfg, env.db)
		encrypted, err := enc.EncryptStringForNamespace(env.ctx, env.namespace, "generated dek")
		require.NoError(t, err)
		require.Equal(t, versions[0].Id, encrypted.ID)
		decrypted, err := enc.DecryptString(env.ctx, encrypted)
		require.NoError(t, err)
		require.Equal(t, "generated dek", decrypted)
	})

	t.Run("does not create duplicate when policy is satisfied", func(t *testing.T) {
		env := setupMockKMSGenerateTest(t, true)

		require.NoError(t, generateDataEncryptionKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))
		require.NoError(t, generateDataEncryptionKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))

		deks, err := env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Len(t, deks, 1)
	})

	t.Run("rotates when current dek exceeds policy age", func(t *testing.T) {
		env := setupMockKMSGenerateTest(t, true)

		require.NoError(t, generateDataEncryptionKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))
		firstCurrent, err := env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Len(t, firstCurrent, 1)

		sconfig.KeyDataMockKMSAddVersion("namespace-kms", "mock-kms-key", "v2", util.MustGenerateSecureRandomKey(32))
		env.clock.Step(2 * time.Hour)

		require.NoError(t, generateDataEncryptionKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))

		deks, err := env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Len(t, deks, 2)
		require.False(t, deks[0].IsCurrent)
		require.True(t, deks[1].IsCurrent)
		require.Equal(t, "v2", deks[1].ProviderVersion)

		versions, err := env.db.ListEncryptionKeyVersionsForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Len(t, versions, 2)
		require.Equal(t, string(deks[1].Id), versions[1].ProviderID)
		require.True(t, versions[1].IsCurrent)
	})

	t.Run("does not create first dek when ensure current is disabled", func(t *testing.T) {
		env := setupMockKMSGenerateTest(t, true)
		env.cfg.GetRoot().SystemAuth.DataEncryptionKeys.EnsureCurrent = util.ToPtr(false)

		require.NoError(t, generateDataEncryptionKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))

		deks, err := env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Empty(t, deks)
	})

	t.Run("returns provider errors without creating partial dek", func(t *testing.T) {
		env := setupMockKMSGenerateTest(t, false)

		err := generateDataEncryptionKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil)
		require.ErrorContains(t, err, "no current version")

		deks, listErr := env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, listErr)
		require.Empty(t, deks)
	})
}

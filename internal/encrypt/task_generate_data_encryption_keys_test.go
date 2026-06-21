package encrypt

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
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

func setupGenerateTest(t *testing.T, keyDataFactory func() *sconfig.KeyData) mockKMSGenerateTestEnv {
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
	globalDEK, globalDEKBytes := createDataEncryptionKeyForTest(t, ctx, db, globalEncryptionKeyID, globalKD)

	keyData := keyDataFactory()

	ekID := apid.New(apid.PrefixKey)
	namespace := "root.kms"
	require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{
		Path: namespace,
	}))

	require.NoError(t, db.CreateKey(ctx, &database.Key{
		Id:               ekID,
		Namespace:        namespace,
		State:            database.KeyStateActive,
		EncryptedKeyData: encryptKeyDataForTest(t, globalDEK.Id, globalDEKBytes, keyData),
	}))
	_, err := db.SetNamespaceKeyId(ctx, namespace, &ekID)
	require.NoError(t, err)

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

func setupMockKMSGenerateTest(t *testing.T, kmsVersions bool) mockKMSGenerateTestEnv {
	t.Helper()

	return setupGenerateTest(t, func() *sconfig.KeyData {
		kmsKeyData := sconfig.NewKeyDataMockKMS("namespace-kms")
		if kmsVersions {
			sconfig.KeyDataMockKMSAddVersion("namespace-kms", "mock-kms-key", "v1", util.MustGenerateSecureRandomKey(32))
		}
		return kmsKeyData
	})
}

func setupMockSecretGenerateTest(t *testing.T, keyBytes []byte) mockKMSGenerateTestEnv {
	t.Helper()

	return setupGenerateTest(t, func() *sconfig.KeyData {
		keyData := sconfig.NewKeyDataMock("namespace-secret")
		sconfig.KeyDataMockAddVersion("namespace-secret", "mock-secret-key", "v1", keyBytes)
		return keyData
	})
}

func TestGenerateDataEncryptionKeysToDatabase(t *testing.T) {
	t.Run("repairs root namespace key", func(t *testing.T) {
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		ctx := context.Background()
		globalKD := sconfig.NewKeyDataMock("global-root-repair")
		sconfig.KeyDataMockAddVersion("global-root-repair", "global-key", "v1", util.MustGenerateSecureRandomKey(32))
		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		cfg, db, rawDb := database.MustApplyBlankTestDbConfigRaw(t, cfg)

		_, err := rawDb.Exec("UPDATE namespaces SET key_id = NULL WHERE path = 'root'")
		require.NoError(t, err)

		root, err := db.GetNamespace(ctx, sconfig.RootNamespace)
		require.NoError(t, err)
		require.Nil(t, root.KeyId)

		require.NoError(t, generateDataEncryptionKeysToDatabase(ctx, cfg, db, slog.Default(), nil))

		root, err = db.GetNamespace(ctx, sconfig.RootNamespace)
		require.NoError(t, err)
		require.NotNil(t, root.KeyId)
		require.Equal(t, database.GlobalKeyID, *root.KeyId)
	})

	t.Run("preserves existing root namespace key", func(t *testing.T) {
		sconfig.ResetKeyDataMockRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockRegistry)

		ctx := context.Background()
		globalKD := sconfig.NewKeyDataMock("global-root-preserve")
		sconfig.KeyDataMockAddVersion("global-root-preserve", "global-key", "v1", util.MustGenerateSecureRandomKey(32))
		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)

		rootKeyID := apid.New(apid.PrefixKey)
		require.NoError(t, db.CreateKey(ctx, &database.Key{
			Id:        rootKeyID,
			Namespace: sconfig.RootNamespace,
		}))
		_, err := db.SetNamespaceKeyId(ctx, sconfig.RootNamespace, &rootKeyID)
		require.NoError(t, err)

		require.NoError(t, generateDataEncryptionKeysToDatabase(ctx, cfg, db, slog.Default(), nil))

		root, err := db.GetNamespace(ctx, sconfig.RootNamespace)
		require.NoError(t, err)
		require.NotNil(t, root.KeyId)
		require.Equal(t, rootKeyID, *root.KeyId)
	})

	t.Run("returns global provider errors without creating partial dek", func(t *testing.T) {
		sconfig.ResetKeyDataMockKMSRegistry()
		t.Cleanup(sconfig.ResetKeyDataMockKMSRegistry)

		ctx := context.Background()
		globalKD := sconfig.NewKeyDataMockKMS("global-kms-error")
		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: globalKD,
			},
		})
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)

		err := generateDataEncryptionKeysToDatabase(ctx, cfg, db, slog.Default(), nil)
		require.ErrorContains(t, err, "failed to reconcile data encryption key for global key")
		require.ErrorContains(t, err, "no current version")

		globalDEKs, listErr := db.ListDataEncryptionKeysForKey(ctx, globalEncryptionKeyID)
		require.NoError(t, listErr)
		require.Empty(t, globalDEKs)
	})

	t.Run("creates first dek without changing wrapping sync state", func(t *testing.T) {
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

		globalDEKs, err := env.db.ListDataEncryptionKeysForKey(env.ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, globalDEKs, 1)
		require.True(t, globalDEKs[0].IsCurrent)
		require.Equal(t, string(sconfig.ProviderTypeMock), globalDEKs[0].Provider)
		require.Equal(t, sconfig.KeyVersionProtectedDataTypeAuthProxyAESGCM, globalDEKs[0].ProtectedData.Type)

		enc := newTestService(env.cfg, env.db)
		encrypted, err := enc.EncryptStringForNamespace(env.ctx, env.namespace, "generated dek")
		require.NoError(t, err)
		require.Equal(t, deks[0].Id, encrypted.ID)
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

		globalDEKs, err := env.db.ListDataEncryptionKeysForKey(env.ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, globalDEKs, 1)
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

		globalDEKs, err := env.db.ListDataEncryptionKeysForKey(env.ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, globalDEKs, 2)
		require.False(t, globalDEKs[0].IsCurrent)
		require.True(t, globalDEKs[1].IsCurrent)
		require.Equal(t, string(sconfig.ProviderTypeMock), globalDEKs[1].Provider)
		require.Equal(t, "v1", globalDEKs[1].ProviderVersion)
	})

	t.Run("does not create first dek when ensure current is disabled", func(t *testing.T) {
		env := setupMockKMSGenerateTest(t, true)
		env.cfg.GetRoot().SystemAuth.DataEncryptionKeys.EnsureCurrent = util.ToPtr(false)

		require.NoError(t, generateDataEncryptionKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))

		deks, err := env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Empty(t, deks)

		globalDEKs, err := env.db.ListDataEncryptionKeysForKey(env.ctx, globalEncryptionKeyID)
		require.NoError(t, err)
		require.Len(t, globalDEKs, 1)
	})

	t.Run("creates authproxy generated dek for secret-backed key", func(t *testing.T) {
		env := setupMockSecretGenerateTest(t, util.MustGenerateSecureRandomKey(32))

		require.NoError(t, generateDataEncryptionKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))

		deks, err := env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Len(t, deks, 1)
		require.True(t, deks[0].IsCurrent)
		require.Equal(t, string(sconfig.ProviderTypeMock), deks[0].Provider)
		require.Equal(t, "mock-secret-key", deks[0].ProviderID)
		require.Equal(t, "v1", deks[0].ProviderVersion)
		require.NotNil(t, deks[0].ProtectedData)
		require.Equal(t, sconfig.KeyVersionProtectedDataTypeAuthProxyAESGCM, deks[0].ProtectedData.Type)
		require.NotEmpty(t, deks[0].ProtectedData.WrappedData)

		require.NoError(t, generateDataEncryptionKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil))
		deks, err = env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, err)
		require.Len(t, deks, 1)
	})

	t.Run("logs provider errors without creating partial dek", func(t *testing.T) {
		env := setupMockKMSGenerateTest(t, false)

		err := generateDataEncryptionKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil)
		require.NoError(t, err)

		deks, listErr := env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, listErr)
		require.Empty(t, deks)

		globalDEKs, listErr := env.db.ListDataEncryptionKeysForKey(env.ctx, globalEncryptionKeyID)
		require.NoError(t, listErr)
		require.Len(t, globalDEKs, 1)
	})

	t.Run("records non-global provider errors as metrics", func(t *testing.T) {
		env := setupMockKMSGenerateTest(t, false)
		reader := sdkmetric.NewManualReader()
		providers := &aptelemetry.Providers{
			Enabled:       true,
			MeterProvider: sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)),
		}
		tel, err := NewDataEncryptionKeyTelemetry(providers, telemetryMetricsEnabledConfig())
		require.NoError(t, err)

		err = generateDataEncryptionKeysToDatabase(
			env.ctx,
			env.cfg,
			env.db,
			env.logger,
			nil,
			WithGenerateDataEncryptionKeysTelemetry(tel),
		)
		require.NoError(t, err)

		rm := metricdata.ResourceMetrics{}
		require.NoError(t, reader.Collect(env.ctx, &rm))
		dp := requireInt64SumDataPoint(t, rm, metricDataEncryptionKeyGenerationFailures)
		require.Equal(t, int64(1), dp.Value)
		attrs := metricAttrMap(dp.Attributes.ToSlice())
		require.Equal(t, dekGenerationFailureReasonReconcile, attrs[attrDEKGenerationFailureReason])
		require.Equal(t, dekGenerationKeyScopeNonGlobal, attrs[attrDEKGenerationKeyScope])
		require.Equal(t, string(database.KeyUsageDataEncryption), attrs[attrKeyUsage])
		require.Equal(t, string(database.KeyMaterialTypeSymmetric), attrs[attrKeyMaterialType])
		require.Equal(t, string(database.KeyStateActive), attrs[attrKeyState])
		require.Equal(t, string(sconfig.ProviderTypeMockKMS), attrs[attrKeyProviderType])
	})

	t.Run("logs fallback provider errors without creating partial dek", func(t *testing.T) {
		env := setupMockSecretGenerateTest(t, []byte("bad"))

		err := generateDataEncryptionKeysToDatabase(env.ctx, env.cfg, env.db, env.logger, nil)
		require.NoError(t, err)

		deks, listErr := env.db.ListDataEncryptionKeysForKey(env.ctx, env.ekID)
		require.NoError(t, listErr)
		require.Empty(t, deks)
	})
}

func telemetryMetricsEnabledConfig() *sconfig.Telemetry {
	enabled := true
	return &sconfig.Telemetry{
		Enabled: &enabled,
		Signals: &sconfig.TelemetrySignals{
			Metrics: &enabled,
		},
	}
}

func requireInt64SumDataPoint(
	t *testing.T,
	rm metricdata.ResourceMetrics,
	name string,
) metricdata.DataPoint[int64] {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "metric %q should be int64 sum, got %T", name, m.Data)
			require.Len(t, sum.DataPoints, 1)
			return sum.DataPoints[0]
		}
	}

	var names []string
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			names = append(names, m.Name)
		}
	}
	t.Fatalf("metric %q not emitted; got: %v", name, names)
	return metricdata.DataPoint[int64]{}
}

func metricAttrMap(kvs []attribute.KeyValue) map[string]any {
	out := make(map[string]any, len(kvs))
	for _, kv := range kvs {
		out[string(kv.Key)] = kv.Value.AsInterface()
	}
	return out
}

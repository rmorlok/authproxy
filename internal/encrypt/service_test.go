package encrypt

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// newTestService creates a service and immediately syncs keys for testing.
func newTestService(cfg config.C, db database.DB) E {
	svc := NewEncryptService(cfg, db, slog.Default())
	if s, ok := svc.(*service); ok {
		s.startForTest()
	}
	return svc
}

func TestService(t *testing.T) {
	someBytes := []byte("some bytes")
	someString := "some string"

	t.Run("bad configuration", func(t *testing.T) {
		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: nil,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)
		s := newTestService(nil, db)
		_, err := s.EncryptGlobal(context.Background(), someBytes)
		require.Error(t, err)

		cfg = config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKey: &sconfig.KeyData{
					InnerVal: &sconfig.KeyDataEnvVar{
						EnvVar: "DOES_NOT_EXIST",
					},
				},
			},
		})
		_, db = database.MustApplyBlankTestDbConfig(t, cfg)
		s = newTestService(cfg, db)
		_, err = s.EncryptGlobal(context.Background(), someBytes)
		require.Error(t, err)
	})

	cfg := config.FromRoot(&sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			GlobalAESKey: sconfig.NewKeyDataRandomBytes(),
		},
	})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	s := newTestService(cfg, db)

	connection := database.Connection{
		Id:               apid.New(apid.PrefixConnection),
		Namespace:        "root.some-namespace",
		ConnectorId:      apid.New(apid.PrefixConnectorVersion),
		ConnectorVersion: 1,
		State:            database.ConnectionStateConfigured,
	}
	require.NoError(t, db.CreateConnection(context.Background(), &connection))

	connectorVersion := database.ConnectorVersion{
		Id:                  apid.New(apid.PrefixConnectorVersion),
		Version:             1,
		Namespace:           "root.some-namespace",
		State:               database.ConnectorVersionStatePrimary,
		Labels:              map[string]string{"type": "test"},
		Hash:                "test",
		EncryptedDefinition: encfield.EncryptedField{ID: "dek_test", Data: "test"},
	}
	require.NoError(t, db.UpsertConnectorVersion(context.Background(), &connectorVersion))

	t.Run("string", func(t *testing.T) {
		t.Run("roundtrip global", func(t *testing.T) {
			encrypted, err := s.EncryptStringGlobal(context.Background(), someString)
			require.NoError(t, err)
			require.False(t, encrypted.IsZero())
			require.True(t, encrypted.ID.HasPrefix(apid.PrefixDataEncryptionKey))

			decrypted, err := s.DecryptString(context.Background(), encrypted)
			require.NoError(t, err)
			require.Equal(t, someString, decrypted)
		})
		t.Run("roundtrip connection", func(t *testing.T) {
			encrypted, err := s.EncryptStringForEntity(context.Background(), &connection, someString)
			require.NoError(t, err)
			require.False(t, encrypted.IsZero())
			require.True(t, encrypted.ID.HasPrefix(apid.PrefixDataEncryptionKey))

			decrypted, err := s.DecryptString(context.Background(), encrypted)
			require.NoError(t, err)
			require.Equal(t, someString, decrypted)
		})
		t.Run("roundtrip connector", func(t *testing.T) {
			encrypted, err := s.EncryptStringForEntity(context.Background(), &connectorVersion, someString)
			require.NoError(t, err)
			require.False(t, encrypted.IsZero())
			require.True(t, encrypted.ID.HasPrefix(apid.PrefixDataEncryptionKey))

			decrypted, err := s.DecryptString(context.Background(), encrypted)
			require.NoError(t, err)
			require.Equal(t, someString, decrypted)
		})
		t.Run("roundtrip namespace", func(t *testing.T) {
			encrypted, err := s.EncryptStringForNamespace(context.Background(), connection.Namespace, someString)
			require.NoError(t, err)
			require.False(t, encrypted.IsZero())
			require.True(t, encrypted.ID.HasPrefix(apid.PrefixDataEncryptionKey))

			decrypted, err := s.DecryptString(context.Background(), encrypted)
			require.NoError(t, err)
			require.Equal(t, someString, decrypted)
		})
	})

	t.Run("bytes", func(t *testing.T) {
		t.Run("roundtrip global", func(t *testing.T) {
			encryptedBytes, err := s.EncryptGlobal(context.Background(), someBytes)
			require.NoError(t, err)
			require.NotEmpty(t, encryptedBytes)
			require.NotEqual(t, someBytes, encryptedBytes)

			decrypted, err := s.Decrypt(context.Background(), encryptedBytes)
			require.NoError(t, err)
			require.Equal(t, someBytes, decrypted)
		})
		t.Run("roundtrip connection", func(t *testing.T) {
			encryptedBytes, err := s.EncryptForEntity(context.Background(), &connection, someBytes)
			require.NoError(t, err)
			require.NotEmpty(t, encryptedBytes)
			require.NotEqual(t, someBytes, encryptedBytes)

			decrypted, err := s.Decrypt(context.Background(), encryptedBytes)
			require.NoError(t, err)
			require.Equal(t, someBytes, decrypted)
		})
		t.Run("roundtrip connector", func(t *testing.T) {
			encryptedBytes, err := s.EncryptForEntity(context.Background(), &connectorVersion, someBytes)
			require.NoError(t, err)
			require.NotEmpty(t, encryptedBytes)
			require.NotEqual(t, someBytes, encryptedBytes)

			decrypted, err := s.Decrypt(context.Background(), encryptedBytes)
			require.NoError(t, err)
			require.Equal(t, someBytes, decrypted)
		})
		t.Run("roundtrip namespace", func(t *testing.T) {
			encryptedBytes, err := s.EncryptForNamespace(context.Background(), connection.Namespace, someBytes)
			require.NoError(t, err)
			require.NotEmpty(t, encryptedBytes)
			require.NotEqual(t, someBytes, encryptedBytes)

			decrypted, err := s.Decrypt(context.Background(), encryptedBytes)
			require.NoError(t, err)
			require.Equal(t, someBytes, decrypted)
		})
		t.Run("zero field decrypts to zero values", func(t *testing.T) {
			decryptedBytes, err := s.Decrypt(context.Background(), encfield.EncryptedField{})
			require.NoError(t, err)
			require.Nil(t, decryptedBytes)

			decryptedString, err := s.DecryptString(context.Background(), encfield.EncryptedField{})
			require.NoError(t, err)
			require.Empty(t, decryptedString)
		})
	})
}

func TestNewTestEncryptServiceDefaultsGlobalKey(t *testing.T) {
	ctx := context.Background()

	t.Run("existing config", func(t *testing.T) {
		cfg := config.FromRoot(&sconfig.Root{})
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)

		cfg, s := NewTestEncryptService(cfg, db)
		require.NotNil(t, cfg.GetRoot().SystemAuth.GlobalAESKey)

		encrypted, err := s.EncryptStringGlobal(ctx, "generated default")
		require.NoError(t, err)
		require.False(t, encrypted.IsZero())

		decrypted, err := s.DecryptString(ctx, encrypted)
		require.NoError(t, err)
		require.Equal(t, "generated default", decrypted)
	})

	t.Run("nil config", func(t *testing.T) {
		cfg := config.FromRoot(&sconfig.Root{})
		_, db := database.MustApplyBlankTestDbConfig(t, cfg)

		cfg, s := NewTestEncryptService(nil, db)
		require.NotNil(t, cfg.GetRoot().SystemAuth.GlobalAESKey)

		encrypted, err := s.EncryptStringGlobal(ctx, "generated from nil config")
		require.NoError(t, err)
		require.False(t, encrypted.IsZero())

		decrypted, err := s.DecryptString(ctx, encrypted)
		require.NoError(t, err)
		require.Equal(t, "generated from nil config", decrypted)
	})
}

func TestServiceStartSyncLoop(t *testing.T) {
	sconfig.ResetKeyDataMockRegistry()
	t.Cleanup(sconfig.ResetKeyDataMockRegistry)

	ctx := context.Background()
	globalKD := sconfig.NewKeyDataMock("service-start-global")
	sconfig.KeyDataMockAddVersion("service-start-global", "global-key", "v1", util.MustGenerateSecureRandomKey(32))

	cfg := config.FromRoot(&sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			GlobalAESKey: globalKD,
		},
	})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	createDataEncryptionKeyForTest(t, ctx, db, globalEncryptionKeyID, globalKD)

	s := NewEncryptService(cfg, db, slog.Default())
	s.Start()
	t.Cleanup(s.Shutdown)

	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	encrypted, err := s.EncryptStringGlobal(syncCtx, "from background sync")
	require.NoError(t, err)

	decrypted, err := s.DecryptString(syncCtx, encrypted)
	require.NoError(t, err)
	require.Equal(t, "from background sync", decrypted)
}

func TestServiceSyncKeysFromDbToMemoryRefreshesNamespaceKey(t *testing.T) {
	sconfig.ResetKeyDataMockRegistry()
	t.Cleanup(sconfig.ResetKeyDataMockRegistry)

	ctx := context.Background()
	globalKD := sconfig.NewKeyDataMock("service-sync-global")
	sconfig.KeyDataMockAddVersion("service-sync-global", "global-key", "v1", util.MustGenerateSecureRandomKey(32))

	cfg := config.FromRoot(&sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			GlobalAESKey: globalKD,
		},
	})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	cfg, s := NewTestEncryptService(cfg, db)

	globalDEK, err := db.GetCurrentDataEncryptionKeyForKey(ctx, globalEncryptionKeyID)
	require.NoError(t, err)
	globalDEKBytes, err := globalKD.UnwrapDataEncryptionKey(ctx, dataEncryptionKeyInfos([]*database.DataEncryptionKey{globalDEK})[0])
	require.NoError(t, err)

	namespacePath := "root.service-sync"
	require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{Path: namespacePath}))

	keyData := sconfig.NewKeyDataMock("service-sync-namespace")
	sconfig.KeyDataMockAddVersion("service-sync-namespace", "namespace-key", "v1", util.MustGenerateSecureRandomKey(32))
	keyID := apid.New(apid.PrefixKey)
	require.NoError(t, db.CreateKey(ctx, &database.Key{
		Id:               keyID,
		Namespace:        namespacePath,
		EncryptedKeyData: encryptKeyDataForTest(t, globalDEK.Id, globalDEKBytes, keyData),
	}))
	_, err = db.SetNamespaceKeyId(ctx, namespacePath, &keyID)
	require.NoError(t, err)
	namespaceDEK, _ := createDataEncryptionKeyForTest(t, ctx, db, keyID, keyData)

	beforeRefresh, err := s.EncryptStringForNamespace(ctx, namespacePath, "namespace data")
	require.NoError(t, err)
	require.Equal(t, globalDEK.Id, beforeRefresh.ID)

	require.NoError(t, s.SyncKeysFromDbToMemory(ctx))

	afterRefresh, err := s.EncryptStringForNamespace(ctx, namespacePath, "namespace data")
	require.NoError(t, err)
	require.Equal(t, namespaceDEK.Id, afterRefresh.ID)

	decrypted, err := s.DecryptString(ctx, afterRefresh)
	require.NoError(t, err)
	require.Equal(t, "namespace data", decrypted)
}

func TestServiceEncryptKeyForNamespace(t *testing.T) {
	env := setupServiceKeyNamespaceTest(t)

	rootKeyMaterial, err := env.enc.EncryptKeyForNamespace(env.ctx, "root", []byte("root key data"))
	require.NoError(t, err)
	require.Equal(t, env.globalDEKID, rootKeyMaterial.ID)

	childData, err := env.enc.EncryptStringForNamespace(env.ctx, env.childNamespace, "child namespace data")
	require.NoError(t, err)
	require.Equal(t, env.childDEKID, childData.ID)

	childKeyMaterial, err := env.enc.EncryptKeyForNamespace(env.ctx, env.childNamespace, []byte("child key data"))
	require.NoError(t, err)
	require.Equal(t, env.parentDEKID, childKeyMaterial.ID)

	decrypted, err := env.enc.Decrypt(env.ctx, childKeyMaterial)
	require.NoError(t, err)
	require.Equal(t, []byte("child key data"), decrypted)
}

func TestServiceReEncryptField(t *testing.T) {
	sconfig.ResetKeyDataMockRegistry()
	t.Cleanup(sconfig.ResetKeyDataMockRegistry)

	ctx := context.Background()
	globalKD := sconfig.NewKeyDataMock("service-reencrypt-global")
	sconfig.KeyDataMockAddVersion("service-reencrypt-global", "global-key", "v1", util.MustGenerateSecureRandomKey(32))

	cfg := config.FromRoot(&sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			GlobalAESKey: globalKD,
		},
	})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	cfg, s := NewTestEncryptService(cfg, db)

	encryptedV1, err := s.EncryptStringGlobal(ctx, "rotated data")
	require.NoError(t, err)

	same, err := s.ReEncryptField(ctx, encryptedV1, encryptedV1.ID)
	require.NoError(t, err)
	require.Equal(t, encryptedV1, same)

	sconfig.KeyDataMockAddVersion("service-reencrypt-global", "global-key", "v2", util.MustGenerateSecureRandomKey(32))
	dekV2, _ := createDataEncryptionKeyForTest(t, ctx, db, globalEncryptionKeyID, cfg.GetRoot().SystemAuth.GlobalAESKey)
	require.NoError(t, s.SyncKeysFromDbToMemory(ctx))

	encryptedV2, err := s.ReEncryptField(ctx, encryptedV1, dekV2.Id)
	require.NoError(t, err)
	require.Equal(t, dekV2.Id, encryptedV2.ID)
	require.NotEqual(t, encryptedV1.Data, encryptedV2.Data)

	decrypted, err := s.DecryptString(ctx, encryptedV2)
	require.NoError(t, err)
	require.Equal(t, "rotated data", decrypted)
}

func TestServiceCachedKeyBytesAreCopied(t *testing.T) {
	sconfig.ResetKeyDataMockRegistry()
	t.Cleanup(sconfig.ResetKeyDataMockRegistry)

	ctx := context.Background()
	globalKD := sconfig.NewKeyDataMock("service-cache-global")
	sconfig.KeyDataMockAddVersion("service-cache-global", "global-key", "v1", util.MustGenerateSecureRandomKey(32))

	cfg := config.FromRoot(&sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			GlobalAESKey: globalKD,
		},
	})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	_, enc := NewTestEncryptService(cfg, db)
	svc := enc.(*service)

	globalDEK, err := db.GetCurrentDataEncryptionKeyForKey(ctx, globalEncryptionKeyID)
	require.NoError(t, err)

	keyBytes, err := svc.getDataEncryptionKeyBytes(globalDEK.Id)
	require.NoError(t, err)
	require.NotEmpty(t, keyBytes)
	original := append([]byte(nil), keyBytes...)

	keyBytes[0] ^= 0xff
	keyBytesAgain, err := svc.getDataEncryptionKeyBytes(globalDEK.Id)
	require.NoError(t, err)
	require.Equal(t, original, keyBytesAgain)

	allKeyBytes := svc.getAllKeyBytes()
	require.NotEmpty(t, allKeyBytes)
	allKeyBytes[0][0] ^= 0xff

	keyBytesAfterAll, err := svc.getDataEncryptionKeyBytes(globalDEK.Id)
	require.NoError(t, err)
	require.Equal(t, original, keyBytesAfterAll)
}

type serviceKeyNamespaceTestEnv struct {
	ctx             context.Context
	enc             E
	globalDEKID     apid.ID
	parentNamespace string
	parentDEKID     apid.ID
	childNamespace  string
	childDEKID      apid.ID
}

func setupServiceKeyNamespaceTest(t *testing.T) serviceKeyNamespaceTestEnv {
	t.Helper()

	sconfig.ResetKeyDataMockRegistry()
	t.Cleanup(sconfig.ResetKeyDataMockRegistry)

	ctx := context.Background()
	globalKD := sconfig.NewKeyDataMock("service-key-global")
	sconfig.KeyDataMockAddVersion("service-key-global", "global-key", "v1", util.MustGenerateSecureRandomKey(32))

	cfg := config.FromRoot(&sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			GlobalAESKey: globalKD,
		},
	})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	globalDEK, globalDEKBytes := createDataEncryptionKeyForTest(t, ctx, db, globalEncryptionKeyID, globalKD)

	parentNamespace := "root.service-key-parent"
	childNamespace := parentNamespace + ".child"
	require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{Path: parentNamespace}))
	require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{Path: childNamespace}))

	parentKeyData := sconfig.NewKeyDataMock("service-key-parent")
	sconfig.KeyDataMockAddVersion("service-key-parent", "parent-key", "v1", util.MustGenerateSecureRandomKey(32))
	parentKeyID := apid.New(apid.PrefixKey)
	require.NoError(t, db.CreateKey(ctx, &database.Key{
		Id:               parentKeyID,
		Namespace:        parentNamespace,
		EncryptedKeyData: encryptKeyDataForTest(t, globalDEK.Id, globalDEKBytes, parentKeyData),
	}))
	_, err := db.SetNamespaceKeyId(ctx, parentNamespace, &parentKeyID)
	require.NoError(t, err)
	parentDEK, parentDEKBytes := createDataEncryptionKeyForTest(t, ctx, db, parentKeyID, parentKeyData)

	childKeyData := sconfig.NewKeyDataMock("service-key-child")
	sconfig.KeyDataMockAddVersion("service-key-child", "child-key", "v1", util.MustGenerateSecureRandomKey(32))
	childKeyID := apid.New(apid.PrefixKey)
	require.NoError(t, db.CreateKey(ctx, &database.Key{
		Id:               childKeyID,
		Namespace:        childNamespace,
		EncryptedKeyData: encryptKeyDataForTest(t, parentDEK.Id, parentDEKBytes, childKeyData),
	}))
	_, err = db.SetNamespaceKeyId(ctx, childNamespace, &childKeyID)
	require.NoError(t, err)
	childDEK, _ := createDataEncryptionKeyForTest(t, ctx, db, childKeyID, childKeyData)

	_, enc := NewTestEncryptService(cfg, db)

	return serviceKeyNamespaceTestEnv{
		ctx:             ctx,
		enc:             enc,
		globalDEKID:     globalDEK.Id,
		parentNamespace: parentNamespace,
		parentDEKID:     parentDEK.Id,
		childNamespace:  childNamespace,
		childDEKID:      childDEK.Id,
	}
}

package encrypt

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

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
		s := NewEncryptService(nil, db)
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
		s = NewEncryptService(cfg, db)
		_, err = s.EncryptGlobal(context.Background(), someBytes)
		require.Error(t, err)
	})

	cfg := config.FromRoot(&sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			GlobalAESKey: sconfig.NewKeyDataRandomBytes(),
		},
	})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	s := NewEncryptService(cfg, db)

	connection := database.Connection{
		Id:               apid.New(apid.PrefixConnection),
		Namespace:        "root.some-namespace",
		ConnectorId:      apid.New(apid.PrefixConnectorVersion),
		ConnectorVersion: 1,
		State:            database.ConnectionStateReady,
	}
	require.NoError(t, db.CreateConnection(context.Background(), &connection))

	connectorVersion := database.ConnectorVersion{
		Id:                  apid.New(apid.PrefixConnectorVersion),
		Version:             1,
		Namespace:           "root.some-namespace",
		State:               database.ConnectorVersionStatePrimary,
		Labels:              map[string]string{"type": "test"},
		Hash:                "test",
		EncryptedDefinition: "test",
	}
	require.NoError(t, db.UpsertConnectorVersion(context.Background(), &connectorVersion))

	t.Run("string", func(t *testing.T) {
		t.Run("roundtrip global", func(t *testing.T) {
			encryptedBase64, err := s.EncryptStringGlobal(context.Background(), someString)
			require.NoError(t, err)
			require.NotEmpty(t, encryptedBase64)
			require.NotEqual(t, someString, encryptedBase64)

			decrypted, err := s.DecryptStringGlobal(context.Background(), encryptedBase64)
			require.NoError(t, err)
			require.Equal(t, someString, decrypted)
		})
		t.Run("roundtrip connection", func(t *testing.T) {
			encryptedBase64, err := s.EncryptStringForConnection(context.Background(), &connection, someString)
			require.NoError(t, err)
			require.NotEmpty(t, encryptedBase64)
			require.NotEqual(t, someString, encryptedBase64)

			decrypted, err := s.DecryptStringForConnection(context.Background(), &connection, encryptedBase64)
			require.NoError(t, err)
			require.Equal(t, someString, decrypted)
		})
		t.Run("roundtrip connector", func(t *testing.T) {
			encryptedBase64, err := s.EncryptStringForConnector(context.Background(), &connectorVersion, someString)
			require.NoError(t, err)
			require.NotEmpty(t, encryptedBase64)
			require.NotEqual(t, someString, encryptedBase64)

			decrypted, err := s.DecryptStringForConnector(context.Background(), &connectorVersion, encryptedBase64)
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

			decrypted, err := s.DecryptGlobal(context.Background(), encryptedBytes)
			require.NoError(t, err)
			require.Equal(t, someBytes, decrypted)
		})
		t.Run("roundtrip connection", func(t *testing.T) {
			encryptedBytes, err := s.EncryptForConnection(context.Background(), &connection, someBytes)
			require.NoError(t, err)
			require.NotEmpty(t, encryptedBytes)
			require.NotEqual(t, someBytes, encryptedBytes)

			decrypted, err := s.DecryptForConnection(context.Background(), &connection, encryptedBytes)
			require.NoError(t, err)
			require.Equal(t, someBytes, decrypted)
		})
		t.Run("roundtrip connector", func(t *testing.T) {
			encryptedBytes, err := s.EncryptForConnector(context.Background(), &connectorVersion, someBytes)
			require.NoError(t, err)
			require.NotEmpty(t, encryptedBytes)
			require.NotEqual(t, someBytes, encryptedBytes)

			decrypted, err := s.DecryptForConnector(context.Background(), &connectorVersion, encryptedBytes)
			require.NoError(t, err)
			require.Equal(t, someBytes, decrypted)
		})
	})

	t.Run("versioned string prefix", func(t *testing.T) {
		encryptedBase64, err := s.EncryptStringGlobal(context.Background(), someString)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(encryptedBase64, string(apid.PrefixEncryptionKeyVersion)), "expected ekv_ prefix")
		require.True(t, s.IsEncryptedWithCurrentKey(encryptedBase64))
	})

	t.Run("IsEncryptedWithCurrentKey", func(t *testing.T) {
		// Encrypt something to ensure keys are synced
		encrypted, err := s.EncryptStringGlobal(context.Background(), someString)
		require.NoError(t, err)
		require.True(t, s.IsEncryptedWithCurrentKey(encrypted))
		require.False(t, s.IsEncryptedWithCurrentKey("somelegacydata"))
		require.False(t, s.IsEncryptedWithCurrentKey("v1:0:somedata"))
	})
}

func TestServiceMultiKey(t *testing.T) {
	someString := "hello multi-key"
	someBytes := []byte("hello bytes multi-key")

	key1 := sconfig.NewKeyDataRandomBytes()
	key2 := sconfig.NewKeyDataRandomBytes()

	t.Run("encrypt with primary, decrypt with both keys available", func(t *testing.T) {
		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKeys: []*sconfig.KeyData{key1, key2},
			},
		})
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
		s := NewEncryptService(cfg, db)

		encrypted, err := s.EncryptStringGlobal(context.Background(), someString)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(encrypted, string(apid.PrefixEncryptionKeyVersion)))

		decrypted, err := s.DecryptStringGlobal(context.Background(), encrypted)
		require.NoError(t, err)
		require.Equal(t, someString, decrypted)
	})

	t.Run("decrypt old key data after rotation with same db", func(t *testing.T) {
		// First, encrypt with key1 as primary
		cfg1 := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKeys: []*sconfig.KeyData{key1},
			},
		})
		cfg1, db1 := database.MustApplyBlankTestDbConfig(t, cfg1)
		s1 := NewEncryptService(cfg1, db1)

		encrypted, err := s1.EncryptStringGlobal(context.Background(), someString)
		require.NoError(t, err)

		// Now rotate: key2 is primary, key1 is secondary, same database
		cfg2 := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKeys: []*sconfig.KeyData{key2, key1},
			},
		})
		cfg2.GetRoot().Database = cfg1.GetRoot().Database
		s2 := NewEncryptService(cfg2, db1)

		// Should succeed: the encrypted data has ekv_id that maps to key1 in the same db
		decrypted, err := s2.DecryptStringGlobal(context.Background(), encrypted)
		require.NoError(t, err)
		require.Equal(t, someString, decrypted)

		// After rotation, IsEncryptedWithCurrentKey should return false because current is now key2
		require.False(t, s2.IsEncryptedWithCurrentKey(encrypted))
	})

	t.Run("legacy format decryption tries all keys", func(t *testing.T) {
		// Encrypt with key1 directly (simulating legacy format)
		ctx := context.Background()
		key1Ver, err := key1.GetCurrentVersion(ctx)
		require.NoError(t, err)

		encryptedBytes, err := encryptWithKey(key1Ver.Data, []byte(someString))
		require.NoError(t, err)
		legacyEncoded := base64.StdEncoding.EncodeToString(encryptedBytes)

		// Create service with key2 as primary, key1 as secondary
		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKeys: []*sconfig.KeyData{key2, key1},
			},
		})
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
		s := NewEncryptService(cfg, db)

		// Legacy format (no prefix) should try all keys and find key1
		decrypted, err := s.DecryptStringGlobal(ctx, legacyEncoded)
		require.NoError(t, err)
		require.Equal(t, someString, decrypted)

		// It's not encrypted with current key (no versioned prefix)
		require.False(t, s.IsEncryptedWithCurrentKey(legacyEncoded))
	})

	t.Run("bytes multi-key roundtrip", func(t *testing.T) {
		// Encrypt with key1
		cfg1 := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKeys: []*sconfig.KeyData{key1},
			},
		})
		cfg1, db1 := database.MustApplyBlankTestDbConfig(t, cfg1)
		s1 := NewEncryptService(cfg1, db1)

		encrypted, err := s1.EncryptGlobal(context.Background(), someBytes)
		require.NoError(t, err)

		// Decrypt with key2 primary, key1 secondary (raw bytes try all keys), same db
		cfg2 := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				GlobalAESKeys: []*sconfig.KeyData{key2, key1},
			},
		})
		cfg2.GetRoot().Database = cfg1.GetRoot().Database
		s2 := NewEncryptService(cfg2, db1)

		decrypted, err := s2.DecryptGlobal(context.Background(), encrypted)
		require.NoError(t, err)
		require.Equal(t, someBytes, decrypted)
	})
}

func TestValidateGlobalAESKeys(t *testing.T) {
	t.Run("both set is error", func(t *testing.T) {
		sa := &sconfig.SystemAuth{
			GlobalAESKey:  sconfig.NewKeyDataRandomBytes(),
			GlobalAESKeys: []*sconfig.KeyData{sconfig.NewKeyDataRandomBytes()},
		}
		require.Error(t, sa.ValidateGlobalAESKeys())
	})

	t.Run("only key set is ok", func(t *testing.T) {
		sa := &sconfig.SystemAuth{
			GlobalAESKey: sconfig.NewKeyDataRandomBytes(),
		}
		require.NoError(t, sa.ValidateGlobalAESKeys())
	})

	t.Run("only keys set is ok", func(t *testing.T) {
		sa := &sconfig.SystemAuth{
			GlobalAESKeys: []*sconfig.KeyData{sconfig.NewKeyDataRandomBytes()},
		}
		require.NoError(t, sa.ValidateGlobalAESKeys())
	})

	t.Run("neither set is ok", func(t *testing.T) {
		sa := &sconfig.SystemAuth{}
		require.NoError(t, sa.ValidateGlobalAESKeys())
	})
}

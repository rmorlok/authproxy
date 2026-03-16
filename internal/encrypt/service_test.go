package encrypt

import (
	"context"
	"log/slog"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
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
		EncryptedDefinition: encfield.EncryptedField{ID: "ekv_test", Data: "test"},
	}
	require.NoError(t, db.UpsertConnectorVersion(context.Background(), &connectorVersion))

	t.Run("string", func(t *testing.T) {
		t.Run("roundtrip global", func(t *testing.T) {
			encrypted, err := s.EncryptStringGlobal(context.Background(), someString)
			require.NoError(t, err)
			require.False(t, encrypted.IsZero())

			decrypted, err := s.DecryptString(context.Background(), encrypted)
			require.NoError(t, err)
			require.Equal(t, someString, decrypted)
		})
		t.Run("roundtrip connection", func(t *testing.T) {
			encrypted, err := s.EncryptStringForEntity(context.Background(), &connection, someString)
			require.NoError(t, err)
			require.False(t, encrypted.IsZero())

			decrypted, err := s.DecryptString(context.Background(), encrypted)
			require.NoError(t, err)
			require.Equal(t, someString, decrypted)
		})
		t.Run("roundtrip connector", func(t *testing.T) {
			encrypted, err := s.EncryptStringForEntity(context.Background(), &connectorVersion, someString)
			require.NoError(t, err)
			require.False(t, encrypted.IsZero())

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
	})
}

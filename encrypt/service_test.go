package encrypt

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestService(t *testing.T) {
	someBytes := []byte("some bytes")
	someString := "some string"

	t.Run("bad configuration", func(t *testing.T) {
		cfg := config.FromRoot(&config.Root{
			SystemAuth: config.SystemAuth{
				GlobalAESKey: nil,
			},
		})
		_, db := database.MustApplyBlankTestDbConfig(t.Name(), cfg)
		s := NewEncryptService(nil, db)
		_, err := s.EncryptGlobal(context.Background(), someBytes)
		require.Error(t, err)

		cfg = config.FromRoot(&config.Root{
			SystemAuth: config.SystemAuth{
				GlobalAESKey: &config.KeyDataEnvVar{
					EnvVar: "DOES_NOT_EXIST",
				},
			},
		})
		_, db = database.MustApplyBlankTestDbConfig(t.Name(), cfg)
		s = NewEncryptService(cfg, db)
		_, err = s.EncryptGlobal(context.Background(), someBytes)
		require.Error(t, err)
	})

	cfg := config.FromRoot(&config.Root{
		SystemAuth: config.SystemAuth{
			GlobalAESKey: &config.KeyDataRandomBytes{},
		},
	})
	cfg, db := database.MustApplyBlankTestDbConfig(t.Name(), cfg)
	s := NewEncryptService(cfg, db)

	connection := database.Connection{
		ID:    uuid.New(),
		State: database.ConnectionStateReady,
	}
	require.NoError(t, db.CreateConnection(context.Background(), &connection))

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
			encryptedBase64, err := s.EncryptStringForConnection(context.Background(), connection, someString)
			require.NoError(t, err)
			require.NotEmpty(t, encryptedBase64)
			require.NotEqual(t, someString, encryptedBase64)

			decrypted, err := s.DecryptStringForConnection(context.Background(), connection, encryptedBase64)
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
			encryptedBytes, err := s.EncryptForConnection(context.Background(), connection, someBytes)
			require.NoError(t, err)
			require.NotEmpty(t, encryptedBytes)
			require.NotEqual(t, someBytes, encryptedBytes)

			decrypted, err := s.DecryptForConnection(context.Background(), connection, encryptedBytes)
			require.NoError(t, err)
			require.Equal(t, someBytes, decrypted)
		})
	})
}

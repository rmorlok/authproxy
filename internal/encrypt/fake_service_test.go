package encrypt

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/require"
)

func TestFakeService(t *testing.T) {
	someBytes := []byte("some bytes")
	someString := "some string"
	for _, doBase64 := range []bool{true, false} {
		t.Run(fmt.Sprintf("doBase64=%v", doBase64), func(t *testing.T) {
			connectorVersion := database.ConnectorVersion{
				Id:                  uuid.New(),
				Version:             1,
				State:               database.ConnectorVersionStatePrimary,
				Type:                "test",
				Hash:                "test",
				EncryptedDefinition: "test",
			}

			cfg := config.FromRoot(&config.Root{
				SystemAuth: config.SystemAuth{
					GlobalAESKey: config.NewKeyDataRandomBytes(),
				},
				DevSettings: &config.DevSettings{
					FakeEncryption:           true,
					FakeEncryptionSkipBase64: !doBase64,
				},
			})
			cfg, db := database.MustApplyBlankTestDbConfig(t.Name(), cfg)
			s := NewEncryptService(cfg, db)

			connection := database.Connection{
				Id:               uuid.New(),
				Namespace:        "root.some-namespace",
				ConnectorId:      uuid.New(),
				ConnectorVersion: 1,
				State:            database.ConnectionStateReady,
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
		})

	}
}

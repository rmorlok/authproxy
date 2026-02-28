package encrypt

import (
	"context"
	"fmt"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestFakeService(t *testing.T) {
	someBytes := []byte("some bytes")
	someString := "some string"
	for _, doBase64 := range []bool{true, false} {
		t.Run(fmt.Sprintf("doBase64=%v", doBase64), func(t *testing.T) {
			connectorVersion := database.ConnectorVersion{
				Id:                  apid.New(apid.PrefixConnectorVersion),
				Version:             1,
				State:               database.ConnectorVersionStatePrimary,
				Labels:              map[string]string{"type": "test"},
				Hash:                "test",
				EncryptedDefinition: encfield.EncryptedField{ID: "ekv_test", Data: "test"},
			}

			cfg := config.FromRoot(&sconfig.Root{
				SystemAuth: sconfig.SystemAuth{
					GlobalAESKey: sconfig.NewKeyDataRandomBytes(),
				},
				DevSettings: &sconfig.DevSettings{
					Enabled:                  true,
					FakeEncryption:           true,
					FakeEncryptionSkipBase64: !doBase64,
				},
			})
			cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
			s := NewEncryptService(cfg, db, nil)

			connection := database.Connection{
				Id:               apid.New(apid.PrefixConnection),
				Namespace:        "root.some-namespace",
				ConnectorId:      apid.New(apid.PrefixConnectorVersion),
				ConnectorVersion: 1,
				State:            database.ConnectionStateReady,
			}
			require.NoError(t, db.CreateConnection(context.Background(), &connection))

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

					decrypted, err := s.Decrypt(context.Background(), encryptedBytes)
					require.NoError(t, err)
					require.Equal(t, someBytes, decrypted)
				})
				t.Run("roundtrip connection", func(t *testing.T) {
					encryptedBytes, err := s.EncryptForEntity(context.Background(), &connection, someBytes)
					require.NoError(t, err)
					require.NotEmpty(t, encryptedBytes)

					decrypted, err := s.Decrypt(context.Background(), encryptedBytes)
					require.NoError(t, err)
					require.Equal(t, someBytes, decrypted)
				})
				t.Run("roundtrip connector", func(t *testing.T) {
					encryptedBytes, err := s.EncryptForEntity(context.Background(), &connectorVersion, someBytes)
					require.NoError(t, err)
					require.NotEmpty(t, encryptedBytes)

					decrypted, err := s.Decrypt(context.Background(), encryptedBytes)
					require.NoError(t, err)
					require.Equal(t, someBytes, decrypted)
				})
			})
		})

	}
}

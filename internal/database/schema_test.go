package database

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyModelSchema(t *testing.T) {
	_, _, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)

	t.Run("keys table replaces encryption_keys", func(t *testing.T) {
		requireQueryable(t, rawDb, "SELECT id, namespace, usage, material_type, encrypted_key_data, state FROM keys LIMIT 0")

		_, err := rawDb.Query("SELECT id FROM encryption_keys LIMIT 0")
		require.Error(t, err)
	})

	t.Run("global key uses key prefix and target defaults", func(t *testing.T) {
		var id, usage, materialType, state string
		err := rawDb.QueryRow("SELECT id, usage, material_type, state FROM keys WHERE id = 'key_global'").
			Scan(&id, &usage, &materialType, &state)
		require.NoError(t, err)
		require.Equal(t, "key_global", id)
		require.Equal(t, string(KeyUsageDataEncryption), usage)
		require.Equal(t, string(KeyMaterialTypeSymmetric), materialType)
		require.Equal(t, string(KeyStateActive), state)
	})

	t.Run("namespaces use key and target DEK columns", func(t *testing.T) {
		requireQueryable(t, rawDb, "SELECT key_id, target_data_encryption_key_id, target_data_encryption_key_updated_at FROM namespaces LIMIT 0")
	})

	t.Run("root namespace uses global key", func(t *testing.T) {
		var keyID string
		err := rawDb.QueryRow("SELECT key_id FROM namespaces WHERE path = 'root'").Scan(&keyID)
		require.NoError(t, err)
		require.Equal(t, string(GlobalKeyID), keyID)
	})

	t.Run("data encryption keys reference keys", func(t *testing.T) {
		requireQueryable(t, rawDb, "SELECT key_id, provider_metadata, protected_data FROM data_encryption_keys LIMIT 0")
	})
}

func requireQueryable(t *testing.T, q queryer, query string) {
	t.Helper()
	rows, err := q.Query(query)
	require.NoError(t, err)
	require.NoError(t, rows.Close())
}

type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

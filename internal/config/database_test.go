package config

import (
	"github.com/rmorlok/authproxy/internal/config/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDatabase(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("sqlite", func(t *testing.T) {
			data := `
      provider: sqlite
      path: ./some/path.db
`
			db, err := UnmarshallYamlDatabaseString(data)
			assert.NoError(err)
			assert.Equal(&DatabaseSqlite{
				Provider: DatabaseProviderSqlite,
				Path:     "./some/path.db",
			}, db)
		})
	})

	t.Run("yaml gen", func(t *testing.T) {
		t.Run("oauth2", func(t *testing.T) {
			data := &DatabaseSqlite{
				Provider: DatabaseProviderSqlite,
				Path:     "./some/path.db",
			}
			assert.Equal(`provider: sqlite
path: ./some/path.db
`, common.MustMarshalToYamlString(data))
		})
	})
}

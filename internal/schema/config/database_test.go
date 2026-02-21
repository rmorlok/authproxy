package config

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDatabase(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("sqlite", func(t *testing.T) {
			data := `
      provider: sqlite
      path: ./some/path.db
`
			var db Database
			assert.NoError(yaml.Unmarshal([]byte(data), &db))
			assert.Equal(Database{InnerVal: &DatabaseSqlite{
				Provider: DatabaseProviderSqlite,
				Path:     "./some/path.db",
			}}, db)
		})
		t.Run("postgres", func(t *testing.T) {
			data := `
      provider: postgres
      host: localhost
      port: 5432
      user: test
      password: secret
      database: authproxy
      sslmode: disable
      params:
        application_name: authproxy-tests
`
			var db Database
			assert.NoError(yaml.Unmarshal([]byte(data), &db))
			assert.Equal(Database{InnerVal: &DatabasePostgres{
				Provider: DatabaseProviderPostgres,
				Host:     common.NewStringValueDirectInline("localhost"),
				Port:     common.NewIntegerValueDirectInline(5432),
				User:     common.NewStringValueDirectInline("test"),
				Password: common.NewStringValueDirectInline("secret"),
				Database: common.NewStringValueDirectInline("authproxy"),
				SSLMode:  common.NewStringValueDirectInline("disable"),
				Params: map[string]string{
					"application_name": "authproxy-tests",
				},
			}}, db)
		})
	})
}

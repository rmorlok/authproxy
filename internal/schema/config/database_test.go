package config

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
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

func TestDatabaseSqlite_Validate(t *testing.T) {
	vc := &common.ValidationContext{Path: "$.database"}

	t.Run("valid config", func(t *testing.T) {
		db := &DatabaseSqlite{
			Provider: DatabaseProviderSqlite,
			Path:     "./test.db",
		}
		assert.NoError(t, db.Validate(vc))
	})

	t.Run("missing path", func(t *testing.T) {
		db := &DatabaseSqlite{
			Provider: DatabaseProviderSqlite,
		}
		err := db.Validate(vc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path must be specified")
	})
}

func TestDatabasePostgres_Validate(t *testing.T) {
	vc := &common.ValidationContext{Path: "$.database"}

	t.Run("valid config", func(t *testing.T) {
		db := &DatabasePostgres{
			Provider: DatabaseProviderPostgres,
			Host:     common.NewStringValueDirectInline("localhost"),
			Database: common.NewStringValueDirectInline("authproxy"),
		}
		assert.NoError(t, db.Validate(vc))
	})

	t.Run("valid config with port", func(t *testing.T) {
		db := &DatabasePostgres{
			Provider: DatabaseProviderPostgres,
			Host:     common.NewStringValueDirectInline("localhost"),
			Port:     common.NewIntegerValueDirectInline(5432),
			User:     common.NewStringValueDirectInline("test"),
			Database: common.NewStringValueDirectInline("authproxy"),
		}
		assert.NoError(t, db.Validate(vc))
	})

	t.Run("missing host", func(t *testing.T) {
		db := &DatabasePostgres{
			Provider: DatabaseProviderPostgres,
			User:     common.NewStringValueDirectInline("test"),
			Database: common.NewStringValueDirectInline("authproxy"),
		}
		err := db.Validate(vc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "host must be specified")
	})

	t.Run("missing database", func(t *testing.T) {
		db := &DatabasePostgres{
			Provider: DatabaseProviderPostgres,
			Host:     common.NewStringValueDirectInline("localhost"),
			User:     common.NewStringValueDirectInline("test"),
		}
		err := db.Validate(vc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database must be specified")
	})

	t.Run("multiple missing fields", func(t *testing.T) {
		db := &DatabasePostgres{
			Provider: DatabaseProviderPostgres,
		}
		err := db.Validate(vc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "host must be specified")
		assert.Contains(t, err.Error(), "database must be specified")
	})

	t.Run("invalid port zero", func(t *testing.T) {
		db := &DatabasePostgres{
			Provider: DatabaseProviderPostgres,
			Host:     common.NewStringValueDirectInline("localhost"),
			Port:     common.NewIntegerValueDirectInline(0),
			User:     common.NewStringValueDirectInline("test"),
			Database: common.NewStringValueDirectInline("authproxy"),
		}
		err := db.Validate(vc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "port must be between 1 and 65535")
	})

	t.Run("invalid port too high", func(t *testing.T) {
		db := &DatabasePostgres{
			Provider: DatabaseProviderPostgres,
			Host:     common.NewStringValueDirectInline("localhost"),
			Port:     common.NewIntegerValueDirectInline(70000),
			User:     common.NewStringValueDirectInline("test"),
			Database: common.NewStringValueDirectInline("authproxy"),
		}
		err := db.Validate(vc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "port must be between 1 and 65535")
	})
}

func TestDatabase_Validate(t *testing.T) {
	vc := &common.ValidationContext{Path: "$.database"}

	t.Run("nil database", func(t *testing.T) {
		var db *Database
		err := db.Validate(vc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database must be specified")
	})

	t.Run("nil inner val", func(t *testing.T) {
		db := &Database{}
		err := db.Validate(vc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database must be specified")
	})

	t.Run("valid sqlite", func(t *testing.T) {
		db := &Database{InnerVal: &DatabaseSqlite{
			Provider: DatabaseProviderSqlite,
			Path:     "./test.db",
		}}
		assert.NoError(t, db.Validate(vc))
	})

	t.Run("valid postgres", func(t *testing.T) {
		db := &Database{InnerVal: &DatabasePostgres{
			Provider: DatabaseProviderPostgres,
			Host:     common.NewStringValueDirectInline("localhost"),
			User:     common.NewStringValueDirectInline("test"),
			Database: common.NewStringValueDirectInline("authproxy"),
		}}
		assert.NoError(t, db.Validate(vc))
	})

	t.Run("delegates to inner validation", func(t *testing.T) {
		db := &Database{InnerVal: &DatabaseSqlite{
			Provider: DatabaseProviderSqlite,
		}}
		err := db.Validate(vc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path must be specified")
	})
}

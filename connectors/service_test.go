package connectors

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/test_utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
)

func TestService(t *testing.T) {
	var cfg config.C
	var db database.DB
	var rawDb *sql.DB
	var service C

	setup := func(t *testing.T, connectors config.Connectors) func() {
		cfg = config.FromRoot(&config.Root{
			DevSettings: &config.DevSettings{
				Enabled:                  true,
				FakeEncryption:           true,
				FakeEncryptionSkipBase64: true,
			},
			Connectors: connectors,
		})

		cfg, db, rawDb = database.MustApplyBlankTestDbConfigRaw(t.Name(), cfg)

		e := encrypt.NewEncryptService(cfg, db)
		logger := slog.Default()

		service = NewConnectorsService(cfg, db, e, logger)

		return func() {
			err := rawDb.Close()
			assert.NoError(t, err)
		}
	}

	t.Run("no connectors", func(t *testing.T) {
		cleanup := setup(t, config.Connectors{})
		defer cleanup()

		err := service.MigrateConnectors(context.Background())
		assert.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

		type connectorResult struct {
			Id      string
			Version int64
			State   string
		}

		test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state FROM connector_versions;
		`, []connectorResult{})
	})

	t.Run("id and version", func(t *testing.T) {
		t.Run("single initial", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:      uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version: 1,
					Type:    "fake",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			type connectorResult struct {
				Id      string
				Version int64
				State   string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state FROM connector_versions;
		`, []connectorResult{
				{
					Id:      "00000000-0000-0000-0000-000000000001",
					Version: 1,
					State:   "primary",
				},
			})
		})

		t.Run("unchanged from initial", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:      uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version: 1,
					Type:    "fake",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			type connectorResult struct {
				Id      string
				Version int64
				State   string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state FROM connector_versions;
		`, []connectorResult{
				{
					Id:      "00000000-0000-0000-0000-000000000001",
					Version: 1,
					State:   "primary",
				},
			})
		})

		t.Run("changed once", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake",
					DisplayName: "initial",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			cfg.GetRoot().Connectors[0].Version = 2
			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{
				{
					Id:          "00000000-0000-0000-0000-000000000001",
					Version:     1,
					State:       "active",
					DisplayName: "initial",
				},
				{
					Id:          "00000000-0000-0000-0000-000000000001",
					Version:     2,
					State:       "primary",
					DisplayName: "changed",
				},
			})
		})

		t.Run("changed once then unchanged", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake",
					DisplayName: "initial",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			cfg.GetRoot().Connectors[0].Version = 2
			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{
				{
					Id:          "00000000-0000-0000-0000-000000000001",
					Version:     1,
					State:       "active",
					DisplayName: "initial",
				},
				{
					Id:          "00000000-0000-0000-0000-000000000001",
					Version:     2,
					State:       "primary",
					DisplayName: "changed",
				},
			})
		})
	})

	t.Run("id", func(t *testing.T) {
		t.Run("single initial", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:   uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type: "fake",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			type connectorResult struct {
				Id      string
				Version int64
				State   string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state FROM connector_versions;
		`, []connectorResult{
				{
					Id:      "00000000-0000-0000-0000-000000000001",
					Version: 1,
					State:   "primary",
				},
			})
		})

		t.Run("unchanged from initial", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:   uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type: "fake",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			type connectorResult struct {
				Id      string
				Version int64
				State   string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state FROM connector_versions;
		`, []connectorResult{
				{
					Id:      "00000000-0000-0000-0000-000000000001",
					Version: 1,
					State:   "primary",
				},
			})
		})

		t.Run("changed once", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type:        "fake",
					DisplayName: "initial",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{
				{
					Id:          "00000000-0000-0000-0000-000000000001",
					Version:     1,
					State:       "active",
					DisplayName: "initial",
				},
				{
					Id:          "00000000-0000-0000-0000-000000000001",
					Version:     2,
					State:       "primary",
					DisplayName: "changed",
				},
			})
		})

		t.Run("changed once then unchanged", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type:        "fake",
					DisplayName: "initial",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{
				{
					Id:          "00000000-0000-0000-0000-000000000001",
					Version:     1,
					State:       "active",
					DisplayName: "initial",
				},
				{
					Id:          "00000000-0000-0000-0000-000000000001",
					Version:     2,
					State:       "primary",
					DisplayName: "changed",
				},
			})
		})
	})

	t.Run("type only", func(t *testing.T) {
		t.Run("single initial", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Type: "fake",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			type connectorResult struct {
				Version int64
				State   string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT version, state FROM connector_versions;
		`, []connectorResult{
				{
					Version: 1,
					State:   "primary",
				},
			})
		})

		t.Run("unchanged initial", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Type: "fake",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			type connectorResult struct {
				Version int64
				State   string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT version, state FROM connector_versions;
		`, []connectorResult{
				{
					Version: 1,
					State:   "primary",
				},
			})
		})

		t.Run("changed once", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Type:        "fake",
					DisplayName: "initial",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			type connectorResult struct {
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{
				{
					Version:     1,
					State:       "active",
					DisplayName: "initial",
				},
				{
					Version:     2,
					State:       "primary",
					DisplayName: "changed",
				},
			})
		})

		t.Run("changed once then unchanged", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Type:        "fake",
					DisplayName: "initial",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err, "MigrateConnectors should not return an error with no connectors")

			type connectorResult struct {
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{
				{
					Version:     1,
					State:       "active",
					DisplayName: "initial",
				},
				{
					Version:     2,
					State:       "primary",
					DisplayName: "changed",
				},
			})
		})
	})
}

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
		assert.NoError(t, err)

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
			require.NoError(t, err)

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

		t.Run("double initial same type", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:      uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version: 1,
					Type:    "fake",
				},
				{
					Id:      uuid.MustParse("00000000-0000-0000-0000-000000000002"),
					Version: 1,
					Type:    "fake",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			type connectorResult struct {
				Id      string
				Version int64
				State   string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state FROM connector_versions ORDER BY id;
		`, []connectorResult{
				{
					Id:      "00000000-0000-0000-0000-000000000001",
					Version: 1,
					State:   "primary",
				},
				{
					Id:      "00000000-0000-0000-0000-000000000002",
					Version: 1,
					State:   "primary",
				},
			})
		})

		t.Run("double initial different type", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:      uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version: 1,
					Type:    "fake1",
				},
				{
					Id:      uuid.MustParse("00000000-0000-0000-0000-000000000002"),
					Version: 1,
					Type:    "fake2",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			type connectorResult struct {
				Id      string
				Version int64
				Type    string
				State   string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, type, state FROM connector_versions ORDER BY id;
		`, []connectorResult{
				{
					Id:      "00000000-0000-0000-0000-000000000001",
					Version: 1,
					Type:    "fake1",
					State:   "primary",
				},
				{
					Id:      "00000000-0000-0000-0000-000000000002",
					Version: 1,
					Type:    "fake2",
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
			require.NoError(t, err)

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].Version = 2
			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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

		t.Run("add draft version", func(t *testing.T) {
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
			require.NoError(t, err)

			// Draft versions can be added; non-specified versions default to primary
			cfg.GetRoot().Connectors = append(cfg.GetRoot().Connectors, config.Connector{
				Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				Version:     2,
				State:       "draft",
				Type:        "fake",
				DisplayName: "changed",
			})

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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
					State:       "primary",
					DisplayName: "initial",
				},
				{
					Id:          "00000000-0000-0000-0000-000000000001",
					Version:     2,
					State:       "draft",
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
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].Version = 2
			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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

		t.Run("changed twice", func(t *testing.T) {
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
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].Version = 2
			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].Version = 3
			cfg.GetRoot().Connectors[0].DisplayName = "changed again"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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
					State:       "active",
					DisplayName: "changed",
				},
				{
					Id:          "00000000-0000-0000-0000-000000000001",
					Version:     3,
					State:       "primary",
					DisplayName: "changed again",
				},
			})
		})

		t.Run("cannot change published version", func(t *testing.T) {
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
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.Error(t, err)

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
					State:       "primary",
					DisplayName: "initial",
				},
			})
		})

		t.Run("does not allow duplicate id versions initial", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake",
					DisplayName: "first",
				},
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake",
					DisplayName: "second",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.Error(t, err)

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{})
		})

		t.Run("does not allow duplicate id versions when migrated", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake",
					DisplayName: "first",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			cfg.GetRoot().Connectors = append(cfg.GetRoot().Connectors, config.Connector{
				Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				Version:     1,
				Type:        "fake",
				DisplayName: "second",
			})

			err = service.MigrateConnectors(context.Background())
			require.Error(t, err)

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
					State:       "primary",
					DisplayName: "first",
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
			require.NoError(t, err)

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

		t.Run("double initial same type", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:   uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type: "fake",
				},
				{
					Id:   uuid.MustParse("00000000-0000-0000-0000-000000000002"),
					Type: "fake",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			type connectorResult struct {
				Id      string
				Version int64
				State   string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state FROM connector_versions ORDER BY id;
		`, []connectorResult{
				{
					Id:      "00000000-0000-0000-0000-000000000001",
					Version: 1,
					State:   "primary",
				},
				{
					Id:      "00000000-0000-0000-0000-000000000002",
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
			require.NoError(t, err)

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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

		t.Run("add draft version", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type:        "fake",
					DisplayName: "initial",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			cfg.GetRoot().Connectors = append(cfg.GetRoot().Connectors, config.Connector{
				Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				Type:        "fake",
				State:       "draft",
				DisplayName: "changed",
			})

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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
					State:       "primary",
					DisplayName: "initial",
				},
				{
					Id:          "00000000-0000-0000-0000-000000000001",
					Version:     2,
					State:       "draft",
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
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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

		t.Run("changed twice", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type:        "fake",
					DisplayName: "initial",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].DisplayName = "changed again"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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
					State:       "active",
					DisplayName: "changed",
				},
				{
					Id:          "00000000-0000-0000-0000-000000000001",
					Version:     3,
					State:       "primary",
					DisplayName: "changed again",
				},
			})
		})

		t.Run("does not allow duplicate id initial", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type:        "fake",
					DisplayName: "first",
				},
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type:        "fake",
					DisplayName: "second",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.Error(t, err)

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{})
		})

		t.Run("does not allow duplicate id when migrated", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type:        "fake",
					DisplayName: "first",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			cfg.GetRoot().Connectors = append(cfg.GetRoot().Connectors, config.Connector{
				Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				Type:        "fake",
				DisplayName: "second",
			})

			err = service.MigrateConnectors(context.Background())
			require.Error(t, err)

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
					State:       "primary",
					DisplayName: "first",
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
			require.NoError(t, err)

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
			require.NoError(t, err)

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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

		t.Run("changed twice", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Type:        "fake",
					DisplayName: "initial",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].DisplayName = "changed"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			cfg.GetRoot().Connectors[0].DisplayName = "changed again"

			err = service.MigrateConnectors(context.Background())
			require.NoError(t, err)

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
					State:       "active",
					DisplayName: "changed",
				},
				{
					Version:     3,
					State:       "primary",
					DisplayName: "changed again",
				},
			})
		})

		t.Run("does not allow duplicate type without id initial", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Type:        "fake",
					DisplayName: "first",
				},
				{
					Type:        "fake",
					DisplayName: "second",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.Error(t, err)

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{})
		})

		t.Run("does not allow duplicate type without id when migrated", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Type:        "fake",
					DisplayName: "first",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.NoError(t, err)

			cfg.GetRoot().Connectors = append(cfg.GetRoot().Connectors, config.Connector{
				Type:        "fake",
				DisplayName: "second",
			})

			err = service.MigrateConnectors(context.Background())
			require.Error(t, err)

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
					State:       "primary",
					DisplayName: "first",
				},
			})
		})
	})

	t.Run("bad config files", func(t *testing.T) {
		t.Run("duplicate id version type", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake",
					DisplayName: "duplicate",
				},
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake",
					DisplayName: "duplicate",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.Error(t, err)

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{})
		})

		t.Run("duplicate id version state primary", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					State:       "primary",
					Type:        "fake1",
					DisplayName: "duplicate",
				},
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					State:       "primary",
					Type:        "fake2",
					DisplayName: "duplicate",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.Error(t, err)

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{})
		})

		t.Run("duplicate id version state draft", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					State:       "draft",
					Type:        "fake1",
					DisplayName: "duplicate",
				},
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					State:       "draft",
					Type:        "fake2",
					DisplayName: "duplicate",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.Error(t, err)

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{})
		})

		t.Run("duplicate id version", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake1",
					DisplayName: "duplicate",
				},
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake2",
					DisplayName: "duplicate",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.Error(t, err)

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{})
		})

		t.Run("id with and without version", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake1",
					DisplayName: "duplicate",
				},
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type:        "fake2",
					DisplayName: "duplicate",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.Error(t, err)

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{})
		})

		t.Run("id version and type without id", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake",
					DisplayName: "duplicate",
				},
				{
					Type:        "fake",
					DisplayName: "duplicate",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.Error(t, err)

			cleanup2 := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake",
					DisplayName: "duplicate",
				},
				{
					Version:     2,
					Type:        "fake",
					DisplayName: "duplicate",
				},
			})
			defer cleanup2()

			err = service.MigrateConnectors(context.Background())
			require.Error(t, err)

			cleanup3 := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Version:     1,
					Type:        "fake",
					DisplayName: "duplicate",
				},
				{
					Version:     2,
					State:       "draft",
					Type:        "fake",
					DisplayName: "duplicate",
				},
			})
			defer cleanup3()

			err = service.MigrateConnectors(context.Background())
			require.Error(t, err)

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{})
		})

		t.Run("id and type without id", func(t *testing.T) {
			cleanup := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type:        "fake",
					DisplayName: "duplicate",
				},
				{
					Type:        "fake",
					DisplayName: "duplicate",
				},
			})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			require.Error(t, err)

			cleanup2 := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type:        "fake",
					DisplayName: "duplicate",
				},
				{
					Version:     2,
					Type:        "fake",
					DisplayName: "duplicate",
				},
			})
			defer cleanup2()

			err = service.MigrateConnectors(context.Background())
			require.Error(t, err)

			cleanup3 := setup(t, config.Connectors{
				{
					Id:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					Type:        "fake",
					DisplayName: "duplicate",
				},
				{
					Version:     2,
					State:       "draft",
					Type:        "fake",
					DisplayName: "duplicate",
				},
			})
			defer cleanup3()

			err = service.MigrateConnectors(context.Background())
			require.Error(t, err)

			type connectorResult struct {
				Id          string
				Version     int64
				State       string
				DisplayName string
			}

			test_utils.AssertSql(t, rawDb, `
			SELECT id, version, state, json_extract(encrypted_definition, '$.display_name') as display_name FROM connector_versions ORDER BY version;
		`, []connectorResult{})
		})
	})
}

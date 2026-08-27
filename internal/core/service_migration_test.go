package core

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	apredismock "github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	hmock "github.com/rmorlok/authproxy/internal/httpf/mock"
	cfgschema "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func displayNameExpr(cfg config.C) string {
	if cfg.GetRoot().Database.GetProvider() == cfgschema.DatabaseProviderPostgres {
		return "(encrypted_definition ->> 'd')::jsonb ->> 'displayName'"
	}
	return "json_extract(json_extract(encrypted_definition, '$.d'), '$.displayName')"
}

func withDisplayNameExpr(cfg config.C, query string) string {
	return strings.ReplaceAll(query, "DISPLAY_NAME_EXPR", displayNameExpr(cfg))
}

func assertSqlWithDisplayName[T any](t *testing.T, rawDb *sql.DB, cfg config.C, query string, expected []T) {
	test_utils.AssertSql[T](t, rawDb, withDisplayNameExpr(cfg, query), expected)
}

func TestMigration(t *testing.T) {
	var cfg config.C
	var db database.DB
	var r apredis.Client
	var h httpf.F
	var rawDb *sql.DB
	var service iface.C
	var asynqClient apasynq.Client

	setup := func(t *testing.T, connectors []cschema.Connector) func() {
		cfg = config.FromRoot(&cfgschema.Root{
			DevSettings: &cfgschema.DevSettings{
				Enabled:                  true,
				FakeEncryption:           true,
				FakeEncryptionSkipBase64: true,
			},
			Connectors: &cfgschema.Connectors{
				LoadFromList: connectors,
			},
		})

		logger := slog.Default()
		cfg, db, rawDb = database.MustApplyBlankTestDbConfigRaw(t, cfg)
		ctrl := gomock.NewController(t)
		r = apredismock.NewMockClient(ctrl)
		e := encrypt.NewEncryptService(cfg, db, logger)

		asynqClient = mock.NewMockClient(ctrl)
		asynqClient.(*mock.MockClient).EXPECT().
			EnqueueContext(gomock.Any(), gomock.Any()).
			AnyTimes().
			Return(nil, nil)
		h = hmock.NewMockF(ctrl)

		service = NewCoreService(cfg, db, e, r, h, asynqClient, logger)

		return func() {
			ctrl.Finish()
			err := rawDb.Close()
			assert.NoError(t, err)
		}
	}

	t.Run("connectors", func(t *testing.T) {
		t.Run("no connectors", func(t *testing.T) {
			cleanup := setup(t, []cschema.Connector{})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			assert.NoError(t, err)

			type connectorResult struct {
				Id      string
				Version int64
				State   string
			}

			assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state FROM connector_definition_versions;
			`, []connectorResult{})
		})

		t.Run("names reconcile connector identity", func(t *testing.T) {
			t.Run("annotation changes preserve the generated connector id", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{{
					Name:        "configured",
					Labels:      map[string]string{"type": "test"},
					Annotations: map[string]string{"example.com/owner": "before@example.com"},
					DisplayName: "Configured connector",
				}})
				defer cleanup()

				require.NoError(t, service.MigrateConnectors(context.Background()))
				first := db.ListConnectorsBuilder().ForName("configured").FetchPage(context.Background())
				require.NoError(t, first.Error)
				require.Len(t, first.Results, 1)
				generatedID := first.Results[0].Id
				require.Equal(t, "before@example.com", first.Results[0].Annotations["example.com/owner"])

				cfg.GetRoot().Connectors.LoadFromList[0].Annotations["example.com/owner"] = "after@example.com"
				require.NoError(t, service.MigrateConnectors(context.Background()))

				result := db.ListConnectorsBuilder().ForName("configured").FetchPage(context.Background())
				require.NoError(t, result.Error)
				require.Len(t, result.Results, 1)
				require.Equal(t, generatedID, result.Results[0].Id)
				require.Equal(t, "after@example.com", result.Results[0].Annotations["example.com/owner"])

				versions := db.ListConnectorDefinitionVersionsBuilder().ForId(generatedID).FetchPage(context.Background())
				require.NoError(t, versions.Error)
				require.Len(t, versions.Results, 2)
			})

			t.Run("label changes preserve the generated connector id", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{{
					Name:        "configured",
					Labels:      map[string]string{"type": "before"},
					DisplayName: "Before",
				}})
				defer cleanup()

				require.NoError(t, service.MigrateConnectors(context.Background()))
				first := db.ListConnectorsBuilder().ForName("configured").FetchPage(context.Background())
				require.NoError(t, first.Error)
				require.Len(t, first.Results, 1)
				generatedID := first.Results[0].Id

				cfg.GetRoot().Connectors.LoadFromList[0].Labels["type"] = "after"
				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "After"
				require.NoError(t, service.MigrateConnectors(context.Background()))

				result := db.ListConnectorsBuilder().ForName("configured").FetchPage(context.Background())
				require.NoError(t, result.Error)
				require.Len(t, result.Results, 1)
				require.Equal(t, generatedID, result.Results[0].Id)
				require.Equal(t, "after", result.Results[0].Labels["type"])

				versions := db.ListConnectorDefinitionVersionsBuilder().ForId(generatedID).FetchPage(context.Background())
				require.NoError(t, versions.Error)
				require.Len(t, versions.Results, 2)
			})

			t.Run("explicit id can rename without creating a version", func(t *testing.T) {
				connectorID := apid.MustParse("cxr_test0000000000001")
				cleanup := setup(t, []cschema.Connector{{
					Id:          connectorID,
					Name:        "before",
					Labels:      map[string]string{"type": "same"},
					DisplayName: "Unchanged definition",
				}})
				defer cleanup()

				require.NoError(t, service.MigrateConnectors(context.Background()))
				cfg.GetRoot().Connectors.LoadFromList[0].Name = "after"
				require.NoError(t, service.MigrateConnectors(context.Background()))

				renamed := db.ListConnectorsBuilder().ForName("after").FetchPage(context.Background())
				require.NoError(t, renamed.Error)
				require.Len(t, renamed.Results, 1)
				require.Equal(t, connectorID, renamed.Results[0].Id)
				require.Equal(t, "after", renamed.Results[0].Labels["apxy/cxr/-/name"])

				versions := db.ListConnectorDefinitionVersionsBuilder().ForId(connectorID).FetchPage(context.Background())
				require.NoError(t, versions.Error)
				require.Len(t, versions.Results, 1)
			})

			t.Run("explicit id can rename while adding a version", func(t *testing.T) {
				connectorID := apid.MustParse("cxr_test0000000000001")
				cleanup := setup(t, []cschema.Connector{{
					Id:          connectorID,
					Name:        "before",
					Version:     1,
					Labels:      map[string]string{"type": "same"},
					DisplayName: "Version one",
				}})
				defer cleanup()

				require.NoError(t, service.MigrateConnectors(context.Background()))
				cfg.GetRoot().Connectors.LoadFromList[0].Name = "after"
				cfg.GetRoot().Connectors.LoadFromList[0].Version = 2
				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "Version two"
				require.NoError(t, service.MigrateConnectors(context.Background()))

				renamed := db.ListConnectorsBuilder().ForName("after").FetchPage(context.Background())
				require.NoError(t, renamed.Error)
				require.Len(t, renamed.Results, 1)
				require.Equal(t, connectorID, renamed.Results[0].Id)

				versions := db.ListConnectorDefinitionVersionsBuilder().ForId(connectorID).FetchPage(context.Background())
				require.NoError(t, versions.Error)
				require.Len(t, versions.Results, 2)
			})

			t.Run("same name in different namespaces creates different connectors", func(t *testing.T) {
				firstNamespace := "root.first"
				secondNamespace := "root.second"
				cleanup := setup(t, []cschema.Connector{
					{Name: "shared", Namespace: &firstNamespace, Labels: map[string]string{"type": "same"}},
					{Name: "shared", Namespace: &secondNamespace, Labels: map[string]string{"type": "same"}},
				})
				defer cleanup()

				require.NoError(t, service.Migrate(context.Background()))

				results := db.ListConnectorsBuilder().ForName("shared").FetchPage(context.Background())
				require.NoError(t, results.Error)
				require.Len(t, results.Results, 2)
				require.NotEqual(t, results.Results[0].Id, results.Results[1].Id)
			})
		})

		t.Run("id and version", func(t *testing.T) {
			t.Run("single initial", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:      apid.MustParse("cxr_test0000000000001"),
						Version: 1,
						Labels:  map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state FROM connector_definition_versions;
		`, []connectorResult{
					{
						Id:      "cxr_test0000000000001",
						Version: 1,
						State:   "primary",
					},
				})
			})

			t.Run("double initial same type", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:      apid.MustParse("cxr_test0000000000001"),
						Version: 1,
						Labels:  map[string]string{"type": "fake"},
					},
					{
						Id:      apid.MustParse("cxr_test0000000000002"),
						Version: 1,
						Labels:  map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state FROM connector_definition_versions ORDER BY id;
		`, []connectorResult{
					{
						Id:      "cxr_test0000000000001",
						Version: 1,
						State:   "primary",
					},
					{
						Id:      "cxr_test0000000000002",
						Version: 1,
						State:   "primary",
					},
				})
			})

			t.Run("double initial different type", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:      apid.MustParse("cxr_test0000000000001"),
						Version: 1,
						Labels:  map[string]string{"type": "fake1"},
					},
					{
						Id:      apid.MustParse("cxr_test0000000000002"),
						Version: 1,
						Labels:  map[string]string{"type": "fake2"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state FROM connector_definition_versions ORDER BY id;
		`, []connectorResult{
					{
						Id:      "cxr_test0000000000001",
						Version: 1,
						State:   "primary",
					},
					{
						Id:      "cxr_test0000000000002",
						Version: 1,
						State:   "primary",
					},
				})
			})

			t.Run("unchanged from initial", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:      apid.MustParse("cxr_test0000000000001"),
						Version: 1,
						Labels:  map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state FROM connector_definition_versions;
		`, []connectorResult{
					{
						Id:      "cxr_test0000000000001",
						Version: 1,
						State:   "primary",
					},
				})
			})

			t.Run("changed once", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				require.NoError(t, db.UpdateConnectorName(
					context.Background(),
					apid.MustParse("cxr_test0000000000001"),
					"renamed",
				))

				cfg.GetRoot().Connectors.LoadFromList[0].Version = 2
				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Version     int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Version:     1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Version:     2,
						State:       "primary",
						DisplayName: "changed",
					},
				})

				var logicalName string
				require.NoError(t, rawDb.QueryRow(`
					SELECT name FROM connectors WHERE id = 'cxr_test0000000000001'
				`).Scan(&logicalName))
				require.Equal(t, "renamed", logicalName)
			})

			t.Run("add draft version", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				// Draft versions can be added; non-specified versions default to primary
				cfg.GetRoot().Connectors.LoadFromList = append(cfg.GetRoot().Connectors.LoadFromList, cschema.Connector{
					Id:          apid.MustParse("cxr_test0000000000001"),
					Version:     2,
					State:       "draft",
					Labels:      map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Version:     1,
						State:       "primary",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Version:     2,
						State:       "draft",
						DisplayName: "changed",
					},
				})
			})

			t.Run("changed once then unchanged", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Version = 2
				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed"

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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Version:     1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Version:     2,
						State:       "primary",
						DisplayName: "changed",
					},
				})
			})

			t.Run("changed twice", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Version = 2
				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Version = 3
				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed again"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Version     int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Version:     1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Version:     2,
						State:       "active",
						DisplayName: "changed",
					},
					{
						Id:          "cxr_test0000000000001",
						Version:     3,
						State:       "primary",
						DisplayName: "changed again",
					},
				})
			})

			t.Run("cannot change published version", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Id          string
					Version     int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Version:     1,
						State:       "primary",
						DisplayName: "initial",
					},
				})
			})

			t.Run("does not allow duplicate id versions initial", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "first",
					},
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{})
			})

			t.Run("does not allow duplicate id versions when migrated", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "first",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList = append(cfg.GetRoot().Connectors.LoadFromList, cschema.Connector{
					Id:          apid.MustParse("cxr_test0000000000001"),
					Version:     1,
					Labels:      map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Version:     1,
						State:       "primary",
						DisplayName: "first",
					},
				})
			})
		})

		t.Run("id", func(t *testing.T) {
			t.Run("single initial", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:     apid.MustParse("cxr_test0000000000001"),
						Labels: map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state FROM connector_definition_versions;
		`, []connectorResult{
					{
						Id:      "cxr_test0000000000001",
						Version: 1,
						State:   "primary",
					},
				})
			})

			t.Run("double initial same type", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:     apid.MustParse("cxr_test0000000000001"),
						Labels: map[string]string{"type": "fake"},
					},
					{
						Id:     apid.MustParse("cxr_test0000000000002"),
						Labels: map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state FROM connector_definition_versions ORDER BY id;
		`, []connectorResult{
					{
						Id:      "cxr_test0000000000001",
						Version: 1,
						State:   "primary",
					},
					{
						Id:      "cxr_test0000000000002",
						Version: 1,
						State:   "primary",
					},
				})
			})

			t.Run("unchanged from initial", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:     apid.MustParse("cxr_test0000000000001"),
						Labels: map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state FROM connector_definition_versions;
		`, []connectorResult{
					{
						Id:      "cxr_test0000000000001",
						Version: 1,
						State:   "primary",
					},
				})
			})

			t.Run("changed once", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Version     int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Version:     1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Version:     2,
						State:       "primary",
						DisplayName: "changed",
					},
				})
			})

			t.Run("add draft version", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList = append(cfg.GetRoot().Connectors.LoadFromList, cschema.Connector{
					Id:          apid.MustParse("cxr_test0000000000001"),
					Labels:      map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Version:     1,
						State:       "primary",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Version:     2,
						State:       "draft",
						DisplayName: "changed",
					},
				})
			})

			t.Run("changed once then unchanged", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed"

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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Version:     1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Version:     2,
						State:       "primary",
						DisplayName: "changed",
					},
				})
			})

			t.Run("changed twice", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed again"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Version     int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Version:     1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Version:     2,
						State:       "active",
						DisplayName: "changed",
					},
					{
						Id:          "cxr_test0000000000001",
						Version:     3,
						State:       "primary",
						DisplayName: "changed again",
					},
				})
			})

			t.Run("does not allow duplicate id initial", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "first",
					},
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{})
			})

			t.Run("does not allow duplicate id when migrated", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "first",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList = append(cfg.GetRoot().Connectors.LoadFromList, cschema.Connector{
					Id:          apid.MustParse("cxr_test0000000000001"),
					Labels:      map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Version:     1,
						State:       "primary",
						DisplayName: "first",
					},
				})
			})
		})

		t.Run("name and version", func(t *testing.T) {
			t.Run("changed once preserves generated id", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Name:        "fake",
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Version = 2
				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Version     int64
					State       string
					DisplayName string
				}

				rows, err := rawDb.Query(withDisplayNameExpr(cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name
			FROM connector_definition_versions
			WHERE connector_id IN (SELECT id FROM connectors WHERE deleted_at IS NULL)
			ORDER BY version;
		`))
				require.NoError(t, err)
				defer rows.Close()

				var results []connectorResult
				for rows.Next() {
					var result connectorResult
					require.NoError(t, rows.Scan(&result.Id, &result.Version, &result.State, &result.DisplayName))
					results = append(results, result)
				}
				require.NoError(t, rows.Err())
				require.Len(t, results, 2)
				require.Equal(t, results[0].Id, results[1].Id)
				require.Equal(t, connectorResult{
					Id:          results[0].Id,
					Version:     1,
					State:       "active",
					DisplayName: "initial",
				}, results[0])
				require.Equal(t, connectorResult{
					Id:          results[0].Id,
					Version:     2,
					State:       "primary",
					DisplayName: "changed",
				}, results[1])
			})

			t.Run("initial version must start at one", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Name:        "fake",
						Version:     2,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Id      string
					Version int64
					State   string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state FROM connector_definition_versions WHERE connector_id IN (SELECT id FROM connectors WHERE deleted_at IS NULL);
		`, []connectorResult{})
			})

			t.Run("cannot change published version", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Name:        "fake",
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Version     int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT version, state, DISPLAY_NAME_EXPR as display_name
			FROM connector_definition_versions
			WHERE connector_id IN (SELECT id FROM connectors WHERE deleted_at IS NULL)
			ORDER BY version;
		`, []connectorResult{
					{
						Version:     1,
						State:       "primary",
						DisplayName: "initial",
					},
				})
			})
		})

		t.Run("name only", func(t *testing.T) {
			t.Run("single initial", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Name:   "fake",
						Labels: map[string]string{"type": "fake"},
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Version int64
					State   string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT version, state FROM connector_definition_versions;
		`, []connectorResult{
					{
						Version: 1,
						State:   "primary",
					},
				})
			})

			t.Run("unchanged initial", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Name:   "fake",
						Labels: map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT version, state FROM connector_definition_versions;
		`, []connectorResult{
					{
						Version: 1,
						State:   "primary",
					},
				})
			})

			t.Run("changed once", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Version     int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
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
				cleanup := setup(t, []cschema.Connector{
					{
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Version     int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
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
				cleanup := setup(t, []cschema.Connector{
					{
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].DisplayName = "changed again"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Version     int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
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

			t.Run("does not allow duplicate name without id initial", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "first",
					},
					{
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{})
			})

			t.Run("does not allow duplicate name without id when migrated", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "first",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList = append(cfg.GetRoot().Connectors.LoadFromList, cschema.Connector{
					Name:        "fake",
					Labels:      map[string]string{"type": "fake"},
					DisplayName: "second",
				})

				err = service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Version     int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
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
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{})
			})

			t.Run("duplicate id version state primary", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						State:       "primary",
						Labels:      map[string]string{"type": "fake1"},
						DisplayName: "duplicate",
					},
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						State:       "primary",
						Labels:      map[string]string{"type": "fake2"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{})
			})

			t.Run("duplicate id version state draft", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						State:       "draft",
						Labels:      map[string]string{"type": "fake1"},
						DisplayName: "duplicate",
					},
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						State:       "draft",
						Labels:      map[string]string{"type": "fake2"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{})
			})

			t.Run("duplicate id version", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake1"},
						DisplayName: "duplicate",
					},
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake2"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{})
			})

			t.Run("id with and without version", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Version:     1,
						Labels:      map[string]string{"type": "fake1"},
						DisplayName: "duplicate",
					},
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake2"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{})
			})

			t.Run("id version and name without id", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Name:        "fake",
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
					{
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.Error(t, err)

				cleanup2 := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Name:        "fake",
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
					{
						Name:        "fake",
						Version:     2,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
				})
				defer cleanup2()

				err = service.MigrateConnectors(context.Background())
				require.Error(t, err)

				cleanup3 := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Name:        "fake",
						Version:     1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
					{
						Name:        "fake",
						Version:     2,
						State:       "draft",
						Labels:      map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{})
			})

			t.Run("id and name without id", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
					{
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.Error(t, err)

				cleanup2 := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
					{
						Name:        "fake",
						Version:     2,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
				})
				defer cleanup2()

				err = service.MigrateConnectors(context.Background())
				require.Error(t, err)

				cleanup3 := setup(t, []cschema.Connector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
					{
						Name:        "fake",
						Version:     2,
						State:       "draft",
						Labels:      map[string]string{"type": "fake"},
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

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state, DISPLAY_NAME_EXPR as display_name FROM connector_definition_versions ORDER BY version;
		`, []connectorResult{})
			})
		})

		t.Run("orphan cleanup", func(t *testing.T) {
			t.Run("config-sourced connector with no connections is removed", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:      apid.MustParse("cxr_test0000000000001"),
						Version: 1,
						Labels:  map[string]string{"type": "fake1"},
					},
					{
						Id:      apid.MustParse("cxr_test0000000000002"),
						Version: 1,
						Labels:  map[string]string{"type": "fake2"},
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				// Drop the second connector from the config and re-run.
				cfg.GetRoot().Connectors.LoadFromList = cfg.GetRoot().Connectors.LoadFromList[:1]

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id      string
					Version int64
					State   string
				}

				// The orphan's row is soft-deleted; only the surviving connector remains.
				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state FROM connector_definition_versions WHERE connector_id IN (SELECT id FROM connectors WHERE deleted_at IS NULL) ORDER BY id;
		`, []connectorResult{
					{Id: "cxr_test0000000000001", Version: 1, State: "primary"},
				})
			})

			t.Run("config-sourced connector with live connections is demoted", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:      apid.MustParse("cxr_test0000000000001"),
						Version: 1,
						Labels:  map[string]string{"type": "fake1"},
					},
					{
						Id:      apid.MustParse("cxr_test0000000000002"),
						Version: 1,
						Labels:  map[string]string{"type": "fake2"},
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				// Create a connection against the connector we are about to drop.
				err = db.CreateConnection(context.Background(), &database.Connection{
					Id:               apid.MustParse("cxn_test0000000000001"),
					Namespace:        "root",
					ConnectorId:      apid.MustParse("cxr_test0000000000002"),
					ConnectorVersion: 1,
					State:            database.ConnectionStateConfigured,
				})
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList = cfg.GetRoot().Connectors.LoadFromList[:1]

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id      string
					Version int64
					State   string
				}

				// Orphan must NOT be deleted (still rows present), and its primary version
				// must be demoted to active.
				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state FROM connector_definition_versions WHERE connector_id IN (SELECT id FROM connectors WHERE deleted_at IS NULL) ORDER BY id;
		`, []connectorResult{
					{Id: "cxr_test0000000000001", Version: 1, State: "primary"},
					{Id: "cxr_test0000000000002", Version: 1, State: "active"},
				})
			})

			t.Run("api-created connectors are not touched", func(t *testing.T) {
				cleanup := setup(t, []cschema.Connector{
					{
						Id:      apid.MustParse("cxr_test0000000000001"),
						Version: 1,
						Labels:  map[string]string{"type": "fake1"},
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				// Insert a connector version directly via the database, simulating
				// an API-driven create. It carries no apxy/cxr/source label.
				apiId := apid.MustParse("cxr_test0000000000099")
				err = db.UpsertConnectorDefinitionVersion(context.Background(), &database.ConnectorWithDefinition{
					Id:                  apiId,
					Version:             1,
					Namespace:           "root",
					State:               database.ConnectorDefinitionVersionStatePrimary,
					EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "api-created"},
					Labels:              database.Labels{"type": "api-only"},
				})
				require.NoError(t, err)

				// Re-run migration with the same config — the API-created connector
				// should remain untouched even though it is not in the config.
				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id      string
					Version int64
					State   string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, version, state FROM connector_definition_versions WHERE connector_id IN (SELECT id FROM connectors WHERE deleted_at IS NULL) ORDER BY id;
		`, []connectorResult{
					{Id: "cxr_test0000000000001", Version: 1, State: "primary"},
					{Id: "cxr_test0000000000099", Version: 1, State: "primary"},
				})
			})
		})
	})

	t.Run("namespaces", func(t *testing.T) {
		t.Run("includes configured actor namespaces", func(t *testing.T) {
			cleanup := setup(t, []cschema.Connector{})
			defer cleanup()

			cfg.GetRoot().SystemAuth.Actors = &cfgschema.ConfiguredActors{
				InnerVal: cfgschema.ConfiguredActorsList{{
					ExternalId: "smoke-user",
					Namespace:  "root.smoke",
					Key: &cfgschema.Key{
						InnerVal: &cfgschema.KeyShared{
							SharedKey: &cfgschema.KeyData{
								InnerVal: &cfgschema.KeyDataBase64Val{Base64: "dGVzdA=="},
							},
						},
					},
				}},
			}

			require.NoError(t, service.Migrate(context.Background()))
			_, err := db.GetNamespace(context.Background(), "root.smoke")
			require.NoError(t, err)
		})
	})
}

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
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	cfgschema "github.com/rmorlok/authproxy/internal/schema/config"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type configuredConnector struct {
	Id          apid.ID
	Name        string
	Namespace   *string
	Generation  uint64
	State       string
	Labels      map[string]string
	Annotations map[string]string
	DisplayName string
}

func configuredConnectorResource(value configuredConnector) cschema.Connector {
	resource := cschema.Connector{
		TypeMeta: meta.NewTypeMeta(cschema.ConnectorKind),
		Metadata: meta.ObjectMeta{
			ID:          value.Id.String(),
			Name:        scommon.ResourceName(value.Name),
			Generation:  value.Generation,
			Labels:      value.Labels,
			Annotations: value.Annotations,
		},
		Spec: cschema.ConnectorSpec{
			Release:    cschema.ConnectorReleaseSpec{DesiredState: cschema.ConnectorReleaseState(value.State)},
			Definition: cschema.ConnectorDefinition{DisplayName: value.DisplayName},
		},
	}
	if value.Namespace != nil {
		resource.Metadata.Namespace = *value.Namespace
	}
	return resource
}

func configuredConnectorResources(values []configuredConnector) []cschema.Connector {
	resources := make([]cschema.Connector, len(values))
	for i, value := range values {
		resources[i] = configuredConnectorResource(value)
	}
	return resources
}

func appendConfiguredConnector(resources []cschema.Connector, value configuredConnector) []cschema.Connector {
	return append(resources, configuredConnectorResource(value))
}

func configuredRateLimit(id, name, namespace string) rlschema.RateLimit {
	return rlschema.RateLimit{
		TypeMeta: meta.NewTypeMeta(rlschema.RateLimitKind),
		Metadata: meta.ObjectMeta{
			ID:        id,
			Name:      scommon.ResourceName(name),
			Namespace: namespace,
		},
		Spec: rlschema.RateLimitSpec{
			Selector: rlschema.Selector{},
			Bucket:   rlschema.Bucket{},
			Algorithm: rlschema.Algorithm{
				TokenBucket: &rlschema.TokenBucket{Capacity: 10, RefillRate: 1},
			},
		},
	}
}

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

	setupRoot := func(t *testing.T, root *cfgschema.Root) func() {
		root.DevSettings = &cfgschema.DevSettings{
			Enabled:                  true,
			FakeEncryption:           true,
			FakeEncryptionSkipBase64: true,
		}
		cfg = config.FromRoot(root)

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

	setup := func(t *testing.T, connectors []configuredConnector) func() {
		return setupRoot(t, &cfgschema.Root{
			DevSettings: &cfgschema.DevSettings{
				Enabled:                  true,
				FakeEncryption:           true,
				FakeEncryptionSkipBase64: true,
			},
			Connectors: &cfgschema.Connectors{
				LoadFromList: configuredConnectorResources(connectors),
			},
		})
	}

	t.Run("rate limits", func(t *testing.T) {
		ns := "root.acme"
		first := configuredRateLimit("rl_test0000000000001", "tenant-default", ns)
		second := configuredRateLimit("", "salesforce", ns)
		second.Spec.Scope = &rlschema.RateLimitScope{ConnectorRef: &meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       cschema.ConnectorKind,
			Name:       "salesforce",
			Namespace:  ns,
		}}
		root := &cfgschema.Root{
			Connectors: &cfgschema.Connectors{LoadFromList: configuredConnectorResources([]configuredConnector{{
				Name:        "salesforce",
				Namespace:   &ns,
				DisplayName: "Salesforce",
			}})},
			RateLimits: &cfgschema.RateLimits{LoadFromList: []rlschema.RateLimit{first, second}},
		}
		cleanup := setupRoot(t, root)
		defer cleanup()

		require.NoError(t, service.Migrate(context.Background()))

		storedFirst, err := db.GetRateLimit(context.Background(), apid.MustParse("rl_test0000000000001"))
		require.NoError(t, err)
		require.Equal(t, scommon.ResourceName("tenant-default"), storedFirst.Name)
		require.Equal(t, "config", storedFirst.Labels[rateLimitSourceLabelKey])

		storedSecondPage := db.ListRateLimitsBuilder().ForNamespaceMatchers([]string{ns}).ForName("salesforce").FetchPage(context.Background())
		require.NoError(t, storedSecondPage.Error)
		require.Len(t, storedSecondPage.Results, 1)
		storedSecond := storedSecondPage.Results[0]
		require.NotNil(t, storedSecond.Definition.Scope)
		require.NotNil(t, storedSecond.Definition.Scope.ConnectorRef)
		require.NotEmpty(t, storedSecond.Definition.Scope.ConnectorRef.ID)
		require.Empty(t, storedSecond.Definition.Scope.ConnectorRef.Name)
		require.Zero(t, storedSecond.Definition.Scope.ConnectorRef.Generation)

		// Reconciliation updates the same explicit identity instead of creating
		// another row, and a removed config-owned resource is cleaned up.
		root.RateLimits.LoadFromList[0].Metadata.Name = "tenant-renamed"
		root.RateLimits.LoadFromList[0].Metadata.Labels = map[string]string{"team": "platform"}
		root.RateLimits.LoadFromList[0].Spec.Mode = rlschema.ModeObserve
		root.RateLimits.LoadFromList = root.RateLimits.LoadFromList[:1]

		apiID := apid.MustParse("rl_test0000000000099")
		require.NoError(t, db.CreateRateLimit(context.Background(), &database.RateLimit{
			Id:         apiID,
			Namespace:  ns,
			Name:       "api-owned",
			Definition: configuredRateLimit("", "", ns).Spec,
		}))
		require.NoError(t, service.Migrate(context.Background()))

		storedFirst, err = db.GetRateLimit(context.Background(), apid.MustParse("rl_test0000000000001"))
		require.NoError(t, err)
		require.Equal(t, scommon.ResourceName("tenant-renamed"), storedFirst.Name)
		require.Equal(t, rlschema.ModeObserve, storedFirst.Definition.Mode)
		require.Equal(t, "platform", storedFirst.Labels["team"])
		_, err = db.GetRateLimit(context.Background(), storedSecond.Id)
		require.ErrorIs(t, err, database.ErrNotFound)
		_, err = db.GetRateLimit(context.Background(), apiID)
		require.NoError(t, err, "API-owned rate limits must not be removed by config reconciliation")
	})

	t.Run("connectors", func(t *testing.T) {
		t.Run("no connectors", func(t *testing.T) {
			cleanup := setup(t, []configuredConnector{})
			defer cleanup()

			err := service.MigrateConnectors(context.Background())
			assert.NoError(t, err)

			type connectorResult struct {
				Id         string
				Generation int64
				State      string
			}

			assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state FROM connector_generations;
			`, []connectorResult{})
		})

		t.Run("names reconcile connector identity", func(t *testing.T) {
			t.Run("annotation changes preserve the generated connector id", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{{
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

				cfg.GetRoot().Connectors.LoadFromList[0].Metadata.Annotations["example.com/owner"] = "after@example.com"
				require.NoError(t, service.MigrateConnectors(context.Background()))

				result := db.ListConnectorsBuilder().ForName("configured").FetchPage(context.Background())
				require.NoError(t, result.Error)
				require.Len(t, result.Results, 1)
				require.Equal(t, generatedID, result.Results[0].Id)
				require.Equal(t, "after@example.com", result.Results[0].Annotations["example.com/owner"])

				generations := db.ListConnectorGenerationsBuilder().ForId(generatedID).FetchPage(context.Background())
				require.NoError(t, generations.Error)
				require.Len(t, generations.Results, 1, "metadata-only changes must not create a connector generation")
			})

			t.Run("label changes preserve the generated connector id", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{{
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

				cfg.GetRoot().Connectors.LoadFromList[0].Metadata.Labels["type"] = "after"
				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "After"
				require.NoError(t, service.MigrateConnectors(context.Background()))

				result := db.ListConnectorsBuilder().ForName("configured").FetchPage(context.Background())
				require.NoError(t, result.Error)
				require.Len(t, result.Results, 1)
				require.Equal(t, generatedID, result.Results[0].Id)
				require.Equal(t, "after", result.Results[0].Labels["type"])

				generations := db.ListConnectorGenerationsBuilder().ForId(generatedID).FetchPage(context.Background())
				require.NoError(t, generations.Error)
				require.Len(t, generations.Results, 2)
			})

			t.Run("explicit id can rename without creating a generation", func(t *testing.T) {
				connectorID := apid.MustParse("cxr_test0000000000001")
				cleanup := setup(t, []configuredConnector{{
					Id:          connectorID,
					Name:        "before",
					Labels:      map[string]string{"type": "same"},
					DisplayName: "Unchanged definition",
				}})
				defer cleanup()

				require.NoError(t, service.MigrateConnectors(context.Background()))
				cfg.GetRoot().Connectors.LoadFromList[0].Metadata.Name = "after"
				require.NoError(t, service.MigrateConnectors(context.Background()))

				renamed := db.ListConnectorsBuilder().ForName("after").FetchPage(context.Background())
				require.NoError(t, renamed.Error)
				require.Len(t, renamed.Results, 1)
				require.Equal(t, connectorID, renamed.Results[0].Id)
				require.Equal(t, "after", renamed.Results[0].Labels["apxy/cxr/-/name"])

				generations := db.ListConnectorGenerationsBuilder().ForId(connectorID).FetchPage(context.Background())
				require.NoError(t, generations.Error)
				require.Len(t, generations.Results, 1)
			})

			t.Run("metadata and release changes do not create a generation", func(t *testing.T) {
				connectorID := apid.MustParse("cxr_test0000000000001")
				cleanup := setup(t, []configuredConnector{{
					Id:          connectorID,
					Name:        "configured",
					State:       "draft",
					Labels:      map[string]string{"environment": "demo"},
					Annotations: map[string]string{"example.com/owner": "integrations"},
					DisplayName: "Unchanged definition",
				}})
				defer cleanup()

				require.NoError(t, service.MigrateConnectors(context.Background()))
				resource := &cfg.GetRoot().Connectors.LoadFromList[0]
				resource.Metadata.Labels = map[string]string{"environment": "production"}
				resource.Metadata.Annotations = map[string]string{"example.com/owner": "platform"}
				resource.Spec.Release.DesiredState = cschema.ConnectorReleaseStatePrimary
				require.NoError(t, service.MigrateConnectors(context.Background()))

				generations := db.ListConnectorGenerationsBuilder().ForId(connectorID).FetchPage(context.Background())
				require.NoError(t, generations.Error)
				require.Len(t, generations.Results, 1)
				require.Equal(t, database.ConnectorGenerationStatePrimary, generations.Results[0].State)
				userLabels, _ := database.SplitUserAndApxyLabels(generations.Results[0].Labels)
				require.Equal(t, database.Labels{"environment": "production"}, userLabels)
				require.Equal(t, database.Annotations{"example.com/owner": "platform"}, generations.Results[0].Annotations)
			})

			t.Run("explicit id can rename while adding a generation", func(t *testing.T) {
				connectorID := apid.MustParse("cxr_test0000000000001")
				cleanup := setup(t, []configuredConnector{{
					Id:          connectorID,
					Name:        "before",
					Generation:  1,
					Labels:      map[string]string{"type": "same"},
					DisplayName: "Generation one",
				}})
				defer cleanup()

				require.NoError(t, service.MigrateConnectors(context.Background()))
				cfg.GetRoot().Connectors.LoadFromList[0].Metadata.Name = "after"
				cfg.GetRoot().Connectors.LoadFromList[0].Metadata.Generation = 2
				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "Generation two"
				require.NoError(t, service.MigrateConnectors(context.Background()))

				renamed := db.ListConnectorsBuilder().ForName("after").FetchPage(context.Background())
				require.NoError(t, renamed.Error)
				require.Len(t, renamed.Results, 1)
				require.Equal(t, connectorID, renamed.Results[0].Id)

				generations := db.ListConnectorGenerationsBuilder().ForId(connectorID).FetchPage(context.Background())
				require.NoError(t, generations.Error)
				require.Len(t, generations.Results, 2)
			})

			t.Run("same name in different namespaces creates different connectors", func(t *testing.T) {
				firstNamespace := "root.first"
				secondNamespace := "root.second"
				cleanup := setup(t, []configuredConnector{
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

		t.Run("id and generation", func(t *testing.T) {
			t.Run("single initial", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:         apid.MustParse("cxr_test0000000000001"),
						Generation: 1,
						Labels:     map[string]string{"type": "fake"},
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id         string
					Generation int64
					State      string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state FROM connector_generations;
		`, []connectorResult{
					{
						Id:         "cxr_test0000000000001",
						Generation: 1,
						State:      "primary",
					},
				})
			})

			t.Run("double initial same type", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:         apid.MustParse("cxr_test0000000000001"),
						Generation: 1,
						Labels:     map[string]string{"type": "fake"},
					},
					{
						Id:         apid.MustParse("cxr_test0000000000002"),
						Generation: 1,
						Labels:     map[string]string{"type": "fake"},
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id         string
					Generation int64
					State      string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state FROM connector_generations ORDER BY id;
		`, []connectorResult{
					{
						Id:         "cxr_test0000000000001",
						Generation: 1,
						State:      "primary",
					},
					{
						Id:         "cxr_test0000000000002",
						Generation: 1,
						State:      "primary",
					},
				})
			})

			t.Run("double initial different type", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:         apid.MustParse("cxr_test0000000000001"),
						Generation: 1,
						Labels:     map[string]string{"type": "fake1"},
					},
					{
						Id:         apid.MustParse("cxr_test0000000000002"),
						Generation: 1,
						Labels:     map[string]string{"type": "fake2"},
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id         string
					Generation int64
					State      string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state FROM connector_generations ORDER BY id;
		`, []connectorResult{
					{
						Id:         "cxr_test0000000000001",
						Generation: 1,
						State:      "primary",
					},
					{
						Id:         "cxr_test0000000000002",
						Generation: 1,
						State:      "primary",
					},
				})
			})

			t.Run("unchanged from initial", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:         apid.MustParse("cxr_test0000000000001"),
						Generation: 1,
						Labels:     map[string]string{"type": "fake"},
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id         string
					Generation int64
					State      string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state FROM connector_generations;
		`, []connectorResult{
					{
						Id:         "cxr_test0000000000001",
						Generation: 1,
						State:      "primary",
					},
				})
			})

			t.Run("changed once", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
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

				cfg.GetRoot().Connectors.LoadFromList[0].Metadata.Generation = 2
				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Generation:  1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Generation:  2,
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

			t.Run("add draft generation", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				// Draft generations can be added; non-specified generations default to primary
				cfg.GetRoot().Connectors.LoadFromList = appendConfiguredConnector(cfg.GetRoot().Connectors.LoadFromList, configuredConnector{
					Id:          apid.MustParse("cxr_test0000000000001"),
					Generation:  2,
					State:       "draft",
					Labels:      map[string]string{"type": "fake"},
					DisplayName: "changed",
				})

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Generation:  1,
						State:       "primary",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Generation:  2,
						State:       "draft",
						DisplayName: "changed",
					},
				})
			})

			t.Run("changed once then unchanged", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Metadata.Generation = 2
				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Generation:  1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Generation:  2,
						State:       "primary",
						DisplayName: "changed",
					},
				})
			})

			t.Run("changed twice", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Metadata.Generation = 2
				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Metadata.Generation = 3
				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed again"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Generation:  1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Generation:  2,
						State:       "active",
						DisplayName: "changed",
					},
					{
						Id:          "cxr_test0000000000001",
						Generation:  3,
						State:       "primary",
						DisplayName: "changed again",
					},
				})
			})

			t.Run("cannot change published generation", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Generation:  1,
						State:       "primary",
						DisplayName: "initial",
					},
				})
			})

			t.Run("does not allow duplicate id generations initial", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "first",
					},
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "second",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{})
			})

			t.Run("does not allow duplicate id generations when migrated", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "first",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList = appendConfiguredConnector(cfg.GetRoot().Connectors.LoadFromList, configuredConnector{
					Id:          apid.MustParse("cxr_test0000000000001"),
					Generation:  1,
					Labels:      map[string]string{"type": "fake"},
					DisplayName: "second",
				})

				err = service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Generation:  1,
						State:       "primary",
						DisplayName: "first",
					},
				})
			})
		})

		t.Run("id", func(t *testing.T) {
			t.Run("single initial", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:     apid.MustParse("cxr_test0000000000001"),
						Labels: map[string]string{"type": "fake"},
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id         string
					Generation int64
					State      string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state FROM connector_generations;
		`, []connectorResult{
					{
						Id:         "cxr_test0000000000001",
						Generation: 1,
						State:      "primary",
					},
				})
			})

			t.Run("double initial same type", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
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
					Id         string
					Generation int64
					State      string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state FROM connector_generations ORDER BY id;
		`, []connectorResult{
					{
						Id:         "cxr_test0000000000001",
						Generation: 1,
						State:      "primary",
					},
					{
						Id:         "cxr_test0000000000002",
						Generation: 1,
						State:      "primary",
					},
				})
			})

			t.Run("unchanged from initial", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
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
					Id         string
					Generation int64
					State      string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state FROM connector_generations;
		`, []connectorResult{
					{
						Id:         "cxr_test0000000000001",
						Generation: 1,
						State:      "primary",
					},
				})
			})

			t.Run("changed once", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Generation:  1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Generation:  2,
						State:       "primary",
						DisplayName: "changed",
					},
				})
			})

			t.Run("add draft generation", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList = appendConfiguredConnector(cfg.GetRoot().Connectors.LoadFromList, configuredConnector{
					Id:          apid.MustParse("cxr_test0000000000001"),
					Labels:      map[string]string{"type": "fake"},
					State:       "draft",
					DisplayName: "changed",
				})

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Generation:  1,
						State:       "primary",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Generation:  2,
						State:       "draft",
						DisplayName: "changed",
					},
				})
			})

			t.Run("changed once then unchanged", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Generation:  1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Generation:  2,
						State:       "primary",
						DisplayName: "changed",
					},
				})
			})

			t.Run("changed twice", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed again"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Generation:  1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Id:          "cxr_test0000000000001",
						Generation:  2,
						State:       "active",
						DisplayName: "changed",
					},
					{
						Id:          "cxr_test0000000000001",
						Generation:  3,
						State:       "primary",
						DisplayName: "changed again",
					},
				})
			})

			t.Run("does not allow duplicate id initial", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
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
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{})
			})

			t.Run("does not allow duplicate id when migrated", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "first",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList = appendConfiguredConnector(cfg.GetRoot().Connectors.LoadFromList, configuredConnector{
					Id:          apid.MustParse("cxr_test0000000000001"),
					Labels:      map[string]string{"type": "fake"},
					DisplayName: "second",
				})

				err = service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Id:          "cxr_test0000000000001",
						Generation:  1,
						State:       "primary",
						DisplayName: "first",
					},
				})
			})
		})

		t.Run("name and generation", func(t *testing.T) {
			t.Run("changed once preserves generated id", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Name:        "fake",
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Metadata.Generation = 2
				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				rows, err := rawDb.Query(withDisplayNameExpr(cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name
			FROM connector_generations
			WHERE connector_id IN (SELECT id FROM connectors WHERE deleted_at IS NULL)
			ORDER BY generation;
		`))
				require.NoError(t, err)
				defer rows.Close()

				var results []connectorResult
				for rows.Next() {
					var result connectorResult
					require.NoError(t, rows.Scan(&result.Id, &result.Generation, &result.State, &result.DisplayName))
					results = append(results, result)
				}
				require.NoError(t, rows.Err())
				require.Len(t, results, 2)
				require.Equal(t, results[0].Id, results[1].Id)
				require.Equal(t, connectorResult{
					Id:          results[0].Id,
					Generation:  1,
					State:       "active",
					DisplayName: "initial",
				}, results[0])
				require.Equal(t, connectorResult{
					Id:          results[0].Id,
					Generation:  2,
					State:       "primary",
					DisplayName: "changed",
				}, results[1])
			})

			t.Run("initial generation must start at one", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Name:        "fake",
						Generation:  2,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Id         string
					Generation int64
					State      string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state FROM connector_generations WHERE connector_id IN (SELECT id FROM connectors WHERE deleted_at IS NULL);
		`, []connectorResult{})
			})

			t.Run("cannot change published generation", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Name:        "fake",
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT generation, state, DISPLAY_NAME_EXPR as display_name
			FROM connector_generations
			WHERE connector_id IN (SELECT id FROM connectors WHERE deleted_at IS NULL)
			ORDER BY generation;
		`, []connectorResult{
					{
						Generation:  1,
						State:       "primary",
						DisplayName: "initial",
					},
				})
			})
		})

		t.Run("name only", func(t *testing.T) {
			t.Run("single initial", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Name:   "fake",
						Labels: map[string]string{"type": "fake"},
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Generation int64
					State      string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT generation, state FROM connector_generations;
		`, []connectorResult{
					{
						Generation: 1,
						State:      "primary",
					},
				})
			})

			t.Run("unchanged initial", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
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
					Generation int64
					State      string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT generation, state FROM connector_generations;
		`, []connectorResult{
					{
						Generation: 1,
						State:      "primary",
					},
				})
			})

			t.Run("changed once", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Generation:  1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Generation:  2,
						State:       "primary",
						DisplayName: "changed",
					},
				})
			})

			t.Run("changed once then unchanged", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Generation:  1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Generation:  2,
						State:       "primary",
						DisplayName: "changed",
					},
				})
			})

			t.Run("changed twice", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "initial",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList[0].Spec.Definition.DisplayName = "changed again"

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Generation:  1,
						State:       "active",
						DisplayName: "initial",
					},
					{
						Generation:  2,
						State:       "active",
						DisplayName: "changed",
					},
					{
						Generation:  3,
						State:       "primary",
						DisplayName: "changed again",
					},
				})
			})

			t.Run("does not allow duplicate name without id initial", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
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
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{})
			})

			t.Run("does not allow duplicate name without id when migrated", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "first",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList = appendConfiguredConnector(cfg.GetRoot().Connectors.LoadFromList, configuredConnector{
					Name:        "fake",
					Labels:      map[string]string{"type": "fake"},
					DisplayName: "second",
				})

				err = service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{
					{
						Generation:  1,
						State:       "primary",
						DisplayName: "first",
					},
				})
			})
		})

		t.Run("bad config files", func(t *testing.T) {
			t.Run("duplicate id generation type", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{})
			})

			t.Run("duplicate id generation state primary", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						State:       "primary",
						Labels:      map[string]string{"type": "fake1"},
						DisplayName: "duplicate",
					},
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
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
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{})
			})

			t.Run("duplicate id generation state draft", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						State:       "draft",
						Labels:      map[string]string{"type": "fake1"},
						DisplayName: "duplicate",
					},
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
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
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{})
			})

			t.Run("duplicate id generation", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						Labels:      map[string]string{"type": "fake1"},
						DisplayName: "duplicate",
					},
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
						Labels:      map[string]string{"type": "fake2"},
						DisplayName: "duplicate",
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.Error(t, err)

				type connectorResult struct {
					Id          string
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{})
			})

			t.Run("id with and without generation", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Generation:  1,
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
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{})
			})

			t.Run("id generation and name without id", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Name:        "fake",
						Generation:  1,
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

				cleanup2 := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Name:        "fake",
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
					{
						Name:        "fake",
						Generation:  2,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
				})
				defer cleanup2()

				err = service.MigrateConnectors(context.Background())
				require.Error(t, err)

				cleanup3 := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Name:        "fake",
						Generation:  1,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
					{
						Name:        "fake",
						Generation:  2,
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
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{})
			})

			t.Run("id and name without id", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
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

				cleanup2 := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
					{
						Name:        "fake",
						Generation:  2,
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
				})
				defer cleanup2()

				err = service.MigrateConnectors(context.Background())
				require.Error(t, err)

				cleanup3 := setup(t, []configuredConnector{
					{
						Id:          apid.MustParse("cxr_test0000000000001"),
						Name:        "fake",
						Labels:      map[string]string{"type": "fake"},
						DisplayName: "duplicate",
					},
					{
						Name:        "fake",
						Generation:  2,
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
					Generation  int64
					State       string
					DisplayName string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state, DISPLAY_NAME_EXPR as display_name FROM connector_generations ORDER BY generation;
		`, []connectorResult{})
			})
		})

		t.Run("orphan cleanup", func(t *testing.T) {
			t.Run("config-sourced connector with no connections is removed", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:         apid.MustParse("cxr_test0000000000001"),
						Generation: 1,
						Labels:     map[string]string{"type": "fake1"},
					},
					{
						Id:         apid.MustParse("cxr_test0000000000002"),
						Generation: 1,
						Labels:     map[string]string{"type": "fake2"},
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
					Id         string
					Generation int64
					State      string
				}

				// The orphan's row is soft-deleted; only the surviving connector remains.
				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state FROM connector_generations WHERE connector_id IN (SELECT id FROM connectors WHERE deleted_at IS NULL) ORDER BY id;
		`, []connectorResult{
					{Id: "cxr_test0000000000001", Generation: 1, State: "primary"},
				})
			})

			t.Run("config-sourced connector with live connections is demoted", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:         apid.MustParse("cxr_test0000000000001"),
						Generation: 1,
						Labels:     map[string]string{"type": "fake1"},
					},
					{
						Id:         apid.MustParse("cxr_test0000000000002"),
						Generation: 1,
						Labels:     map[string]string{"type": "fake2"},
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				// Create a connection against the connector we are about to drop.
				err = db.CreateConnection(context.Background(), &database.Connection{
					Id:                  apid.MustParse("cxn_test0000000000001"),
					Namespace:           "root",
					ConnectorId:         apid.MustParse("cxr_test0000000000002"),
					ConnectorGeneration: 1,
					State:               database.ConnectionStateConfigured,
				})
				require.NoError(t, err)

				cfg.GetRoot().Connectors.LoadFromList = cfg.GetRoot().Connectors.LoadFromList[:1]

				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id         string
					Generation int64
					State      string
				}

				// Orphan must NOT be deleted (still rows present), and its primary generation
				// must be demoted to active.
				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state FROM connector_generations WHERE connector_id IN (SELECT id FROM connectors WHERE deleted_at IS NULL) ORDER BY id;
		`, []connectorResult{
					{Id: "cxr_test0000000000001", Generation: 1, State: "primary"},
					{Id: "cxr_test0000000000002", Generation: 1, State: "active"},
				})
			})

			t.Run("api-created connectors are not touched", func(t *testing.T) {
				cleanup := setup(t, []configuredConnector{
					{
						Id:         apid.MustParse("cxr_test0000000000001"),
						Generation: 1,
						Labels:     map[string]string{"type": "fake1"},
					},
				})
				defer cleanup()

				err := service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				// Insert a connector generation directly via the database, simulating
				// an API-driven create. It carries no apxy/cxr/source label.
				apiId := apid.MustParse("cxr_test0000000000099")
				err = db.UpsertConnectorGeneration(context.Background(), &database.ConnectorWithDefinition{
					Id:                  apiId,
					Generation:          1,
					Namespace:           "root",
					State:               database.ConnectorGenerationStatePrimary,
					EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "api-created"},
					Labels:              database.Labels{"type": "api-only"},
				})
				require.NoError(t, err)

				// Re-run migration with the same config — the API-created connector
				// should remain untouched even though it is not in the config.
				err = service.MigrateConnectors(context.Background())
				require.NoError(t, err)

				type connectorResult struct {
					Id         string
					Generation int64
					State      string
				}

				assertSqlWithDisplayName(t, rawDb, cfg, `
			SELECT connector_id AS id, generation, state FROM connector_generations WHERE connector_id IN (SELECT id FROM connectors WHERE deleted_at IS NULL) ORDER BY id;
		`, []connectorResult{
					{Id: "cxr_test0000000000001", Generation: 1, State: "primary"},
					{Id: "cxr_test0000000000099", Generation: 1, State: "primary"},
				})
			})
		})
	})

	t.Run("namespaces", func(t *testing.T) {
		t.Run("includes configured actor namespaces", func(t *testing.T) {
			cleanup := setup(t, []configuredConnector{})
			defer cleanup()

			actor := actorschema.NewActor()
			actor.Metadata.Namespace = "root.smoke"
			actor.Spec.ExternalId = "smoke-user"
			actor.Spec.SigningKey = &cfgschema.Key{
				InnerVal: &cfgschema.KeyShared{
					SharedKey: &cfgschema.KeyData{
						InnerVal: &cfgschema.KeyDataBase64Val{Base64: "dGVzdA=="},
					},
				},
			}
			cfg.GetRoot().SystemAuth.Actors = &cfgschema.ConfiguredActors{
				InnerVal: cfgschema.ConfiguredActorsList{actor},
			}

			require.NoError(t, service.Migrate(context.Background()))
			_, err := db.GetNamespace(context.Background(), "root.smoke")
			require.NoError(t, err)
		})
	})
}

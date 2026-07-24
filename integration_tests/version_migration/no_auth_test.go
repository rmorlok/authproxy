//go:build integration

package version_migration

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const noAuthMigrationTimeout = 20 * time.Second

func TestNoAuthVersionMigrationAppliesDefaultsAndRunsAllTargetProbes(t *testing.T) {
	var existingProbeCalls atomic.Int64
	var addedProbeCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/existing":
			existingProbeCalls.Add(1)
		case "/added":
			addedProbeCalls.Add(1)
		default:
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)

	env := helpers.Setup(t, helpers.SetupOptions{Service: helpers.ServiceTypeAPI})
	t.Cleanup(env.Cleanup)
	helpers.StartCoreWorkflowWorker(t, env)

	created := env.CreateConnector(
		t,
		noAuthMigrationConnectorDefinition(
			"no-auth-migration-v1",
			nil,
			[]connectors.Probe{migrationHTTPProbe("existing", upstream.URL+"/existing")},
		),
		nil,
		nil,
	)
	require.Equal(t, uint64(1), created.Version)
	primary := env.ForceConnectorVersionState(t, created.Id, created.Version, schemaapi.ConnectorVersionStatePrimary)
	require.Equal(t, schemaapi.ConnectorVersionStatePrimary, primary.State)

	connectionID := env.InitiateNoAuthConnection(t, created.Id)
	initial := env.GetConnection(t, connectionID)
	require.Equal(t, uint64(1), initial.ConnectorVersion)
	require.Equal(t, database.ConnectionStateConfigured, initial.State)
	require.Equal(t, database.ConnectionHealthStateHealthy, initial.HealthState)
	require.Nil(t, initial.SetupStep)
	require.Nil(t, initial.SetupError)
	require.Zero(t, existingProbeCalls.Load())
	require.Zero(t, addedProbeCalls.Load())

	published := env.PublishConnectorVersion(
		t,
		created.Id,
		noAuthMigrationConnectorDefinition(
			"no-auth-migration-v2",
			defaultedConfigureFlow("region", "us-east-1"),
			[]connectors.Probe{
				migrationHTTPProbe("existing", upstream.URL+"/existing"),
				migrationHTTPProbe("added", upstream.URL+"/added"),
			},
		),
		nil,
		nil,
	)
	require.Equal(t, uint64(2), published.Version)

	migration := env.MigrateConnectionVersionAndWait(t, connectionID, 2, noAuthMigrationTimeout)
	require.Equal(t, uint64(1), migration.SourceVersion)
	require.Equal(t, uint64(2), migration.TargetVersion)

	migrated := env.GetConnection(t, connectionID)
	require.Equal(t, uint64(2), migrated.ConnectorVersion)
	require.Equal(t, database.ConnectionStateConfigured, migrated.State)
	require.Equal(t, database.ConnectionHealthStateHealthy, migrated.HealthState)
	require.Nil(t, migrated.SetupStep)
	require.Nil(t, migrated.SetupError)

	cfg := env.DecryptConnectionConfiguration(t, connectionID)
	assert.Equal(t, "us-east-1", cfg["region"])
	assert.Equal(t, int64(1), existingProbeCalls.Load(), "the retained target probe should run after migration")
	assert.Equal(t, int64(1), addedProbeCalls.Load(), "the newly added target probe should run after migration")
	env.RequireNoActiveConnectionNotifications(t, connectionID)
}

func noAuthMigrationConnectorDefinition(
	displayName string,
	setupFlow *connectors.SetupFlow,
	probes []connectors.Probe,
) sconfig.Connector {
	connector := helpers.NewNoAuthConnector(apid.New(apid.PrefixConnectorVersion), displayName, nil)
	connector.SetupFlow = setupFlow
	connector.Probes = probes
	return connector
}

func defaultedConfigureFlow(fieldName, value string) *connectors.SetupFlow {
	return &connectors.SetupFlow{
		Configure: &connectors.SetupFlowPhase{
			Steps: []connectors.SetupFlowStep{{
				Id:    "configure-defaults",
				Title: "Defaults",
				JsonSchema: common.RawJSON(`{
					"type": "object",
					"properties": {
						"` + fieldName + `": {
							"type": "string",
							"default": "` + value + `"
						}
					},
					"required": ["` + fieldName + `"],
					"additionalProperties": false
				}`),
			}},
		},
	}
}

func migrationHTTPProbe(id, url string) connectors.Probe {
	return connectors.Probe{
		Id: id,
		Http: &connectors.ProbeHttp{
			Method: http.MethodGet,
			URL:    url,
		},
	}
}

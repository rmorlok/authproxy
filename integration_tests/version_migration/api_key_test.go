//go:build integration

package version_migration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

const (
	apiKeyMigrationTimeout       = 20 * time.Second
	configureWorkspaceStepID     = "configure-workspace"
	preconnectRegionStepID       = "select-region"
	apiKeyMigrationCredentialKey = "version-migration-api-key"
)

func TestApiKeyVersionMigrationConfigureChangeRequiresSetup(t *testing.T) {
	stub := helpers.NewApiKeyStubUpstream(t, helpers.ApiKeyStubOptions{
		Placement:   connectors.ApiKeyPlacementBearer,
		AcceptedKey: apiKeyMigrationCredentialKey,
	})

	env := helpers.Setup(t, helpers.SetupOptions{Service: helpers.ServiceTypeAPI})
	defer env.Cleanup()
	helpers.StartCoreWorkflowWorker(t, env)

	connectorID := createPrimaryAPIKeyConnector(t, env, "api-key-migration-configure-v1", stub, nil)
	connectionID := createHealthyAPIKeyConnection(t, env, connectorID, apiKeyMigrationCredentialKey)

	v2 := apiKeyMigrationConnectorDefinition("api-key-migration-configure-v2", stub, &connectors.SetupFlow{
		Configure: &connectors.SetupFlowPhase{
			Steps: []connectors.SetupFlowStep{requiredStringStep(configureWorkspaceStepID, "Workspace", "workspace")},
		},
	})
	published := env.PublishConnectorVersion(t, connectorID, v2, nil, nil)
	require.Equal(t, uint64(2), published.Version)

	env.MigrateConnectionVersionAndWait(t, connectionID, 2, apiKeyMigrationTimeout)

	migrated := env.GetConnection(t, connectionID)
	require.Equal(t, uint64(2), migrated.ConnectorVersion)
	require.Equal(t, database.ConnectionStateConfigured, migrated.State)
	require.Equal(t, database.ConnectionHealthStateHealthy, migrated.HealthState)
	require.NotNil(t, migrated.SetupStep)
	assert.Equal(t, configureWorkspaceStepID, migrated.SetupStep.Id())

	notification := env.RequireSingleActiveConnectionNotification(
		t,
		connectionID,
		helpers.NotificationKeySuffixSetupRequired,
	)
	assertNoAuthProxyCopy(t, notification)
	assert.True(t, notification.CanAction)
	assert.Contains(t, notification.ActionUrl, "action=configure")

	w := env.SubmitSetupForm(t, connectionID, configureWorkspaceStepID, map[string]any{
		"workspace": "north",
	})
	requireConnectionSetupComplete(t, w)

	afterSetup := env.GetConnection(t, connectionID)
	require.Equal(t, uint64(2), afterSetup.ConnectorVersion)
	require.Equal(t, database.ConnectionStateConfigured, afterSetup.State)
	require.Equal(t, database.ConnectionHealthStateHealthy, afterSetup.HealthState)
	require.Nil(t, afterSetup.SetupStep)
	require.Nil(t, afterSetup.SetupError)

	cfg := env.DecryptConnectionConfiguration(t, connectionID)
	assert.Equal(t, "north", cfg["workspace"])

	env.RequireNoActiveConnectionNotifications(t, connectionID)
	env.RequireResolvedConnectionNotification(t, connectionID, helpers.NotificationKeySuffixSetupRequired)
}

func TestApiKeyVersionMigrationPreconnectChangeRequiresReauth(t *testing.T) {
	const rotatedKey = "version-migration-api-key-reauth"

	stub := helpers.NewApiKeyStubUpstream(t, helpers.ApiKeyStubOptions{
		Placement:   connectors.ApiKeyPlacementBearer,
		AcceptedKey: apiKeyMigrationCredentialKey,
	})

	env := helpers.Setup(t, helpers.SetupOptions{Service: helpers.ServiceTypeAPI})
	defer env.Cleanup()
	helpers.StartCoreWorkflowWorker(t, env)

	connectorID := createPrimaryAPIKeyConnector(t, env, "api-key-migration-preconnect-v1", stub, nil)
	connectionID := createHealthyAPIKeyConnection(t, env, connectorID, apiKeyMigrationCredentialKey)

	v2 := apiKeyMigrationConnectorDefinition("api-key-migration-preconnect-v2", stub, &connectors.SetupFlow{
		Preconnect: &connectors.SetupFlowPhase{
			Steps: []connectors.SetupFlowStep{requiredStringStep(preconnectRegionStepID, "Region", "region")},
		},
	})
	published := env.PublishConnectorVersion(t, connectorID, v2, nil, nil)
	require.Equal(t, uint64(2), published.Version)

	env.MigrateConnectionVersionAndWait(t, connectionID, 2, apiKeyMigrationTimeout)

	migrated := env.GetConnection(t, connectionID)
	require.Equal(t, uint64(2), migrated.ConnectorVersion)
	require.Equal(t, database.ConnectionStateConfigured, migrated.State)
	require.Equal(t, database.ConnectionHealthStateUnhealthy, migrated.HealthState)
	require.Nil(t, migrated.SetupStep)

	notification := env.RequireSingleActiveConnectionNotification(
		t,
		connectionID,
		helpers.NotificationKeySuffixAuthRequired,
	)
	assertNoAuthProxyCopy(t, notification)
	assert.True(t, notification.CanAction)
	assert.Contains(t, notification.ActionUrl, "action=reauth")

	stub.RotateAcceptedKey(rotatedKey)

	w := env.ReauthConnection(t, connectionID)
	form := requireConnectionSetupForm(t, w)
	require.Equal(t, preconnectRegionStepID, form.StepId)

	w = env.SubmitSetupForm(t, connectionID, preconnectRegionStepID, map[string]any{
		"region": "us-east-1",
	})
	form = requireConnectionSetupForm(t, w)
	require.Equal(t, helpers.ApiKeySubmitFormStepId(), form.StepId)

	w = env.SubmitApiKeyCredentials(t, connectionID, helpers.ApiKeySubmitFormStepId(), map[string]any{
		"api_key": rotatedKey,
	})
	requireConnectionSetupVerifying(t, w)
	require.NoError(t, env.RunVerifyConnection(t, connectionID))

	afterReauth := env.GetConnection(t, connectionID)
	require.Equal(t, uint64(2), afterReauth.ConnectorVersion)
	require.Equal(t, database.ConnectionStateConfigured, afterReauth.State)
	require.Equal(t, database.ConnectionHealthStateHealthy, afterReauth.HealthState)
	require.Nil(t, afterReauth.SetupStep)
	require.Nil(t, afterReauth.SetupError)

	cfg := env.DecryptConnectionConfiguration(t, connectionID)
	assert.Equal(t, "us-east-1", cfg["region"])
	assert.Equal(t, rotatedKey, env.DecryptApiKeyCredential(t, connectionID).ApiKey)

	env.RequireNoActiveConnectionNotifications(t, connectionID)
	env.RequireResolvedConnectionNotification(t, connectionID, helpers.NotificationKeySuffixAuthRequired)
}

func createPrimaryAPIKeyConnector(
	t *testing.T,
	env *helpers.IntegrationTestEnv,
	displayName string,
	stub *helpers.ApiKeyStubUpstream,
	setupFlow *connectors.SetupFlow,
) apid.ID {
	t.Helper()

	created := env.CreateConnector(t, apiKeyMigrationConnectorDefinition(displayName, stub, setupFlow), nil, nil)
	require.Equal(t, uint64(1), created.Version)
	require.Equal(t, schemaapi.ConnectorVersionStateDraft, created.State)

	primary := env.ForceConnectorVersionState(t, created.Id, created.Version, schemaapi.ConnectorVersionStatePrimary)
	require.Equal(t, schemaapi.ConnectorVersionStatePrimary, primary.State)
	return created.Id
}

func createHealthyAPIKeyConnection(
	t *testing.T,
	env *helpers.IntegrationTestEnv,
	connectorID apid.ID,
	apiKey string,
) string {
	t.Helper()

	connectionID, form := env.InitiateApiKeyConnection(t, connectorID)
	require.Equal(t, helpers.ApiKeySubmitFormStepId(), form.StepId)

	w := env.SubmitApiKeyCredentials(t, connectionID, form.StepId, map[string]any{"api_key": apiKey})
	requireConnectionSetupVerifying(t, w)
	require.NoError(t, env.RunVerifyConnection(t, connectionID))

	conn := env.GetConnection(t, connectionID)
	require.Equal(t, uint64(1), conn.ConnectorVersion)
	require.Equal(t, database.ConnectionStateConfigured, conn.State)
	require.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState)
	require.Nil(t, conn.SetupStep)
	require.Nil(t, conn.SetupError)
	return connectionID
}

func apiKeyMigrationConnectorDefinition(
	displayName string,
	stub *helpers.ApiKeyStubUpstream,
	setupFlow *connectors.SetupFlow,
) sconfig.Connector {
	conn := helpers.NewApiKeyConnector(apid.New(apid.PrefixConnectorVersion), displayName, helpers.ApiKeyConnectorOptions{
		Placement: connectors.ApiKeyPlacementBearer,
		ProbeURL:  stub.BaseURL + "/probe",
	})
	conn.SetupFlow = setupFlow
	return conn
}

func requiredStringStep(stepID, title, fieldName string) connectors.SetupFlowStep {
	return connectors.SetupFlowStep{
		Id:    stepID,
		Title: title,
		JsonSchema: common.RawJSON(`{
			"type": "object",
			"properties": {
				"` + fieldName + `": {
					"type": "string",
					"minLength": 1
				}
			},
			"required": ["` + fieldName + `"],
			"additionalProperties": false
		}`),
	}
}

func requireConnectionSetupForm(t *testing.T, w *httptest.ResponseRecorder) schemaapi.ConnectionSetupForm {
	t.Helper()
	require.Equalf(t, http.StatusOK, w.Code, "setup request failed: %s", w.Body.String())
	requireConnectionSetupResponseType(t, w, schemaapi.ConnectionSetupResponseTypeForm)

	var form schemaapi.ConnectionSetupForm
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &form))
	return form
}

func requireConnectionSetupVerifying(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equalf(t, http.StatusOK, w.Code, "setup request failed: %s", w.Body.String())
	requireConnectionSetupResponseType(t, w, schemaapi.ConnectionSetupResponseTypeVerifying)
}

func requireConnectionSetupComplete(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equalf(t, http.StatusOK, w.Code, "setup request failed: %s", w.Body.String())
	requireConnectionSetupResponseType(t, w, schemaapi.ConnectionSetupResponseTypeComplete)
}

func requireConnectionSetupResponseType(
	t *testing.T,
	w *httptest.ResponseRecorder,
	expected schemaapi.ConnectionSetupResponseType,
) {
	t.Helper()

	var generic struct {
		Type schemaapi.ConnectionSetupResponseType `json:"type"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &generic))
	require.Equalf(t, expected, generic.Type, "unexpected setup response: %s", w.Body.String())
}

func assertNoAuthProxyCopy(t *testing.T, notification schemaapi.NotificationJson) {
	t.Helper()
	assert.NotContains(t, strings.ToLower(notification.Title), "authproxy")
	assert.NotContains(t, strings.ToLower(notification.Message), "authproxy")
}

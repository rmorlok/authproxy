//go:build integration

package api_key

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApiKeyConnection_BearerPlacement drives the full connection lifecycle
// for a bearer-placement connector — initiate → form → submit credentials →
// verify (via inline RunVerifyConnection) → ready — and confirms the stub
// upstream actually received a bearer header carrying the submitted key.
func TestApiKeyConnection_BearerPlacement(t *testing.T) {
	runPerPlacementLifecycle(t, "bearer", helpers.ApiKeyStubOptions{
		Placement:   connectors.ApiKeyPlacementBearer,
		AcceptedKey: "secret-bearer-key",
	}, helpers.ApiKeyConnectorOptions{
		Placement: connectors.ApiKeyPlacementBearer,
	}, map[string]any{
		"api_key": "secret-bearer-key",
	}, func(t *testing.T, reqs []helpers.ApiKeyStubRequest) {
		require.NotEmpty(t, reqs, "stub should have observed at least one probe request")
		latest := reqs[len(reqs)-1]
		assert.Equal(t, "Bearer secret-bearer-key", latest.Headers.Get("Authorization"),
			"probe must hit upstream with Bearer scheme carrying the submitted key")
	})
}

// TestApiKeyConnection_HeaderPlacement covers the header placement with a
// custom name + prefix.
func TestApiKeyConnection_HeaderPlacement(t *testing.T) {
	runPerPlacementLifecycle(t, "header", helpers.ApiKeyStubOptions{
		Placement:    connectors.ApiKeyPlacementHeader,
		HeaderName:   "X-Api-Key",
		HeaderPrefix: "Token ",
		AcceptedKey:  "secret-header-key",
	}, helpers.ApiKeyConnectorOptions{
		Placement:    connectors.ApiKeyPlacementHeader,
		HeaderName:   "X-Api-Key",
		HeaderPrefix: "Token ",
	}, map[string]any{
		"api_key": "secret-header-key",
	}, func(t *testing.T, reqs []helpers.ApiKeyStubRequest) {
		require.NotEmpty(t, reqs)
		latest := reqs[len(reqs)-1]
		assert.Equal(t, "Token secret-header-key", latest.Headers.Get("X-Api-Key"),
			"probe must hit upstream with the configured header carrying prefix+key")
	})
}

// TestApiKeyConnection_QueryPlacement covers query-string placement.
func TestApiKeyConnection_QueryPlacement(t *testing.T) {
	runPerPlacementLifecycle(t, "query", helpers.ApiKeyStubOptions{
		Placement:   connectors.ApiKeyPlacementQuery,
		ParamName:   "apiKey",
		AcceptedKey: "secret-query-key",
	}, helpers.ApiKeyConnectorOptions{
		Placement: connectors.ApiKeyPlacementQuery,
		ParamName: "apiKey",
	}, map[string]any{
		"api_key": "secret-query-key",
	}, func(t *testing.T, reqs []helpers.ApiKeyStubRequest) {
		require.NotEmpty(t, reqs)
		latest := reqs[len(reqs)-1]
		assert.Contains(t, latest.RawQuery, "apiKey=secret-query-key",
			"probe must hit upstream with the key in the configured query parameter")
	})
}

// TestApiKeyConnection_BasicPlacement covers basic-auth placement (username
// + password=key). The submit payload carries both fields; the stub upstream
// validates the decoded "<user>:<key>" pair.
func TestApiKeyConnection_BasicPlacement(t *testing.T) {
	runPerPlacementLifecycle(t, "basic", helpers.ApiKeyStubOptions{
		Placement:        connectors.ApiKeyPlacementBasic,
		AcceptedKey:      "secret-basic-key",
		AcceptedUsername: "alice@example.com",
	}, helpers.ApiKeyConnectorOptions{
		Placement:     connectors.ApiKeyPlacementBasic,
		UsernameField: "account_email",
	}, map[string]any{
		"api_key":       "secret-basic-key",
		"account_email": "alice@example.com",
	}, func(t *testing.T, reqs []helpers.ApiKeyStubRequest) {
		require.NotEmpty(t, reqs)
		latest := reqs[len(reqs)-1]
		got := latest.Headers.Get("Authorization")
		require.Truef(t, strings.HasPrefix(got, "Basic "),
			"probe must use Basic scheme; got %q", got)
	})
}

// runPerPlacementLifecycle wraps the placement-dispatched setup so each
// per-placement test only declares its inputs and the placement-specific
// upstream assertion. Lifecycle steps that don't vary by placement
// (initiate, submit, verify, ready, health=healthy, no-replay sanity)
// happen here.
func runPerPlacementLifecycle(
	t *testing.T,
	label string,
	stubOpts helpers.ApiKeyStubOptions,
	connectorOpts helpers.ApiKeyConnectorOptions,
	credentialPayload map[string]any,
	assertProbeRequest func(t *testing.T, reqs []helpers.ApiKeyStubRequest),
) {
	t.Helper()

	stub := helpers.NewApiKeyStubUpstream(t, stubOpts)

	// Connector definitions need a connector id; suffix with the placement
	// label so parallel runs of this helper don't collide on the id namespace.
	connectorID := apid.New(apid.PrefixConnectorVersion)
	connectorOpts.ProbeURL = stub.BaseURL + "/probe-" + label
	conn := helpers.NewApiKeyConnector(connectorID, fmt.Sprintf("api-key-%s", label), connectorOpts)

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:    helpers.ServiceTypeAPI,
		Connectors: []sconfig.Connector{conn},
	})
	defer env.Cleanup()

	connectionID, form := env.InitiateApiKeyConnection(t, connectorID)
	require.Equalf(t, helpers.ApiKeySubmitFormStepId(), form.StepId,
		"initiate should return the synthesized credentials step id; got %q", form.StepId)

	// No prior credential bytes should ever appear in a form payload —
	// even on first initiate the synthesized JSON schema is built from the
	// connector definition only. Catching it here too costs nothing and
	// guards against accidental regressions where the schema gains a
	// pre-filled default field that leaks state.
	rawSchema := string(form.JsonSchema)
	for _, v := range credentialPayload {
		if vs, ok := v.(string); ok {
			assert.NotContainsf(t, rawSchema, vs,
				"credential bytes %q must not appear in initial form schema", vs)
		}
	}

	w := env.SubmitApiKeyCredentials(t, connectionID, form.StepId, credentialPayload)
	require.Equalf(t, http.StatusOK, w.Code, "submit failed: %s", w.Body.String())

	// Submit transitions the connection into the verify phase (probes
	// pending). Run verify inline — production has the asynq worker drain
	// it; tests don't run a worker, so we drive the same code path
	// synchronously.
	require.NoError(t, env.RunVerifyConnection(t, connectionID), "verify should succeed against stub upstream")

	conn2 := env.GetConnection(t, connectionID)
	require.Equalf(t, database.ConnectionStateReady, conn2.State,
		"connection should be Ready after submit + verify; got %s (setup_step=%v, setup_error=%v)",
		conn2.State, conn2.SetupStep, conn2.SetupError)
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn2.HealthState,
		"connection should be healthy after a successful verify")

	// The probe must have actually hit the stub upstream with the submitted
	// credential — placement-specific assertion below.
	reqs := stub.Requests()
	require.NotEmpty(t, reqs, "stub upstream should have received the probe request")
	require.Greater(t, stub.SuccessCount(), int64(0), "probe should have succeeded at the stub")
	require.Zerof(t, stub.UnauthorizedCount(),
		"stub should not have rejected any requests during a clean lifecycle; got %d", stub.UnauthorizedCount())
	assertProbeRequest(t, reqs)

	// Active credential row should exist after submit (and only one — no
	// rotation has happened yet on this connection).
	id, err := apid.Parse(connectionID)
	require.NoError(t, err)
	cred, err := env.Db.GetActiveApiKeyCredential(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, id, cred.ConnectionId, "active credential should be for this connection")
	require.NotNilf(t, cred.PlacementSnapshot, "credential row should snapshot the placement at submit time")
	assert.Equalf(t, connectorOpts.Placement, cred.PlacementSnapshot.Type,
		"placement snapshot should mirror the connector's placement type")
}

//go:build integration

package api_key

import (
	"context"
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApiKeyAutoReauth walks the full unhealthy-to-healthy recovery path
// driven by probe-driven health:
//
//  1. Drive the connection to Ready with key=v1.
//  2. Stub upstream rotates its accepted secret to v2 — v1 now 401s.
//  3. A periodic probe fails — connection's health_state flips to unhealthy
//     (failure_threshold=1 for test determinism).
//  4. User submits the new key via the reauth flow.
//  5. A subsequent periodic probe succeeds — health_state flips back to
//     healthy (recovery_threshold=1).
//
// This is a single end-to-end pass that exercises both the failure and
// recovery sides of the probe-driven health logic against an api-key
// connection.
func TestApiKeyAutoReauth(t *testing.T) {
	const keyV1 = "auto-reauth-key-v1"
	const keyV2 = "auto-reauth-key-v2"

	stub := helpers.NewApiKeyStubUpstream(t, helpers.ApiKeyStubOptions{
		Placement:   connectors.ApiKeyPlacementBearer,
		AcceptedKey: keyV1,
	})

	one := 1
	connectorID := apid.New(apid.PrefixConnectorVersion)
	conn := helpers.NewApiKeyConnector(connectorID, "api-key-auto-reauth", helpers.ApiKeyConnectorOptions{
		Placement: connectors.ApiKeyPlacementBearer,
		ProbeURL:  stub.BaseURL + "/probe",
		// Tighten the thresholds so a single failure flips unhealthy and a
		// single success flips back. Without this, the default
		// failure_threshold (3) would require three failed probe runs to
		// observe the transition.
		ProbeFailureThreshold:  &one,
		ProbeRecoveryThreshold: &one,
	})

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:    helpers.ServiceTypeAPI,
		Connectors: []sconfig.Connector{conn},
	})
	defer env.Cleanup()

	// Drive to Ready with the v1 key.
	connectionID, form := env.InitiateApiKeyConnection(t, connectorID)
	w := env.SubmitApiKeyCredentials(t, connectionID, form.StepId, map[string]any{"api_key": keyV1})
	require.Equalf(t, http.StatusOK, w.Code, "submit failed: %s", w.Body.String())
	require.NoError(t, env.RunVerifyConnection(t, connectionID))

	cReady := env.GetConnection(t, connectionID)
	require.Equal(t, database.ConnectionStateConfigured, cReady.State)
	require.Equal(t, database.ConnectionHealthStateHealthy, cReady.HealthState,
		"connection should be healthy after a successful verify")

	// Simulate provider-side rotation. The stored credential (v1) is now
	// invalid against the upstream — the next probe will 401.
	stub.RotateAcceptedKey(keyV2)

	// One failed probe must flip health to unhealthy (failure_threshold=1).
	require.Error(t, env.RunProbe(t, connectionID, "verify-credential"),
		"probe should fail after upstream rotation invalidated the stored key")

	cUnhealthy := env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionStateConfigured, cUnhealthy.State,
		"state should still be Ready — only health_state flips")
	require.Equal(t, database.ConnectionHealthStateUnhealthy, cUnhealthy.HealthState,
		"connection should be unhealthy after one failed probe (threshold=1)")

	// User submits the new key via reauth — same UI flow as TestApiKey-
	// ManualRotation, just driven against an unhealthy connection.
	w = env.ReauthConnection(t, connectionID)
	require.Equalf(t, http.StatusOK, w.Code, "reauth failed: %s", w.Body.String())

	w = env.SubmitApiKeyCredentials(t, connectionID, helpers.ApiKeySubmitFormStepId(), map[string]any{
		"api_key": keyV2,
	})
	require.Equalf(t, http.StatusOK, w.Code, "rotation submit failed: %s", w.Body.String())

	// Verify the new credential — onVerifyPassed marks healthy too, but
	// the issue specifically calls out the probe-driven recovery path.
	// To exercise it cleanly, advance through verify first (sets healthy
	// once) and then assert a follow-up periodic probe is also recorded
	// as a successful outcome.
	require.NoError(t, env.RunVerifyConnection(t, connectionID))

	cRecovered := env.GetConnection(t, connectionID)
	require.Equal(t, database.ConnectionStateConfigured, cRecovered.State)
	require.Equal(t, database.ConnectionHealthStateHealthy, cRecovered.HealthState,
		"connection should be healthy again after submitting the new key")

	// A periodic probe with the rotated credential should still succeed —
	// closing the loop on recovery (recovery_threshold=1).
	require.NoError(t, env.RunProbe(t, connectionID, "verify-credential"),
		"probe should now succeed against the rotated upstream with the new key")

	cFinal := env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionHealthStateHealthy, cFinal.HealthState,
		"connection should remain healthy after a successful periodic probe")

	// Active credential is the new one.
	id, err := apid.Parse(connectionID)
	require.NoError(t, err)
	cred, err := env.Db.GetActiveApiKeyCredential(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, cred)
	assert.Equal(t, keyV2, env.DecryptApiKeyCredential(t, connectionID).ApiKey)
}

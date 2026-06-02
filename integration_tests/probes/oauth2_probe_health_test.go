//go:build integration

// Package probes holds cross-cutting integration tests that exercise the
// probe-driven health logic across auth methods. The probe-runtime is
// auth-agnostic — failure/recovery thresholds and the connection's
// health_state should behave identically regardless of whether credentials
// were established via OAuth2 or api-key. The api-key half lives in
// integration_tests/api_key/auto_reauth_test.go; this file proves the same
// invariants hold against an OAuth2 connection.
package probes

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOAuth2ProbeHealth_FailureAndRecovery verifies that the probe-driven
// health-state transitions behave identically for OAuth2 connections:
// forced probe failures flip the connection to unhealthy, and subsequent
// successes flip it back to healthy. Uses the test provider's resource
// endpoint as the probe target and scripts it to return 401 to force the
// failure window.
func TestOAuth2ProbeHealth_FailureAndRecovery(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := "probe-health-client-" + suffix
	clientSecret := "probe-health-secret-" + suffix
	userEmail := "probe-health-" + suffix + "@example.com"
	userPassword := "p4ssw0rd-" + suffix

	connectorID := apid.New(apid.PrefixConnectorVersion)

	one := 1
	probeURL := provider.ResourceURL("/probe-" + suffix)
	connector := helpers.NewOAuth2Connector(connectorID, "probe-health-test", provider, helpers.OAuth2ConnectorOptions{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Scopes:       []string{"read"},
		Probes: []connectors.Probe{{
			Id: "probe-health",
			ProxyHttp: &connectors.ProbeHttp{
				Method: "GET",
				URL:    probeURL,
			},
			// Tight thresholds so a single probe outcome flips the
			// connection on each transition — the alternative is many
			// outcome cycles to cross the default thresholds.
			FailureThreshold:  &one,
			RecoveryThreshold: &one,
		}},
	})

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:       helpers.ServiceTypeAPI,
		IncludePublic: true,
		Connectors:    []sconfig.Connector{connector},
	})
	defer env.Cleanup()

	registered := provider.CreateClient(helpers.CreateClientRequest{
		Key:                     clientKey,
		Secret:                  clientSecret,
		RedirectURI:             env.PublicOAuthCallbackURL(),
		TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
		Scope:                   "read",
	})
	require.Equal(t, clientKey, registered.Key)

	user := provider.CreateUser(helpers.CreateUserRequest{
		Username: userEmail,
		Password: userPassword,
		Email:    userEmail,
	})
	require.NotEmpty(t, user.ID)

	// Drive the OAuth2 flow to Ready. Same shortcut path the
	// proxyRefreshRig uses — /test/authorize for the consent step
	// (#169) — since we're testing health transitions, not the user-
	// facing consent leg.
	returnToURL := env.Cfg.GetRoot().Public.GetBaseUrl() + "/connections"
	connID, redirectURL := env.InitiateOAuth2Connection(t, connectorID, returnToURL)
	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID := parsed.Query().Get("state_id")
	require.NotEmpty(t, stateID)

	authResp := provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    clientKey,
		UserID:      user.ID,
		RedirectURI: env.PublicOAuthCallbackURL(),
		Scope:       "read",
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	})
	pc, err := url.Parse(authResp.RedirectURL)
	require.NoError(t, err)
	code := pc.Query().Get("code")
	require.NotEmpty(t, code)

	loc := env.DeliverOAuth2Callback(t, env.ForgeOAuth2CallbackURL(stateID, code))
	require.Truef(t, strings.HasPrefix(loc, returnToURL),
		"auth flow should land on return_to_url; got %q", loc)

	// After auth callback the connection is in verify phase (we added a
	// probe). Drive verify synchronously — production has the asynq
	// worker drain it; tests don't run a worker.
	require.NoError(t, env.RunVerifyConnection(t, connID))

	conn := env.GetConnection(t, connID)
	require.Equal(t, database.ConnectionStateConfigured, conn.State,
		"connection should land Ready after a successful verify")
	require.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState,
		"healthy is the baseline after a successful initial verify")

	// 1. Force probe failure: script the resource endpoint to return 401
	//    for the next several calls. FailCount > 1 because OAuth2's
	//    RecoverFrom401 path attempts a token refresh on a 401 and then
	//    replays the resource request — the script needs to cover both
	//    the original call and the retry so the probe sees 401 each time.
	//    Clearing the script after the probe stops further interference.
	provider.Script("", helpers.EndpointResource, helpers.ScriptAction{
		Status:    401,
		Body:      `{"error":"invalid_token"}`,
		FailCount: 10,
	})

	probeErr := env.RunProbe(t, connID, "probe-health")
	provider.ClearScripts("", helpers.EndpointResource)
	require.Error(t, probeErr, "probe must surface upstream 401 as a failure outcome")

	conn = env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateConfigured, conn.State,
		"state should still be Ready — only health_state flips on probe failures")
	require.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState,
		"failure_threshold=1: a single failed probe must flip health to unhealthy")

	// 2. Clear the failure window. Subsequent probes hit the un-scripted
	//    resource endpoint and 200.
	provider.ClearScripts("", helpers.EndpointResource)

	require.NoError(t, env.RunProbe(t, connID, "probe-health"),
		"probe should succeed once the upstream stops 401ing")

	conn = env.GetConnection(t, connID)
	require.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState,
		"recovery_threshold=1: a single successful probe must flip health back to healthy")

	// The most recent recorded probe outcome should reflect the success.
	id, err := apid.Parse(connID)
	require.NoError(t, err)
	outcomes, err := env.Db.GetRecentProbeOutcomes(context.Background(), id, "probe-health", 1)
	require.NoError(t, err)
	require.Lenf(t, outcomes, 1, "expected exactly one recent outcome row; got %d", len(outcomes))
	assert.Equalf(t, database.ProbeOutcomeStatusSuccess, outcomes[0].Outcome,
		"most recent outcome should be success after recovery; got %q", outcomes[0].Outcome)
}

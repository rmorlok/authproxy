//go:build integration

package api_key

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApiKeyProbeAcceleration_401EnqueuesProbeNow walks the central
// invariant of #257 end-to-end: an upstream 401 against a user-initiated
// proxy request enqueues a one-shot probe-now task. We assert two
// observable side-effects:
//
//  1. The throttle key is present in Redis — proves the SetNX ran (the
//     synchronization point inside EnqueueProbeNow), which means the
//     proxy's 401-detection branch fired and we reached the per-probe
//     loop.
//  2. The asynq queue contains a pending core:probe task for this
//     connection — proves the enqueue actually happened.
//
// A subsequent call from the same connection within the throttle window
// must NOT add a second task, demonstrating the throttle behavior.
func TestApiKeyProbeAcceleration_401EnqueuesProbeNow(t *testing.T) {
	const goodKey = "accel-good-key"

	stub := helpers.NewApiKeyStubUpstream(t, helpers.ApiKeyStubOptions{
		Placement:   connectors.ApiKeyPlacementBearer,
		AcceptedKey: goodKey,
	})

	connectorID := apid.New(apid.PrefixConnectorVersion)
	conn := helpers.NewApiKeyConnector(connectorID, "api-key-accel", helpers.ApiKeyConnectorOptions{
		Placement: connectors.ApiKeyPlacementBearer,
		ProbeURL:  stub.BaseURL + "/probe",
	})

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:    helpers.ServiceTypeAPI,
		Connectors: []sconfig.Connector{conn},
	})
	defer env.Cleanup()

	// Drive to Ready with the good key. Subsequent proxy traffic uses
	// this credential.
	connectionID, form := env.InitiateApiKeyConnection(t, connectorID)
	w := env.SubmitApiKeyCredentials(t, connectionID, form.StepId, map[string]any{"api_key": goodKey})
	require.Equalf(t, http.StatusOK, w.Code, "submit failed: %s", w.Body.String())
	require.NoError(t, env.RunVerifyConnection(t, connectionID))

	// Rotate the upstream so the stored key now 401s.
	stub.RotateAcceptedKey("rotated")

	// Snapshot pending task count before the user request so we can
	// attribute newly-enqueued tasks to this proxy call.
	pendingBefore := countPendingProbeTasksForConnection(t, env, connectionID)

	// Issue a proxy request — upstream returns 401. The wrapped /_proxy
	// endpoint always returns 200 with the upstream's status code carried
	// in the response body; the side-effects we care about (probe-now
	// enqueue + throttle key) fire regardless of how the body shape
	// presents the upstream status to the customer.
	resp := env.DoProxyRequest(t, connectionID, stub.BaseURL+"/something", "GET")
	require.Equalf(t, http.StatusOK, resp.Code,
		"wrapped /_proxy endpoint returns 200 with the upstream status in body; got %d body=%s",
		resp.Code, resp.Body.String())

	// The throttle key should now exist in Redis (proves the enqueue
	// path executed).
	id, err := apid.Parse(connectionID)
	require.NoError(t, err)
	throttleKey := probeNowThrottleKey(id, "verify-credential")
	exists, err := env.DM.GetRedisClient().Exists(context.Background(), throttleKey).Result()
	require.NoError(t, err)
	require.Equalf(t, int64(1), exists,
		"throttle key %q should exist in Redis after a 401-triggered enqueue", throttleKey)

	// And the asynq queue should contain one new pending probe task.
	pendingAfter := countPendingProbeTasksForConnection(t, env, connectionID)
	require.Equalf(t, pendingBefore+1, pendingAfter,
		"401 from upstream must enqueue exactly one new probe task; got %d → %d",
		pendingBefore, pendingAfter)

	// Issue a second user request while still in the throttle window.
	// The proxy still fires the acceleration path, but SetNX returns
	// false, so no new task is enqueued.
	resp2 := env.DoProxyRequest(t, connectionID, stub.BaseURL+"/something-else", "GET")
	require.Equal(t, http.StatusOK, resp2.Code, "wrapped /_proxy always returns 200")

	pendingFinal := countPendingProbeTasksForConnection(t, env, connectionID)
	assert.Equalf(t, pendingAfter, pendingFinal,
		"second 401 within throttle window must not enqueue another task; got %d → %d",
		pendingAfter, pendingFinal)
}

// TestApiKeyProbeAcceleration_2xxDoesNotEnqueue confirms the negative
// invariant: a successful (200) proxied response must NOT trigger
// probe-now, both because it would be wasted work and because the
// per-probe throttle would silently absorb a real failure that arrived
// later in the same window.
func TestApiKeyProbeAcceleration_2xxDoesNotEnqueue(t *testing.T) {
	const goodKey = "accel-2xx-key"
	stub := helpers.NewApiKeyStubUpstream(t, helpers.ApiKeyStubOptions{
		Placement:   connectors.ApiKeyPlacementBearer,
		AcceptedKey: goodKey,
	})

	connectorID := apid.New(apid.PrefixConnectorVersion)
	conn := helpers.NewApiKeyConnector(connectorID, "api-key-accel-2xx", helpers.ApiKeyConnectorOptions{
		Placement: connectors.ApiKeyPlacementBearer,
		ProbeURL:  stub.BaseURL + "/probe",
	})

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:    helpers.ServiceTypeAPI,
		Connectors: []sconfig.Connector{conn},
	})
	defer env.Cleanup()

	connectionID, form := env.InitiateApiKeyConnection(t, connectorID)
	w := env.SubmitApiKeyCredentials(t, connectionID, form.StepId, map[string]any{"api_key": goodKey})
	require.Equalf(t, http.StatusOK, w.Code, "submit failed: %s", w.Body.String())
	require.NoError(t, env.RunVerifyConnection(t, connectionID))

	pendingBefore := countPendingProbeTasksForConnection(t, env, connectionID)

	// Proxy a successful request — upstream returns 200.
	resp := env.DoProxyRequest(t, connectionID, stub.BaseURL+"/anything", "GET")
	require.Equalf(t, http.StatusOK, resp.Code, "expected 200 on a request with the valid key; got %d", resp.Code)

	id, err := apid.Parse(connectionID)
	require.NoError(t, err)
	throttleKey := probeNowThrottleKey(id, "verify-credential")
	exists, err := env.DM.GetRedisClient().Exists(context.Background(), throttleKey).Result()
	require.NoError(t, err)
	assert.Equalf(t, int64(0), exists,
		"throttle key %q must not exist after a 2xx response", throttleKey)

	pendingAfter := countPendingProbeTasksForConnection(t, env, connectionID)
	assert.Equalf(t, pendingBefore, pendingAfter,
		"2xx response must not enqueue a probe task; got %d → %d", pendingBefore, pendingAfter)
}

// probeNowThrottleKey mirrors the production helper in
// internal/core/service_probe_now.go. Duplicated here (rather than
// imported) because the integration_tests module deliberately depends only
// on iface — using the internal helper would couple test code to the core
// package. The shape is short and stable enough that drift is unlikely.
func probeNowThrottleKey(connectionId apid.ID, probeId string) string {
	return "probe_now:throttle:" + connectionId.String() + ":" + probeId
}

// countPendingProbeTasksForConnection counts pending core:probe tasks for
// the supplied connection in the default asynq queue. Filters by both task
// type and payload so accumulated tasks from other test runs (the
// integration Redis is shared across tests inside a `go test` invocation)
// don't drown out the signal. A large page size avoids pagination
// boundaries — the alternative of paging manually would compound the same
// fragility.
func countPendingProbeTasksForConnection(t *testing.T, env *helpers.IntegrationTestEnv, connectionID string) int {
	t.Helper()
	tasks, err := env.DM.GetAsyncInspector().ListPendingTasks("default", asynq.PageSize(10000))
	if err != nil {
		return 0
	}
	count := 0
	needle := []byte(connectionID)
	for _, ti := range tasks {
		if ti.Type != "core:probe" {
			continue
		}
		if !bytes.Contains(ti.Payload, needle) {
			continue
		}
		count++
	}
	_ = database.ConnectionStateReady // keep import referenced
	return count
}

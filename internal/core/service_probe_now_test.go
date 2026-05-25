package core

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	mockAsynq "github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/apid"
	mockRedisPkg "github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnqueueProbeNow_EnqueuesEachProbeWhenThrottleAllows is the happy path:
// SetNX returns true (key not yet present) for each probe → newProbeTask
// + Enqueue for each.
func TestEnqueueProbeNow_EnqueuesEachProbeWhenThrottleAllows(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "probe-now-happy",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		Probes: []cschema.Probe{
			{Id: "p1", Http: &cschema.ProbeHttp{Method: "GET", URL: "https://x.example/health"}},
			{Id: "p2", Http: &cschema.ProbeHttp{Method: "GET", URL: "https://y.example/health"}},
		},
	}
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateReady,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
	}

	svc, _, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()
	svc.r = mockRedisAllowingSetNX(ctrl, connectionId, []string{"p1", "p2"}, true)
	svc.ac = mockAsynqExpectingEnqueues(ctrl, 2)

	require.NoError(t, svc.EnqueueProbeNow(context.Background(), connectionId))
}

// TestEnqueueProbeNow_ThrottleSkipsEnqueue: SetNX returns false → throttled
// → no Enqueue call. The mock asynq client EXPECT nothing — if Enqueue were
// called, gomock's controller fails at Finish.
func TestEnqueueProbeNow_ThrottleSkipsEnqueue(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "probe-now-throttled",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		Probes: []cschema.Probe{
			{Id: "p1", Http: &cschema.ProbeHttp{Method: "GET", URL: "https://x.example/health"}},
		},
	}
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateReady,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
	}

	svc, _, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()
	svc.r = mockRedisAllowingSetNX(ctrl, connectionId, []string{"p1"}, false)
	// No Enqueue expectations — would fail at ctrl.Finish if called.
	svc.ac = mockAsynqExpectingEnqueues(ctrl, 0)

	require.NoError(t, svc.EnqueueProbeNow(context.Background(), connectionId))
}

// TestEnqueueProbeNow_MixedThrottle: two probes, one allowed and one
// throttled — only the allowed one results in an Enqueue.
func TestEnqueueProbeNow_MixedThrottle(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "probe-now-mixed",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		Probes: []cschema.Probe{
			{Id: "p1", Http: &cschema.ProbeHttp{Method: "GET", URL: "https://x.example/health"}},
			{Id: "p2", Http: &cschema.ProbeHttp{Method: "GET", URL: "https://y.example/health"}},
		},
	}
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateReady,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
	}

	svc, _, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()

	r := mockRedisPkg.NewMockClient(ctrl)
	r.EXPECT().
		SetNX(gomock.Any(), probeNowThrottleKey(connectionId, "p1"), "1", DefaultProbeNowThrottleWindow).
		Return(redis.NewBoolResult(true, nil))
	r.EXPECT().
		SetNX(gomock.Any(), probeNowThrottleKey(connectionId, "p2"), "1", DefaultProbeNowThrottleWindow).
		Return(redis.NewBoolResult(false, nil))
	svc.r = r

	svc.ac = mockAsynqExpectingEnqueues(ctrl, 1)

	require.NoError(t, svc.EnqueueProbeNow(context.Background(), connectionId))
}

// TestEnqueueProbeNow_NoProbesShortCircuits: connector without probes
// returns nil without touching Redis or asynq.
func TestEnqueueProbeNow_NoProbesShortCircuits(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "probe-now-no-probes",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
	}
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateReady,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
	}

	svc, _, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()
	// No SetNX / Enqueue expectations — neither path should be touched.

	require.NoError(t, svc.EnqueueProbeNow(context.Background(), connectionId))
}

// TestEnqueueProbeNow_RedisErrorIsSwallowedPerProbe: a Redis error from
// SetNX must not abort the loop — other probes should still be considered.
// And the error itself does not surface to the caller (best-effort
// contract).
func TestEnqueueProbeNow_RedisErrorIsSwallowedPerProbe(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "probe-now-redis-err",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		Probes: []cschema.Probe{
			{Id: "p1", Http: &cschema.ProbeHttp{Method: "GET", URL: "https://x.example/health"}},
			{Id: "p2", Http: &cschema.ProbeHttp{Method: "GET", URL: "https://y.example/health"}},
		},
	}
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateReady,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
	}

	svc, _, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()

	r := mockRedisPkg.NewMockClient(ctrl)
	r.EXPECT().
		SetNX(gomock.Any(), probeNowThrottleKey(connectionId, "p1"), "1", DefaultProbeNowThrottleWindow).
		Return(redis.NewBoolResult(false, errors.New("redis down")))
	r.EXPECT().
		SetNX(gomock.Any(), probeNowThrottleKey(connectionId, "p2"), "1", DefaultProbeNowThrottleWindow).
		Return(redis.NewBoolResult(true, nil))
	svc.r = r

	// p1 short-circuited by redis error → skipped; p2 succeeded → enqueued.
	svc.ac = mockAsynqExpectingEnqueues(ctrl, 1)

	err := svc.EnqueueProbeNow(context.Background(), connectionId)
	assert.NoError(t, err, "redis errors are best-effort, should not surface")
}

// TestEnqueueProbeNow_ConnectionNotFoundSurfacesError: unlike the per-probe
// errors that are best-effort, a missing connection is a structural
// problem — the caller (the proxy 401 path) gets the error so it can log
// it.
func TestEnqueueProbeNow_ConnectionNotFoundSurfacesError(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svc, db, _, _, _, _ := FullMockService(t, ctrl)
	db.EXPECT().
		GetConnection(gomock.Any(), connectionId).
		Return(nil, database.ErrNotFound)

	err := svc.EnqueueProbeNow(context.Background(), connectionId)
	require.Error(t, err)
}

// mockRedisAllowingSetNX returns a redis mock where SetNX against each
// probe key returns the supplied "allowed" result.
func mockRedisAllowingSetNX(ctrl *gomock.Controller, connectionId apid.ID, probeIds []string, allowed bool) *mockRedisPkg.MockClient {
	r := mockRedisPkg.NewMockClient(ctrl)
	for _, pid := range probeIds {
		r.EXPECT().
			SetNX(gomock.Any(), probeNowThrottleKey(connectionId, pid), "1", DefaultProbeNowThrottleWindow).
			Return(redis.NewBoolResult(allowed, nil))
	}
	return r
}

// mockAsynqExpectingEnqueues returns an asynq mock that expects exactly N
// EnqueueContext calls. Each call returns a no-op TaskInfo so the
// caller's path stays inside the success branch.
func mockAsynqExpectingEnqueues(ctrl *gomock.Controller, n int) *mockAsynq.MockClient {
	ac := mockAsynq.NewMockClient(ctrl)
	if n > 0 {
		ac.EXPECT().
			EnqueueContext(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&asynq.TaskInfo{}, nil).
			Times(n)
	}
	return ac
}

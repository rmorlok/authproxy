package core

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

// stubProbe is a minimal iface.Probe used only as a token for the threshold
// accessors and id — the test layer calls neither Invoke nor IsPeriodic.
type stubProbe struct {
	id             string
	failureThresh  int
	recoveryThresh int
}

func (p *stubProbe) GetId() string                              { return p.id }
func (p *stubProbe) Invoke(ctx context.Context) (string, error) { return "", nil }
func (p *stubProbe) IsPeriodic() bool                           { return true }
func (p *stubProbe) GetScheduleString() string                  { return "*/5 * * * *" }
func (p *stubProbe) EffectiveFailureThreshold() int             { return p.failureThresh }
func (p *stubProbe) EffectiveRecoveryThreshold() int            { return p.recoveryThresh }

// newProbeHealthTestConn builds a test connection whose connector definition
// carries the supplied probes. probes are used by the recovery-time threshold
// lookups against other probes for the "any probe over threshold blocks
// recovery" rule.
func newProbeHealthTestConn(
	t *testing.T,
	ctrl *gomock.Controller,
	probes []cschema.Probe,
	initialHealth database.ConnectionHealthState,
) (*connection, *mockDb.MockDB, *bytes.Buffer) {
	t.Helper()
	s, db, _, _, _, _ := FullMockService(t, ctrl)
	cv := NewTestConnectorVersion(cschema.Connector{Probes: probes})
	connId := apid.New(apid.PrefixConnection)

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	return &connection{
		Connection: database.Connection{
			Id:               connId,
			Namespace:        "root",
			State:            database.ConnectionStateReady,
			HealthState:      initialHealth,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      s,
		cv:     cv,
		logger: logger,
	}, db, &buf
}

func TestRecordPeriodicProbeOutcome_SingleFailureDoesNotFlip(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	conn, db, buf := newProbeHealthTestConn(t, ctrl, nil, database.ConnectionHealthStateHealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	db.EXPECT().
		RecordProbeFailure(gomock.Any(), conn.Id, "ping").
		Return(&database.ConnectionProbeHealth{ConsecutiveFailures: 1}, nil)
	// SetConnectionHealthState is NOT expected — single failure is sub-threshold.

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, false))
	for _, rec := range decodeJSONLines(t, buf) {
		assert.NotEqual(t, connectionHealthStateChangedMessage, rec["msg"],
			"no transition should fire on first failure")
	}
}

func TestRecordPeriodicProbeOutcome_NthFailureFlipsUnhealthy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	conn, db, buf := newProbeHealthTestConn(t, ctrl, nil, database.ConnectionHealthStateHealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	// Counter increment returns the post-update count = exactly the threshold.
	db.EXPECT().
		RecordProbeFailure(gomock.Any(), conn.Id, "ping").
		Return(&database.ConnectionProbeHealth{ConsecutiveFailures: 3}, nil)
	db.EXPECT().
		SetConnectionHealthState(gomock.Any(), conn.Id, database.ConnectionHealthStateUnhealthy).
		Return(nil)

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, false))
	assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState)

	recs := decodeJSONLines(t, buf)
	var found bool
	for _, rec := range recs {
		if rec["msg"] == connectionHealthStateChangedMessage {
			assert.Equal(t, "healthy", rec["previous_health_state"])
			assert.Equal(t, "unhealthy", rec["health_state"])
			assert.Equal(t, healthReasonPrefix+"ping", rec["reason"])
			found = true
		}
	}
	require.True(t, found, "expected a health_state changed event")
}

func TestRecordPeriodicProbeOutcome_AlreadyUnhealthyFailureDoesNotEmitDuplicate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	conn, db, buf := newProbeHealthTestConn(t, ctrl, nil, database.ConnectionHealthStateUnhealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	db.EXPECT().
		RecordProbeFailure(gomock.Any(), conn.Id, "ping").
		Return(&database.ConnectionProbeHealth{ConsecutiveFailures: 5}, nil)
	// MarkHealthState IS called (to unhealthy) but it's idempotent — no DB
	// write and no event. Omitting EXPECT for SetConnectionHealthState
	// verifies the DB layer is not touched.

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, false))
	for _, rec := range decodeJSONLines(t, buf) {
		assert.NotEqual(t, connectionHealthStateChangedMessage, rec["msg"],
			"flip-to-current-state must be a no-op")
	}
}

func TestRecordPeriodicProbeOutcome_SuccessWhileHealthyIsNoop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	conn, db, _ := newProbeHealthTestConn(t, ctrl, nil, database.ConnectionHealthStateHealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	db.EXPECT().
		RecordProbeSuccess(gomock.Any(), conn.Id, "ping").
		Return(&database.ConnectionProbeHealth{ConsecutiveSuccesses: 1}, nil)
	// NO ListConnectionProbeHealth (recovery path skipped — already healthy).
	// NO SetConnectionHealthState (still healthy).
	// NO ResetConnectionProbeHealth.

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, true))
}

func TestRecordPeriodicProbeOutcome_RecoveryThresholdFlipsHealthy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	// One probe configured, recovery_threshold = 2.
	probes := []cschema.Probe{
		{Id: "ping", FailureThreshold: intPtr(3), RecoveryThreshold: intPtr(2)},
	}
	conn, db, buf := newProbeHealthTestConn(t, ctrl, probes, database.ConnectionHealthStateUnhealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 2}
	db.EXPECT().
		RecordProbeSuccess(gomock.Any(), conn.Id, "ping").
		Return(&database.ConnectionProbeHealth{ConsecutiveSuccesses: 2}, nil)
	db.EXPECT().
		ListConnectionProbeHealth(gomock.Any(), conn.Id).
		Return(map[string]*database.ConnectionProbeHealth{
			"ping": {ConsecutiveSuccesses: 2, ConsecutiveFailures: 0},
		}, nil)
	db.EXPECT().
		SetConnectionHealthState(gomock.Any(), conn.Id, database.ConnectionHealthStateHealthy).
		Return(nil)
	db.EXPECT().
		ResetConnectionProbeHealth(gomock.Any(), conn.Id).
		Return(nil)

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, true))
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState)

	recs := decodeJSONLines(t, buf)
	var found bool
	for _, rec := range recs {
		if rec["msg"] == connectionHealthStateChangedMessage {
			assert.Equal(t, "unhealthy", rec["previous_health_state"])
			assert.Equal(t, "healthy", rec["health_state"])
			assert.Equal(t, healthReasonPrefix+"ping", rec["reason"])
			found = true
		}
	}
	require.True(t, found)
}

func TestRecordPeriodicProbeOutcome_SuccessBelowRecoveryThresholdDoesNotFlip(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	probes := []cschema.Probe{
		{Id: "ping", FailureThreshold: intPtr(3), RecoveryThreshold: intPtr(2)},
	}
	conn, db, _ := newProbeHealthTestConn(t, ctrl, probes, database.ConnectionHealthStateUnhealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 2}
	db.EXPECT().
		RecordProbeSuccess(gomock.Any(), conn.Id, "ping").
		Return(&database.ConnectionProbeHealth{ConsecutiveSuccesses: 1}, nil)
	// NOT enough successes yet — no list/reset/flip expected.

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, true))
	assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState)
}

func TestRecordPeriodicProbeOutcome_AnotherProbeOverThresholdBlocksRecovery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	// Two probes — ping recovers but pong is still failing.
	probes := []cschema.Probe{
		{Id: "ping", FailureThreshold: intPtr(3), RecoveryThreshold: intPtr(1)},
		{Id: "pong", FailureThreshold: intPtr(3), RecoveryThreshold: intPtr(1)},
	}
	conn, db, buf := newProbeHealthTestConn(t, ctrl, probes, database.ConnectionHealthStateUnhealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	db.EXPECT().
		RecordProbeSuccess(gomock.Any(), conn.Id, "ping").
		Return(&database.ConnectionProbeHealth{ConsecutiveSuccesses: 1}, nil)
	db.EXPECT().
		ListConnectionProbeHealth(gomock.Any(), conn.Id).
		Return(map[string]*database.ConnectionProbeHealth{
			"ping": {ConsecutiveSuccesses: 1, ConsecutiveFailures: 0},
			"pong": {ConsecutiveFailures: 4, ConsecutiveSuccesses: 0}, // still over threshold
		}, nil)
	// NO flip-to-healthy, NO reset.

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, true))
	assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState)
	for _, rec := range decodeJSONLines(t, buf) {
		assert.NotEqual(t, connectionHealthStateChangedMessage, rec["msg"],
			"recovery blocked while another probe is still failing")
	}
}

func TestRecordPeriodicProbeOutcome_ApiKeyConnectionStampsLastValidatedAt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	s, db, _, _, _, _ := FullMockService(t, ctrl)
	cv := NewTestConnectorVersion(cschema.Connector{
		Auth: &cschema.Auth{InnerVal: &cschema.AuthApiKey{
			Type:      cschema.AuthTypeAPIKey,
			Placement: &cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer},
		}},
	})
	connId := apid.New(apid.PrefixConnection)

	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	conn := &connection{
		Connection: database.Connection{
			Id:               connId,
			Namespace:        "root",
			State:            database.ConnectionStateReady,
			HealthState:      database.ConnectionHealthStateHealthy,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      s,
		cv:     cv,
		logger: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
	}

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	credId := apid.New(apid.PrefixApiKeyCredential)
	db.EXPECT().
		RecordProbeSuccess(gomock.Any(), connId, "ping").
		Return(&database.ConnectionProbeHealth{ConsecutiveSuccesses: 1}, nil)
	db.EXPECT().
		GetActiveApiKeyCredential(gomock.Any(), connId).
		Return(&database.ApiKeyCredential{
			Id:                   credId,
			ConnectionId:         connId,
			EncryptedCredentials: encfield.EncryptedField{ID: "ekv_fake", Data: "x"},
		}, nil)
	db.EXPECT().
		UpdateApiKeyCredentialLastValidated(gomock.Any(), credId, now).
		Return(nil)

	require.NoError(t, conn.recordPeriodicProbeOutcome(ctx, probe, true))
}

func TestRecordPeriodicProbeOutcome_OAuth2ConnectionSkipsLastValidatedAt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	s, db, _, _, _, _ := FullMockService(t, ctrl)
	cv := NewTestConnectorVersion(cschema.Connector{
		Auth: &cschema.Auth{InnerVal: &cschema.AuthOAuth2{Type: cschema.AuthTypeOAuth2}},
	})
	connId := apid.New(apid.PrefixConnection)

	conn := &connection{
		Connection: database.Connection{
			Id:               connId,
			Namespace:        "root",
			State:            database.ConnectionStateReady,
			HealthState:      database.ConnectionHealthStateHealthy,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      s,
		cv:     cv,
		logger: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
	}

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	db.EXPECT().
		RecordProbeSuccess(gomock.Any(), connId, "ping").
		Return(&database.ConnectionProbeHealth{ConsecutiveSuccesses: 1}, nil)
	// NO GetActiveApiKeyCredential expected.
	// NO UpdateApiKeyCredentialLastValidated expected.

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, true))
}

func intPtr(i int) *int { return &i }

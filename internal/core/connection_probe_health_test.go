package core

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

// stubProbe is a minimal iface.Probe used only as a threshold + id carrier
// for the runtime tests. Invoke is never called from the helper under test.
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

// outcomes builds a most-recent-first slice of probe outcomes for use as the
// GetRecentProbeOutcomes mock return. Each rune is one event: 's' = success,
// 'f' = failure. "ffs" = newest-to-oldest: failure, failure, success.
func outcomes(s string) []*database.ConnectionProbeOutcome {
	out := make([]*database.ConnectionProbeOutcome, 0, len(s))
	for _, r := range s {
		var oc string
		switch r {
		case 's':
			oc = database.ProbeOutcomeStatusSuccess
		case 'f':
			oc = database.ProbeOutcomeStatusFailure
		default:
			panic("outcomes: invalid rune; use 's' or 'f'")
		}
		out = append(out, &database.ConnectionProbeOutcome{Outcome: oc})
	}
	return out
}

// newProbeHealthTestConn builds a test connection whose connector definition
// carries the supplied probes — the runtime reads those for recovery-time
// threshold lookups against other probes.
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
			State:            database.ConnectionStateConfigured,
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
		InsertProbeOutcome(gomock.Any(), conn.Id, "ping", database.ProbeOutcomeStatusFailure, "boom").
		Return(&database.ConnectionProbeOutcome{}, nil)
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), conn.Id, "ping", 3).
		Return(outcomes("f"), nil)
	// NO SetConnectionHealthState — streak is sub-threshold.

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, false, errors.New("boom")))
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
	db.EXPECT().
		InsertProbeOutcome(gomock.Any(), conn.Id, "ping", database.ProbeOutcomeStatusFailure, "").
		Return(&database.ConnectionProbeOutcome{}, nil)
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), conn.Id, "ping", 3).
		Return(outcomes("fff"), nil)
	db.EXPECT().
		SetConnectionHealthState(gomock.Any(), conn.Id, database.ConnectionHealthStateUnhealthy).
		Return(nil)

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, false, nil))
	assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState)

	var found bool
	for _, rec := range decodeJSONLines(t, buf) {
		if rec["msg"] == connectionHealthStateChangedMessage {
			assert.Equal(t, "healthy", rec["previous_health_state"])
			assert.Equal(t, "unhealthy", rec["health_state"])
			assert.Equal(t, healthReasonPrefix+"ping", rec["reason"])
			found = true
		}
	}
	require.True(t, found, "expected a health_state changed event")
}

func TestRecordPeriodicProbeOutcome_FailureStreakBrokenByInterveningSuccess(t *testing.T) {
	// The just-inserted failure is preceded by a success in the recent log —
	// the consecutive streak is 1, not 2.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	conn, db, _ := newProbeHealthTestConn(t, ctrl, nil, database.ConnectionHealthStateHealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	db.EXPECT().
		InsertProbeOutcome(gomock.Any(), conn.Id, "ping", database.ProbeOutcomeStatusFailure, "").
		Return(&database.ConnectionProbeOutcome{}, nil)
	// Newest first: f, s, f. Counting consecutive failures from the head
	// stops at the success → streak = 1.
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), conn.Id, "ping", 3).
		Return(outcomes("fsf"), nil)
	// NO flip.

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, false, nil))
}

func TestRecordPeriodicProbeOutcome_AlreadyUnhealthyFailureNoDuplicate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	conn, db, buf := newProbeHealthTestConn(t, ctrl, nil, database.ConnectionHealthStateUnhealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	db.EXPECT().
		InsertProbeOutcome(gomock.Any(), conn.Id, "ping", database.ProbeOutcomeStatusFailure, "").
		Return(&database.ConnectionProbeOutcome{}, nil)
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), conn.Id, "ping", 3).
		Return(outcomes("fff"), nil)
	// MarkHealthState IS called but is idempotent — no DB write, no event.

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, false, nil))
	for _, rec := range decodeJSONLines(t, buf) {
		assert.NotEqual(t, connectionHealthStateChangedMessage, rec["msg"])
	}
}

func TestRecordPeriodicProbeOutcome_SuccessWhileHealthyIsNoop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	conn, db, _ := newProbeHealthTestConn(t, ctrl, nil, database.ConnectionHealthStateHealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	db.EXPECT().
		InsertProbeOutcome(gomock.Any(), conn.Id, "ping", database.ProbeOutcomeStatusSuccess, "").
		Return(&database.ConnectionProbeOutcome{}, nil)
	// Recovery path skipped — already healthy.

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, true, nil))
}

func TestRecordPeriodicProbeOutcome_RecoveryThresholdFlipsHealthy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	probes := []cschema.Probe{
		{Id: "ping", FailureThreshold: intPtr(3), RecoveryThreshold: intPtr(2)},
	}
	conn, db, buf := newProbeHealthTestConn(t, ctrl, probes, database.ConnectionHealthStateUnhealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 2}
	db.EXPECT().
		InsertProbeOutcome(gomock.Any(), conn.Id, "ping", database.ProbeOutcomeStatusSuccess, "").
		Return(&database.ConnectionProbeOutcome{}, nil)
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), conn.Id, "ping", 2).
		Return(outcomes("ss"), nil)
	// Sole probe — cross-probe loop has nothing else to inspect.
	db.EXPECT().
		SetConnectionHealthState(gomock.Any(), conn.Id, database.ConnectionHealthStateHealthy).
		Return(nil)

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, true, nil))
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState)

	var found bool
	for _, rec := range decodeJSONLines(t, buf) {
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
		InsertProbeOutcome(gomock.Any(), conn.Id, "ping", database.ProbeOutcomeStatusSuccess, "").
		Return(&database.ConnectionProbeOutcome{}, nil)
	// 1 success then 1 failure — streak = 1, below threshold 2.
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), conn.Id, "ping", 2).
		Return(outcomes("sf"), nil)

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, true, nil))
	assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState)
}

func TestRecordPeriodicProbeOutcome_AnotherProbeOverThresholdBlocksRecovery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	probes := []cschema.Probe{
		{Id: "ping", FailureThreshold: intPtr(3), RecoveryThreshold: intPtr(1)},
		{Id: "pong", FailureThreshold: intPtr(3), RecoveryThreshold: intPtr(1)},
	}
	conn, db, buf := newProbeHealthTestConn(t, ctrl, probes, database.ConnectionHealthStateUnhealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	db.EXPECT().
		InsertProbeOutcome(gomock.Any(), conn.Id, "ping", database.ProbeOutcomeStatusSuccess, "").
		Return(&database.ConnectionProbeOutcome{}, nil)
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), conn.Id, "ping", 1).
		Return(outcomes("s"), nil)
	// Cross-probe check: pong's recent outcomes show 3 consecutive failures.
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), conn.Id, "pong", 3).
		Return(outcomes("fff"), nil)

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, true, nil))
	assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState,
		"recovery blocked while another probe is still failing")
	for _, rec := range decodeJSONLines(t, buf) {
		assert.NotEqual(t, connectionHealthStateChangedMessage, rec["msg"])
	}
}

func TestRecordPeriodicProbeOutcome_DisabledPeerDoesNotBlockRecovery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	probes := []cschema.Probe{
		{Id: "ping", FailureThreshold: intPtr(3), RecoveryThreshold: intPtr(1)},
		{
			Id:                "pong",
			If:                &common.Predicate{Javascript: `cfg.enable_pong === true`},
			FailureThreshold:  intPtr(1),
			RecoveryThreshold: intPtr(1),
		},
	}
	conn, db, _ := newProbeHealthTestConn(t, ctrl, probes, database.ConnectionHealthStateUnhealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	db.EXPECT().
		InsertProbeOutcome(gomock.Any(), conn.Id, "ping", database.ProbeOutcomeStatusSuccess, "").
		Return(&database.ConnectionProbeOutcome{}, nil)
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), conn.Id, "ping", 1).
		Return(outcomes("s"), nil)
	db.EXPECT().
		SetConnectionHealthState(gomock.Any(), conn.Id, database.ConnectionHealthStateHealthy).
		Return(nil)
	// No pong outcome lookup — disabled peer probes do not block recovery.

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, true, nil))
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState)
}

func TestRecordPeriodicProbeOutcome_AnotherProbeRecoveredAllowsRecovery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	probes := []cschema.Probe{
		{Id: "ping", FailureThreshold: intPtr(3), RecoveryThreshold: intPtr(1)},
		{Id: "pong", FailureThreshold: intPtr(3), RecoveryThreshold: intPtr(1)},
	}
	conn, db, _ := newProbeHealthTestConn(t, ctrl, probes, database.ConnectionHealthStateUnhealthy)

	probe := &stubProbe{id: "ping", failureThresh: 3, recoveryThresh: 1}
	db.EXPECT().
		InsertProbeOutcome(gomock.Any(), conn.Id, "ping", database.ProbeOutcomeStatusSuccess, "").
		Return(&database.ConnectionProbeOutcome{}, nil)
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), conn.Id, "ping", 1).
		Return(outcomes("s"), nil)
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), conn.Id, "pong", 3).
		Return(outcomes("sff"), nil) // success at head breaks pong's failure streak
	db.EXPECT().
		SetConnectionHealthState(gomock.Any(), conn.Id, database.ConnectionHealthStateHealthy).
		Return(nil)

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, true, nil))
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState)
}

func TestRecordPeriodicProbeOutcome_ApiKeyStampsLastValidatedAt(t *testing.T) {
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
			State:            database.ConnectionStateConfigured,
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
		InsertProbeOutcome(gomock.Any(), connId, "ping", database.ProbeOutcomeStatusSuccess, "").
		Return(&database.ConnectionProbeOutcome{}, nil)
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

	require.NoError(t, conn.recordPeriodicProbeOutcome(ctx, probe, true, nil))
}

func TestRecordPeriodicProbeOutcome_OAuth2SkipsLastValidatedAt(t *testing.T) {
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
			State:            database.ConnectionStateConfigured,
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
		InsertProbeOutcome(gomock.Any(), connId, "ping", database.ProbeOutcomeStatusSuccess, "").
		Return(&database.ConnectionProbeOutcome{}, nil)
	// NO GetActiveApiKeyCredential / UpdateApiKeyCredentialLastValidated.

	require.NoError(t, conn.recordPeriodicProbeOutcome(context.Background(), probe, true, nil))
}

func intPtr(i int) *int { return &i }

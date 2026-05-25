package core

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	mockH "github.com/rmorlok/authproxy/internal/httpf/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	genmock "gopkg.in/h2non/gentleman-mock.v2"
)

// TestRunProbe_Success exercises the periodic-probe path inline: probe.Invoke
// returns ok, recordPeriodicProbeOutcome appends a success row, and (since
// the connection is already healthy) no health-state transition fires.
func TestRunProbe_Success(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connectorId := apid.New(apid.PrefixConnectorVersion)
	connector := &cschema.Connector{
		Id:          connectorId,
		Version:     1,
		DisplayName: "probe-success-test",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		Probes: []cschema.Probe{{
			Id: "ping",
			Http: &cschema.ProbeHttp{
				Method: "GET",
				URL:    "https://upstream.example.invalid/health",
			},
		}},
	}
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateConfigured,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
	}

	svc, db, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()

	svc.httpf = mockH.NewFactoryWithMockingClient(ctrl)
	genmock.New("https://upstream.example.invalid").Get("/health").Reply(200)

	// Periodic probe path: success → append outcome row. Connection
	// already healthy → no transition → no SetConnectionHealthState call.
	db.EXPECT().
		InsertProbeOutcome(gomock.Any(), connectionId, "ping", database.ProbeOutcomeStatusSuccess, "").
		Return(&database.ConnectionProbeOutcome{}, nil)
	// maybeUpdateApiKeyLastValidated on success against an api-key
	// connection: look up the active credential, stamp last_validated_at.
	// No active credential in this test setup; GetActiveApiKeyCredential
	// returns ErrNotFound and the call is a no-op.
	db.EXPECT().
		GetActiveApiKeyCredential(gomock.Any(), connectionId).
		Return(nil, database.ErrNotFound)

	require.NoError(t, svc.RunProbe(context.Background(), connectionId, "ping"))
}

// TestRunProbe_FailureBelowThreshold: a single failure when the probe's
// failure threshold is 3 records the outcome but does NOT flip health.
// The runtime returns the probe's invoke error so callers can log it.
func TestRunProbe_FailureBelowThreshold(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	threshold := 3
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "probe-failure-test",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		Probes: []cschema.Probe{{
			Id: "ping",
			Http: &cschema.ProbeHttp{
				Method: "GET",
				URL:    "https://upstream.example.invalid/health",
			},
			FailureThreshold: &threshold,
		}},
	}
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateConfigured,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
	}

	svc, db, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()

	svc.httpf = mockH.NewFactoryWithMockingClient(ctrl)
	genmock.New("https://upstream.example.invalid").Get("/health").Reply(401)

	db.EXPECT().
		InsertProbeOutcome(gomock.Any(), connectionId, "ping", database.ProbeOutcomeStatusFailure, gomock.Any()).
		Return(&database.ConnectionProbeOutcome{}, nil)
	// Streak lookup returns one failure (just-inserted), threshold is 3
	// — sub-threshold, no transition.
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), connectionId, "ping", threshold).
		Return([]*database.ConnectionProbeOutcome{
			{Outcome: database.ProbeOutcomeStatusFailure},
		}, nil)
	// NO SetConnectionHealthState call — streak below threshold.

	err := svc.RunProbe(context.Background(), connectionId, "ping")
	require.Error(t, err, "RunProbe must surface the probe's invoke error")
	assert.Contains(t, err.Error(), "status",
		"recorded error should mention the upstream status code")
}

// TestRunProbe_FailureCrossesThreshold: with threshold=1 and one failure,
// the runtime flips the connection to unhealthy via MarkHealthState (which
// calls SetConnectionHealthState).
func TestRunProbe_FailureCrossesThreshold(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	threshold := 1
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "probe-threshold-test",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		Probes: []cschema.Probe{{
			Id: "ping",
			Http: &cschema.ProbeHttp{
				Method: "GET",
				URL:    "https://upstream.example.invalid/health",
			},
			FailureThreshold: &threshold,
		}},
	}
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateConfigured,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
	}

	svc, db, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()

	svc.httpf = mockH.NewFactoryWithMockingClient(ctrl)
	genmock.New("https://upstream.example.invalid").Get("/health").Reply(401)

	db.EXPECT().
		InsertProbeOutcome(gomock.Any(), connectionId, "ping", database.ProbeOutcomeStatusFailure, gomock.Any()).
		Return(&database.ConnectionProbeOutcome{}, nil)
	db.EXPECT().
		GetRecentProbeOutcomes(gomock.Any(), connectionId, "ping", threshold).
		Return([]*database.ConnectionProbeOutcome{
			{Outcome: database.ProbeOutcomeStatusFailure},
		}, nil)
	db.EXPECT().
		SetConnectionHealthState(gomock.Any(), connectionId, database.ConnectionHealthStateUnhealthy).
		Return(nil)

	err := svc.RunProbe(context.Background(), connectionId, "ping")
	require.Error(t, err)
}

// TestRunProbe_ProbeNotFound covers the lookup-failure branch — probe id
// missing from the connector definition is treated as a non-retryable
// condition (the probe will never appear on retry).
func TestRunProbe_ProbeNotFound(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "missing-probe-test",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		// No probes — the lookup for "ping" must fail.
	}
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateConfigured,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
	}

	svc, _, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()

	err := svc.RunProbe(context.Background(), connectionId, "ping")
	require.Error(t, err)
	assert.ErrorIs(t, err, asynq.SkipRetry,
		"missing probe id should be SkipRetry — retrying won't bring it back")
}

// TestRunProbe_ConnectionNotFound returns SkipRetry so the asynq retry loop
// doesn't pile up against a non-existent row.
func TestRunProbe_ConnectionNotFound(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := mockDb.NewMockDB(ctrl)
	encrypt := mockEncrypt.NewMockE(ctrl)
	logger, _ := mockLog.NewTestLogger(t)

	svc := &service{db: db, encrypt: encrypt, logger: logger}

	db.EXPECT().
		GetConnection(gomock.Any(), connectionId).
		Return(nil, database.ErrNotFound)

	err := svc.RunProbe(context.Background(), connectionId, "ping")
	require.Error(t, err)
	assert.ErrorIs(t, err, asynq.SkipRetry)
}

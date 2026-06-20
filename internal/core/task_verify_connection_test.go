package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	mockH "github.com/rmorlok/authproxy/internal/httpf/mock"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	genmock "gopkg.in/h2non/gentleman-mock.v2"
)

// setupVerifyTest wires a minimal service whose db.GetConnection returns the
// supplied connection and whose db.GetConnectorVersion + encrypt.DecryptString
// resolve to the supplied connector. Lets each subtest customize the
// connection's setup_step / health_state without sharing fixture state.
func setupVerifyTest(
	t *testing.T,
	connectionId apid.ID,
	conn *database.Connection,
	connector *cschema.Connector,
) (*service, *mockDb.MockDB, *mockEncrypt.MockE, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	db := mockDb.NewMockDB(ctrl)
	encrypt := mockEncrypt.NewMockE(ctrl)
	logger, _ := mockLog.NewTestLogger(t)

	svc := &service{
		cfg:     nil,
		db:      db,
		encrypt: encrypt,
		logger:  logger,
	}

	db.EXPECT().
		GetConnection(gomock.Any(), connectionId).
		Return(conn, nil).
		AnyTimes()

	encryptedDef := encfield.EncryptedField{ID: "dek_mock", Data: "encrypted-def"}
	db.EXPECT().
		GetConnectorVersion(gomock.Any(), connector.Id, connector.Version).
		Return(&database.ConnectorVersion{
			Id:                  connector.Id,
			Version:             connector.Version,
			State:               database.ConnectorVersionStatePrimary,
			Hash:                "hash",
			EncryptedDefinition: encryptedDef,
		}, nil).
		AnyTimes()

	connJSON, err := json.Marshal(connector)
	require.NoError(t, err)
	encrypt.EXPECT().
		DecryptString(gomock.Any(), encryptedDef).
		Return(string(connJSON), nil).
		AnyTimes()

	return svc, db, encrypt, ctrl
}

// TestRunVerifyConnection_NoProbes_AdvancesToReady covers the happy path:
// connector with no probes → verify is a structural no-op (loop runs zero
// times) → onVerifyPassed clears setup_step and advances state to Ready.
func TestRunVerifyConnection_NoProbes_AdvancesToReady(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "verify-test",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		// No probes → runVerifyConnection's loop does nothing.
	}
	verifyStep := cschema.SetupStepVerify
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateSetup,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
		SetupStep:        &verifyStep,
	}

	svc, db, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()

	// onVerifyPassed: no configure step → next is zero → clear setup_step
	// + advance to Ready.
	db.EXPECT().
		SetConnectionSetupStep(gomock.Any(), connectionId, (*cschema.SetupStep)(nil)).
		Return(nil)
	db.EXPECT().
		SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateConfigured).
		Return(nil)

	require.NoError(t, svc.RunVerifyConnection(context.Background(), connectionId))
}

func TestRunVerifyConnection_AllProbesDisabled_AdvancesToReady(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "verify-disabled-test",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		Probes: []cschema.Probe{{
			Id: "disabled",
			If: &common.Predicate{Javascript: `cfg.run_probe === true`},
			Http: &cschema.ProbeHttp{
				Method: "GET",
				URL:    "https://upstream.example.invalid/disabled",
			},
		}},
	}
	verifyStep := cschema.SetupStepVerify
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateSetup,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
		SetupStep:        &verifyStep,
	}

	svc, db, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()

	db.EXPECT().
		SetConnectionSetupStep(gomock.Any(), connectionId, (*cschema.SetupStep)(nil)).
		Return(nil)
	db.EXPECT().
		SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateConfigured).
		Return(nil)

	require.NoError(t, svc.RunVerifyConnection(context.Background(), connectionId))
}

func TestRunVerifyConnection_MixedProbePredicates_RunOnlyEnabled(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "verify-mixed-test",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		Probes: []cschema.Probe{
			{
				Id: "enabled",
				Http: &cschema.ProbeHttp{
					Method: "GET",
					URL:    "https://upstream.example.invalid/enabled",
				},
			},
			{
				Id: "disabled",
				If: &common.Predicate{Javascript: `cfg.run_disabled === true`},
				Http: &cschema.ProbeHttp{
					Method: "GET",
					URL:    "https://upstream.example.invalid/disabled",
				},
			},
		},
	}
	verifyStep := cschema.SetupStepVerify
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateSetup,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
		SetupStep:        &verifyStep,
	}

	svc, db, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()

	svc.httpf = mockH.NewFactoryWithMockingClient(ctrl)
	genmock.New("https://upstream.example.invalid").Get("/enabled").Reply(200)

	db.EXPECT().
		SetConnectionSetupStep(gomock.Any(), connectionId, (*cschema.SetupStep)(nil)).
		Return(nil)
	db.EXPECT().
		SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateConfigured).
		Return(nil)

	require.NoError(t, svc.RunVerifyConnection(context.Background(), connectionId))
}

// TestRunVerifyConnection_StalePhase_SkipsCleanly covers the guard that
// short-circuits the runtime when the connection has moved past the verify
// phase (e.g. a stale asynq task fires after a manual reset). The handler
// must return nil and must not write to the DB.
func TestRunVerifyConnection_StalePhase_SkipsCleanly(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "stale-test",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
	}
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateConfigured,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
		SetupStep:        nil, // already past verify
	}

	svc, _, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()

	// No SetConnectionSetupStep / SetConnectionState expectations — the
	// guard returns before either is reached. gomock controller fails at
	// Finish if either is called.
	require.NoError(t, svc.RunVerifyConnection(context.Background(), connectionId))
}

// TestRunVerifyConnection_ConnectionNotFound returns SkipRetry so the asynq
// retry loop doesn't pile up against a non-existent row.
func TestRunVerifyConnection_ConnectionNotFound(t *testing.T) {
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

	err := svc.RunVerifyConnection(context.Background(), connectionId)
	require.Error(t, err)
	assert.ErrorIs(t, err, asynq.SkipRetry)
}

// TestRunVerifyConnection_ProbeFailure routes through onVerifyFailed: the
// probe returns a non-2xx → recorded failure → setup_error populated →
// setup_step advanced to the verify_failed terminal pseudo-step → asynq
// SkipRetry surfaces so the retry loop doesn't fight recorded state.
func TestRunVerifyConnection_ProbeFailure(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "fail-test",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		Probes: []cschema.Probe{{
			Id: "always-fails",
			Http: &cschema.ProbeHttp{
				Method: "GET",
				URL:    "https://upstream.example.invalid/health",
			},
		}},
	}
	verifyStep := cschema.SetupStepVerify
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateSetup,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
		SetupStep:        &verifyStep,
	}

	svc, db, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()

	// gentleman-mock returns 500 for any unmatched request, which is then
	// classified by probe_http as a failure (non-2xx). Wire the httpf
	// factory so the probe actually dispatches through gentleman + gock.
	svc.httpf = mockH.NewFactoryWithMockingClient(ctrl)
	genmock.New("https://upstream.example.invalid").Get("/health").Reply(500)

	// onVerifyFailed → setup_error stored, setup_step=verify_failed.
	failedStep := cschema.SetupStepVerifyFailed
	db.EXPECT().
		SetConnectionSetupError(gomock.Any(), connectionId, gomock.AssignableToTypeOf((*string)(nil))).
		Return(nil)
	db.EXPECT().
		SetConnectionSetupStep(gomock.Any(), connectionId, &failedStep).
		Return(nil)

	err := svc.RunVerifyConnection(context.Background(), connectionId)
	require.Error(t, err, "verify must surface an error when a probe fails")
	assert.ErrorIs(t, err, asynq.SkipRetry,
		"verify failure must be SkipRetry — recorded state, no point retrying")
}

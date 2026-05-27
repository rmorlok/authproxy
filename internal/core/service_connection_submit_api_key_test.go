package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	mockAsynq "github.com/rmorlok/authproxy/internal/apasynq/mock"
	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/auth_methods/api_key"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

// newTestApiKeyConnection builds a test connection backed by an AuthApiKey
// connector with the given placement. The connector is Normalize()d first so
// the synthesized credentials step is present, mirroring runtime behavior.
func newTestApiKeyConnection(
	t *testing.T,
	ctrl *gomock.Controller,
	placement *cschema.ApiKeyPlacement,
	probes []cschema.Probe,
) (*connection, *mockDb.MockDB, *mockAsynq.MockClient) {
	t.Helper()

	e := encrypt.NewFakeEncryptService(false)
	s, db, _, _, ac, _ := FullMockService(t, ctrl)
	// Replace the registry's default api-key factory with one bound to the
	// fake encrypt service so the captured blob in tests round-trips as
	// plaintext JSON (rather than the production-style ciphertext that the
	// mock encrypt service in FullMockService would produce).
	s.encrypt = e
	s.authMethodFactories[cschema.AuthTypeAPIKey] = api_key.NewFactory(db, e, s.httpf, s.logger)

	connector := cschema.Connector{
		Auth: &cschema.Auth{InnerVal: &cschema.AuthApiKey{
			Type:      cschema.AuthTypeAPIKey,
			Placement: placement,
		}},
		Probes: probes,
	}
	connector.Normalize()
	cv := NewTestConnectorVersion(connector)

	conn := &connection{
		Connection: database.Connection{
			Id:               "cxn_test1111111111aa",
			Namespace:        "root",
			State:            database.ConnectionStateSetup,
			HealthState:      database.ConnectionHealthStateHealthy,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      s,
		cv:     cv,
		logger: aplog.NewNoopLogger(),
	}
	return conn, db, ac
}

func contextWithActor(t *testing.T) (context.Context, apid.ID) {
	t.Helper()
	actorId := apid.MustParse("act_test1111111111aa")
	ra := apauthcore.NewAuthenticatedRequestAuth(&apauthcore.Actor{
		Id:        actorId,
		Namespace: "root",
	})
	return ra.ContextWith(context.Background()), actorId
}

// captureEncCredential captures the encrypted blob passed to InsertApiKeyCredential.
type captureEncCredential struct{ field encfield.EncryptedField }

func (c *captureEncCredential) Matches(x any) bool {
	v, ok := x.(encfield.EncryptedField)
	if !ok {
		return false
	}
	c.field = v
	return true
}
func (c *captureEncCredential) String() string { return "captured encfield.EncryptedField" }

func TestApiKeySubmit_BearerPersistsCredentialAndAdvancesToReady(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, db, _ := newTestApiKeyConnection(t, ctrl, &cschema.ApiKeyPlacement{
		Type: cschema.ApiKeyPlacementBearer,
	}, nil)

	step := cschema.MustNewSetupStep(cschema.SynthesizedApiKeyCredentialsStepId)
	conn.SetupStep = &step

	credCap := &captureEncCredential{}
	ctx, actorId := contextWithActor(t)

	// Critical: SetConnectionEncryptedConfiguration is NOT expected. The
	// credentials phase routes plaintext to the auth method, never into the
	// general per-connection config blob. Omitting an EXPECT here means the
	// mock controller will fail the test if the call ever happens — which is
	// exactly the invariant we're guarding.
	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, credCap, gomock.AssignableToTypeOf(&cschema.ApiKeyPlacement{}), &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)
	db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*cschema.SetupStep)(nil)).Return(nil)
	db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateConfigured).Return(nil)

	resp, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: cschema.SynthesizedApiKeyCredentialsStepId,
		Data:   json.RawMessage(`{"api_key":"sk-abc-123"}`),
	})
	require.NoError(t, err)
	require.IsType(t, &iface.ConnectionSetupComplete{}, resp)

	// Credential blob holds the JSON plaintext (fake encrypt stores plaintext in Data).
	var plaintext database.ApiKeyCredentialPlaintext
	require.NoError(t, json.Unmarshal([]byte(credCap.field.Data), &plaintext))
	require.Equal(t, "sk-abc-123", plaintext.ApiKey)
	require.Empty(t, plaintext.Username)
}

func TestApiKeySubmit_BasicPersistsBothCredentialFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, db, _ := newTestApiKeyConnection(t, ctrl, &cschema.ApiKeyPlacement{
		Type:          cschema.ApiKeyPlacementBasic,
		UsernameField: "account_id",
	}, nil)

	step := cschema.MustNewSetupStep(cschema.SynthesizedApiKeyCredentialsStepId)
	conn.SetupStep = &step

	credCap := &captureEncCredential{}
	ctx, actorId := contextWithActor(t)

	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, credCap, gomock.AssignableToTypeOf(&cschema.ApiKeyPlacement{}), &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)
	db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*cschema.SetupStep)(nil)).Return(nil)
	db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateConfigured).Return(nil)

	resp, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: cschema.SynthesizedApiKeyCredentialsStepId,
		Data:   json.RawMessage(`{"api_key":"key-xyz","account_id":"acct-001"}`),
	})
	require.NoError(t, err)
	require.IsType(t, &iface.ConnectionSetupComplete{}, resp)

	var plaintext database.ApiKeyCredentialPlaintext
	require.NoError(t, json.Unmarshal([]byte(credCap.field.Data), &plaintext))
	require.Equal(t, "key-xyz", plaintext.ApiKey)
	require.Equal(t, "acct-001", plaintext.Username)
}

func TestApiKeySubmit_TransitionsToVerifyWhenProbesPresent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, db, ac := newTestApiKeyConnection(t, ctrl,
		&cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer},
		[]cschema.Probe{
			{Id: "ping", Http: &cschema.ProbeHttp{Method: "GET", URL: "https://example.com/ping"}},
		},
	)
	step := cschema.MustNewSetupStep(cschema.SynthesizedApiKeyCredentialsStepId)
	conn.SetupStep = &step

	ctx, actorId := contextWithActor(t)
	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, gomock.Any(), gomock.Any(), &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)
	db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &cschema.SetupStepVerify).Return(nil)
	ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)

	resp, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: cschema.SynthesizedApiKeyCredentialsStepId,
		Data:   json.RawMessage(`{"api_key":"sk-abc"}`),
	})
	require.NoError(t, err)
	require.IsType(t, &iface.ConnectionSetupVerifying{}, resp)
}

func TestApiKeySubmit_NoReplay_PriorCredentialNeverDecryptedIntoForm(t *testing.T) {
	// The submit flow must NOT decrypt a prior credential and echo it back.
	// This is the foundation for the reauth no-replay guarantee.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, db, _ := newTestApiKeyConnection(t, ctrl, &cschema.ApiKeyPlacement{
		Type: cschema.ApiKeyPlacementBearer,
	}, nil)
	step := cschema.MustNewSetupStep(cschema.SynthesizedApiKeyCredentialsStepId)
	conn.SetupStep = &step

	ctx, actorId := contextWithActor(t)
	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, gomock.Any(), gomock.Any(), &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)
	db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*cschema.SetupStep)(nil)).Return(nil)
	db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateConfigured).Return(nil)

	// CRITICAL: GetActiveApiKeyCredential must NOT be called during submit.
	// gomock fails ctrl.Finish() if submit reads the prior credential.

	resp, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: cschema.SynthesizedApiKeyCredentialsStepId,
		Data:   json.RawMessage(`{"api_key":"new-key"}`),
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

// TestApiKeySubmit_PreconnectFieldNamedApiKeyIsNotTreatedAsCredential is the
// regression test for the naming-conflict bug. A connector author can declare
// a preconnect step that happens to include a field literally named "api_key"
// — that field belongs to the connection config, NOT to the credential. The
// new phase-dispatched submit handler routes by phase, so a preconnect submit
// never goes near the credential persistence path.
func TestApiKeySubmit_PreconnectFieldNamedApiKeyIsNotTreatedAsCredential(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Author declares a preconnect step with a field called "api_key" — a
	// pathological case to stress the dispatcher. The synthesized credentials
	// step still exists alongside (Normalize doesn't suppress credentials when
	// preconnect is present).
	preconnect := cschema.SetupFlowStep{
		Id: "tenant",
		JsonSchema: common.RawJSON(`{
			"type": "object",
			"required": ["api_key"],
			"properties": {"api_key": {"type": "string"}},
			"additionalProperties": false
		}`),
	}
	e := encrypt.NewFakeEncryptService(false)
	s, db, _, _, _, _ := FullMockService(t, ctrl)
	s.encrypt = e
	s.authMethodFactories[cschema.AuthTypeAPIKey] = api_key.NewFactory(db, e, s.httpf, s.logger)

	connector := cschema.Connector{
		Auth: &cschema.Auth{InnerVal: &cschema.AuthApiKey{
			Type:      cschema.AuthTypeAPIKey,
			Placement: &cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer},
		}},
		SetupFlow: &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{Steps: []cschema.SetupFlowStep{preconnect}},
		},
	}
	connector.Normalize()
	cv := NewTestConnectorVersion(connector)
	conn := &connection{
		Connection: database.Connection{
			Id:               "cxn_test2222222222aa",
			Namespace:        "root",
			State:            database.ConnectionStateSetup,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      s,
		cv:     cv,
		logger: aplog.NewNoopLogger(),
	}

	step := cschema.MustNewSetupStep("tenant")
	conn.SetupStep = &step

	// Critical invariants:
	//   - InsertApiKeyCredential must NOT be called for a preconnect submit
	//     (omitting EXPECT enforces this).
	//   - The author's "api_key" preconnect field IS expected to land in the
	//     connection config — SetConnectionEncryptedConfiguration must be called
	//     and the value must be present.
	db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ apid.ID, ef *encfield.EncryptedField) error {
			var cfg map[string]any
			require.NoError(t, json.Unmarshal([]byte(ef.Data), &cfg))
			require.Equal(t, "tenant-supplied-value", cfg["api_key"],
				"preconnect's own api_key field must reach EncryptedConfiguration unchanged")
			return nil
		})
	// After preconnect completes the next step is the synthesized credentials step,
	// so SubmitForm returns that form (not complete).
	credsStep := cschema.MustNewSetupStep(cschema.SynthesizedApiKeyCredentialsStepId)
	db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &credsStep).Return(nil)

	ctx, _ := contextWithActor(t)
	resp, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: "tenant",
		Data:   json.RawMessage(`{"api_key":"tenant-supplied-value"}`),
	})
	require.NoError(t, err)
	form, ok := resp.(*iface.ConnectionSetupForm)
	require.True(t, ok, "expected next step to be the credentials form, got %T", resp)
	require.Equal(t, cschema.SynthesizedApiKeyCredentialsStepId, form.StepId)
}

// TestApiKeySubmit_TransitionsToConfigureWhenNoProbes covers the third
// post-credentials transition (besides ready and verify): when the connector
// has a configure phase but no probes, credentials submit advances to configure:0.
func TestApiKeySubmit_TransitionsToConfigureWhenNoProbes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	e := encrypt.NewFakeEncryptService(false)
	s, db, _, _, _, _ := FullMockService(t, ctrl)
	s.encrypt = e
	s.authMethodFactories[cschema.AuthTypeAPIKey] = api_key.NewFactory(db, e, s.httpf, s.logger)

	configureStep := cschema.SetupFlowStep{
		Id:         "workspace",
		JsonSchema: common.RawJSON(`{"type":"object","properties":{"workspace_id":{"type":"string"}}}`),
	}
	connector := cschema.Connector{
		Auth: &cschema.Auth{InnerVal: &cschema.AuthApiKey{
			Type:      cschema.AuthTypeAPIKey,
			Placement: &cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer},
		}},
		SetupFlow: &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{Steps: []cschema.SetupFlowStep{configureStep}},
		},
	}
	connector.Normalize()
	cv := NewTestConnectorVersion(connector)
	conn := &connection{
		Connection: database.Connection{
			Id:               "cxn_test3333333333aa",
			Namespace:        "root",
			State:            database.ConnectionStateSetup,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      s,
		cv:     cv,
		logger: aplog.NewNoopLogger(),
	}

	step := cschema.MustNewSetupStep(cschema.SynthesizedApiKeyCredentialsStepId)
	conn.SetupStep = &step

	ctx, actorId := contextWithActor(t)
	configureFirst := cschema.MustNewSetupStep("workspace")
	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, gomock.Any(), gomock.Any(), &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)
	db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &configureFirst).Return(nil)
	// Resuming via GetCurrentSetupStepResponse: rebuilds the configure form,
	// which requires SetConnectionSetupStep to be called once more with the
	// same step (buildFormResponse re-asserts the setup step).
	db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &configureFirst).Return(nil)

	resp, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: cschema.SynthesizedApiKeyCredentialsStepId,
		Data:   json.RawMessage(`{"api_key":"sk-abc"}`),
	})
	require.NoError(t, err)
	form, ok := resp.(*iface.ConnectionSetupForm)
	require.True(t, ok, "expected configure form, got %T", resp)
	require.Equal(t, "workspace", form.StepId)
}

// TestApiKeySubmit_RejectsMismatchedStepId asserts schema-step-id validation
// before any credential persistence kicks in.
func TestApiKeySubmit_RejectsMismatchedStepId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, _, _ := newTestApiKeyConnection(t, ctrl, &cschema.ApiKeyPlacement{
		Type: cschema.ApiKeyPlacementBearer,
	}, nil)
	step := cschema.MustNewSetupStep(cschema.SynthesizedApiKeyCredentialsStepId)
	conn.SetupStep = &step

	// No InsertApiKeyCredential expected — submit should error before that.

	ctx, _ := contextWithActor(t)
	_, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: "some-other-step-id",
		Data:   json.RawMessage(`{"api_key":"sk-abc"}`),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "step_id")
}

// TestApiKeySubmit_RejectsSchemaViolation asserts the credentials step's
// json_schema is enforced — missing api_key returns a 400 before encryption.
func TestApiKeySubmit_RejectsSchemaViolation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, _, _ := newTestApiKeyConnection(t, ctrl, &cschema.ApiKeyPlacement{
		Type: cschema.ApiKeyPlacementBearer,
	}, nil)
	step := cschema.MustNewSetupStep(cschema.SynthesizedApiKeyCredentialsStepId)
	conn.SetupStep = &step

	// No InsertApiKeyCredential expected.

	ctx, _ := contextWithActor(t)
	_, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: cschema.SynthesizedApiKeyCredentialsStepId,
		Data:   json.RawMessage(`{}`), // missing required api_key
	})
	require.Error(t, err)
}

// TestApiKeySubmit_RejectsBasicMissingUsername asserts the basic placement
// requires the configured username field. JSON Schema marks it as required, so
// this surfaces as a schema validation error.
func TestApiKeySubmit_RejectsBasicMissingUsername(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, _, _ := newTestApiKeyConnection(t, ctrl, &cschema.ApiKeyPlacement{
		Type:          cschema.ApiKeyPlacementBasic,
		UsernameField: "account_id",
	}, nil)
	step := cschema.MustNewSetupStep(cschema.SynthesizedApiKeyCredentialsStepId)
	conn.SetupStep = &step

	// No InsertApiKeyCredential expected.

	ctx, _ := contextWithActor(t)
	_, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: cschema.SynthesizedApiKeyCredentialsStepId,
		Data:   json.RawMessage(`{"api_key":"sk-abc"}`), // missing required account_id
	})
	require.Error(t, err)
}

// TestApiKeySubmit_RejectsUnknownAuthTypeOnCredentialsPhase guards against a
// malformed connector configuration: a credentials phase declared in YAML for
// a non-api-key auth type. submitCredentialsStep's default branch should
// surface a 500 rather than persisting anything.
func TestApiKeySubmit_RejectsUnknownAuthTypeOnCredentialsPhase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	e := encrypt.NewFakeEncryptService(false)
	s, _, _, _, _, _ := FullMockService(t, ctrl)
	s.encrypt = e

	// OAuth2 connector with a manually-declared credentials phase — not
	// produced by Normalize, but possible if a YAML author hand-writes it.
	connector := cschema.Connector{
		Auth: &cschema.Auth{InnerVal: &cschema.AuthOAuth2{Type: cschema.AuthTypeOAuth2}},
		SetupFlow: &cschema.SetupFlow{
			Credentials: &cschema.SetupFlowPhase{Steps: []cschema.SetupFlowStep{{
				Id:         "creds",
				JsonSchema: common.RawJSON(`{"type":"object","properties":{"api_key":{"type":"string"}},"required":["api_key"]}`),
			}}},
		},
	}
	cv := NewTestConnectorVersion(connector)
	conn := &connection{
		Connection: database.Connection{
			Id:               "cxn_test4444444444aa",
			Namespace:        "root",
			State:            database.ConnectionStateSetup,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      s,
		cv:     cv,
		logger: aplog.NewNoopLogger(),
	}
	step := cschema.MustNewSetupStep("creds")
	conn.SetupStep = &step

	ctx, _ := contextWithActor(t)
	_, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: "creds",
		Data:   json.RawMessage(`{"api_key":"sk-abc"}`),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not accept credentials submissions")
}

// TestApiKeySubmit_DBErrorSurfacesUnchanged asserts a database failure from
// InsertApiKeyCredential propagates out of SubmitForm and the connection state
// is NOT advanced (no SetConnectionSetupStep / SetConnectionState calls).
func TestApiKeySubmit_DBErrorSurfacesUnchanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, db, _ := newTestApiKeyConnection(t, ctrl, &cschema.ApiKeyPlacement{
		Type: cschema.ApiKeyPlacementBearer,
	}, nil)
	step := cschema.MustNewSetupStep(cschema.SynthesizedApiKeyCredentialsStepId)
	conn.SetupStep = &step

	ctx, actorId := contextWithActor(t)
	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, gomock.Any(), gomock.Any(), &actorId).
		Return(nil, context.DeadlineExceeded)
	// No SetConnectionSetupStep / SetConnectionState expected — submit must
	// error out before advancing.

	_, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: cschema.SynthesizedApiKeyCredentialsStepId,
		Data:   json.RawMessage(`{"api_key":"sk-abc"}`),
	})
	require.Error(t, err)
}

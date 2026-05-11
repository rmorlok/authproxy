package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	mockAsynq "github.com/rmorlok/authproxy/internal/apasynq/mock"
	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestApiKeyConnection builds a test connection backed by an AuthApiKey
// connector with the given placement. The connector is Normalize()d first so
// the synthesized preconnect step is present, mirroring runtime behavior.
func newTestApiKeyConnection(
	t *testing.T,
	ctrl *gomock.Controller,
	placement *cschema.ApiKeyPlacement,
	probes []cschema.Probe,
) (*connection, *mockDb.MockDB, *mockAsynq.MockClient) {
	t.Helper()

	e := encrypt.NewFakeEncryptService(false)
	s, db, _, _, ac, _ := FullMockService(t, ctrl)
	s.encrypt = e

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
			State:            database.ConnectionStateCreated,
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

// captureEncConfig captures the value passed to SetConnectionEncryptedConfiguration.
type captureEncConfig struct{ field *encfield.EncryptedField }

func (c *captureEncConfig) Matches(x any) bool {
	v, ok := x.(*encfield.EncryptedField)
	if !ok {
		return false
	}
	c.field = v
	return true
}
func (c *captureEncConfig) String() string { return "captured *encfield.EncryptedField" }

// captureEncCredential captures the value passed to InsertApiKeyCredential.
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

	step := cschema.MustNewIndexedSetupStep(cschema.SetupPhasePreconnect, 0)
	conn.SetupStep = &step

	configCap := &captureEncConfig{}
	credCap := &captureEncCredential{}
	ctx, actorId := contextWithActor(t)

	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, credCap, gomock.AssignableToTypeOf(&cschema.ApiKeyPlacement{}), &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)
	db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, configCap).Return(nil)
	db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*cschema.SetupStep)(nil)).Return(nil)
	db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateReady).Return(nil)

	resp, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: cschema.SynthesizedApiKeyPreconnectStepId,
		Data:   json.RawMessage(`{"api_key":"sk-abc-123"}`),
	})
	require.NoError(t, err)
	require.IsType(t, &iface.ConnectionSetupComplete{}, resp)

	// Credential blob holds the JSON plaintext (fake encrypt stores plaintext in Data).
	var plaintext database.ApiKeyCredentialPlaintext
	require.NoError(t, json.Unmarshal([]byte(credCap.field.Data), &plaintext))
	require.Equal(t, "sk-abc-123", plaintext.ApiKey)
	require.Empty(t, plaintext.Username)

	// EncryptedConfiguration must not contain the api_key plaintext.
	require.NotNil(t, configCap.field)
	require.False(t, strings.Contains(configCap.field.Data, "sk-abc-123"),
		"api_key plaintext leaked into EncryptedConfiguration")
	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(configCap.field.Data), &cfg))
	require.NotContains(t, cfg, "api_key", "api_key field must be stripped from connection config")
}

func TestApiKeySubmit_BasicPersistsBothCredentialFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, db, _ := newTestApiKeyConnection(t, ctrl, &cschema.ApiKeyPlacement{
		Type:          cschema.ApiKeyPlacementBasic,
		UsernameField: "account_id",
	}, nil)

	step := cschema.MustNewIndexedSetupStep(cschema.SetupPhasePreconnect, 0)
	conn.SetupStep = &step

	configCap := &captureEncConfig{}
	credCap := &captureEncCredential{}
	ctx, actorId := contextWithActor(t)

	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, credCap, gomock.AssignableToTypeOf(&cschema.ApiKeyPlacement{}), &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)
	db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, configCap).Return(nil)
	db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*cschema.SetupStep)(nil)).Return(nil)
	db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateReady).Return(nil)

	resp, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: cschema.SynthesizedApiKeyPreconnectStepId,
		Data:   json.RawMessage(`{"api_key":"key-xyz","account_id":"acct-001"}`),
	})
	require.NoError(t, err)
	require.IsType(t, &iface.ConnectionSetupComplete{}, resp)

	var plaintext database.ApiKeyCredentialPlaintext
	require.NoError(t, json.Unmarshal([]byte(credCap.field.Data), &plaintext))
	require.Equal(t, "key-xyz", plaintext.ApiKey)
	require.Equal(t, "acct-001", plaintext.Username)

	// Neither credential field nor value should appear in the merged config.
	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(configCap.field.Data), &cfg))
	require.NotContains(t, cfg, "api_key")
	require.NotContains(t, cfg, "account_id")
	assert.False(t, strings.Contains(configCap.field.Data, "key-xyz"))
	assert.False(t, strings.Contains(configCap.field.Data, "acct-001"))
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
	step := cschema.MustNewIndexedSetupStep(cschema.SetupPhasePreconnect, 0)
	conn.SetupStep = &step

	ctx, actorId := contextWithActor(t)
	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, gomock.Any(), gomock.Any(), &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)
	db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil)
	db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &cschema.SetupStepVerify).Return(nil)
	ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)

	resp, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: cschema.SynthesizedApiKeyPreconnectStepId,
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
	step := cschema.MustNewIndexedSetupStep(cschema.SetupPhasePreconnect, 0)
	conn.SetupStep = &step

	ctx, actorId := contextWithActor(t)
	db.EXPECT().
		InsertApiKeyCredential(gomock.Any(), conn.Id, gomock.Any(), gomock.Any(), &actorId).
		Return(&database.ApiKeyCredential{Id: apid.New(apid.PrefixApiKeyCredential)}, nil)
	db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil)
	db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*cschema.SetupStep)(nil)).Return(nil)
	db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateReady).Return(nil)

	// CRITICAL: GetActiveApiKeyCredential must NOT be called during submit.
	// gomock's default expectation is zero calls, so omitting an EXPECT here
	// suffices — if submit reads the prior credential, ctrl.Finish() will fail.

	resp, err := conn.SubmitForm(ctx, iface.SubmitConnectionRequest{
		StepId: cschema.SynthesizedApiKeyPreconnectStepId,
		Data:   json.RawMessage(`{"api_key":"new-key"}`),
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

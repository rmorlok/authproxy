package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	mockAsynq "github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var tenantSchema = common.RawJSON(`{
	"type": "object",
	"required": ["tenant"],
	"properties": {
		"tenant": {"type": "string"}
	}
}`)

var regionSchema = common.RawJSON(`{
	"type": "object",
	"properties": {
		"region": {"type": "string"}
	}
}`)

var workspaceSchema = common.RawJSON(`{
	"type": "object",
	"required": ["workspace_id"],
	"properties": {
		"workspace_id": {"type": "string"}
	}
}`)

func newTestConnectionWithSetupFlow(t *testing.T, ctrl *gomock.Controller, sf *cschema.SetupFlow) (*connection, *mockDb.MockDB) {
	conn, db, _ := newTestConnectionWithSetupFlowAndAsynq(t, ctrl, sf)
	return conn, db
}

func newTestConnectionWithSetupFlowAndAsynq(t *testing.T, ctrl *gomock.Controller, sf *cschema.SetupFlow) (*connection, *mockDb.MockDB, *mockAsynq.MockClient) {
	e := encrypt.NewFakeEncryptService(false)
	s, db, _, _, ac, _ := FullMockService(t, ctrl)
	s.encrypt = e

	connector := cschema.Connector{
		SetupFlow: sf,
	}
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

func TestSubmitForm(t *testing.T) {
	t.Run("returns error when no setup step", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", JsonSchema: tenantSchema},
				},
			},
		})

		_, err := conn.SubmitForm(context.Background(), iface.SubmitConnectionRequest{
			StepId: "tenant",
			Data:   json.RawMessage(`{"tenant":"acme"}`),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no active setup step")
	})

	t.Run("returns error when current step is not in the manifest", func(t *testing.T) {
		// Connector has no setup flow at all; a request that names a step id
		// that isn't materialized in the manifest is a 500 (manifest /
		// connection state disagreement).
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, nil)
		step := cschema.MustNewSetupStep("tenant")
		conn.SetupStep = &step

		_, err := conn.SubmitForm(context.Background(), iface.SubmitConnectionRequest{
			StepId: "tenant",
			Data:   json.RawMessage(`{"key":"value"}`),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not addressable in manifest")
	})

	t.Run("advances preconnect:0 to preconnect:1", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sf := &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", Title: "Tenant", JsonSchema: tenantSchema},
					{Id: "region", Title: "Region", JsonSchema: regionSchema},
				},
			},
		}

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, sf)
		step := cschema.MustNewSetupStep("tenant")
		conn.SetupStep = &step

		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil)
		nextStep := cschema.MustNewSetupStep("region")
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &nextStep).Return(nil)

		resp, err := conn.SubmitForm(context.Background(), iface.SubmitConnectionRequest{
			StepId: "tenant",
			Data:   json.RawMessage(`{"tenant":"acme"}`),
		})
		require.NoError(t, err)
		require.IsType(t, &iface.ConnectionSetupForm{}, resp)

		form := resp.(*iface.ConnectionSetupForm)
		assert.Equal(t, "region", form.StepId)
		assert.Equal(t, "Region", form.StepTitle)
		assert.Equal(t, 1, form.CurrentStep)
		assert.Equal(t, 2, form.TotalSteps)
	})

	t.Run("last preconnect step transitions to OAuth2 redirect requires return_to_url", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sf := &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", JsonSchema: tenantSchema},
				},
			},
		}

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, sf)
		// Promote the no-auth connector to OAuth2 so the manifest emits the
		// authorize redirect step after the last preconnect step.
		conn.cv = NewTestConnectorVersion(cschema.Connector{
			Auth:      &cschema.Auth{InnerVal: &cschema.AuthOAuth2{Type: cschema.AuthTypeOAuth2}},
			SetupFlow: sf,
		})
		conn.ConnectorId = conn.cv.GetId()
		conn.ConnectorVersion = conn.cv.GetVersion()
		step := cschema.MustNewSetupStep("tenant")
		conn.SetupStep = &step

		// OnSubmit will merge config, then advanceToStep will render the
		// redirect — which rejects (return_to_url empty) before SetSetupStep
		// is called. So the only DB write we expect is the config merge.
		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil)

		_, err := conn.SubmitForm(context.Background(), iface.SubmitConnectionRequest{
			StepId: "tenant",
			Data:   json.RawMessage(`{"tenant":"acme"}`),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "return_to_url is required")
	})

	t.Run("configure step completes flow", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sf := &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "workspace", JsonSchema: workspaceSchema},
				},
			},
		}

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, sf)
		step := cschema.MustNewSetupStep("workspace")
		conn.SetupStep = &step

		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil)
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*cschema.SetupStep)(nil)).Return(nil)
		db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateConfigured).Return(nil)

		resp, err := conn.SubmitForm(context.Background(), iface.SubmitConnectionRequest{
			StepId: "workspace",
			Data:   json.RawMessage(`{"workspace_id":"ws-123"}`),
		})
		require.NoError(t, err)
		require.IsType(t, &iface.ConnectionSetupComplete{}, resp)
		assert.Equal(t, iface.ConnectionSetupResponseTypeComplete, resp.GetType())
	})

	t.Run("merges data from multiple steps", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		step1Schema := common.RawJSON(`{
			"type": "object",
			"properties": {
				"tenant": {"type": "string"}
			}
		}`)
		step2Schema := common.RawJSON(`{
			"type": "object",
			"properties": {
				"workspace": {"type": "string"}
			}
		}`)

		sf := &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "step1", JsonSchema: step1Schema},
					{Id: "step2", JsonSchema: step2Schema},
				},
			},
		}

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, sf)
		step := cschema.MustNewSetupStep("step1")
		conn.SetupStep = &step

		// First submit — sets config with tenant
		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil)
		nextStep := cschema.MustNewSetupStep("step2")
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &nextStep).Return(nil)

		resp, err := conn.SubmitForm(context.Background(), iface.SubmitConnectionRequest{
			StepId: "step1",
			Data:   json.RawMessage(`{"tenant":"acme"}`),
		})
		require.NoError(t, err)
		require.IsType(t, &iface.ConnectionSetupForm{}, resp)

		// Second submit — merges workspace into existing config
		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil)
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*cschema.SetupStep)(nil)).Return(nil)
		db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateConfigured).Return(nil)

		resp, err = conn.SubmitForm(context.Background(), iface.SubmitConnectionRequest{
			StepId: "step2",
			Data:   json.RawMessage(`{"workspace":"main"}`),
		})
		require.NoError(t, err)
		require.IsType(t, &iface.ConnectionSetupComplete{}, resp)

		// Verify both values are in config
		cfg, err := conn.GetConfiguration(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "acme", cfg["tenant"])
		assert.Equal(t, "main", cfg["workspace"])
	})
}

func TestGetCurrentSetupStepResponse(t *testing.T) {
	t.Run("returns complete when no setup step", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})

		resp, err := conn.GetCurrentSetupStepResponse(context.Background())
		require.NoError(t, err)
		assert.Equal(t, iface.ConnectionSetupResponseTypeComplete, resp.GetType())
	})

	t.Run("returns form for preconnect step", func(t *testing.T) {
		// Resume path is read-only — no setup_step writes.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sf := &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", Title: "Enter Tenant", JsonSchema: tenantSchema},
				},
			},
		}
		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, sf)
		step := cschema.MustNewSetupStep("tenant")
		conn.SetupStep = &step

		resp, err := conn.GetCurrentSetupStepResponse(context.Background())
		require.NoError(t, err)
		require.IsType(t, &iface.ConnectionSetupForm{}, resp)

		form := resp.(*iface.ConnectionSetupForm)
		assert.Equal(t, "tenant", form.StepId)
		assert.Equal(t, "Enter Tenant", form.StepTitle)
		assert.Equal(t, 0, form.CurrentStep)
		assert.Equal(t, 1, form.TotalSteps)
	})

	t.Run("returns redirect for OAuth2 authorize step", func(t *testing.T) {
		// Resume path returns the redirect response type with an empty URL —
		// the user has been redirected already and the UI knows to wait on
		// the callback rather than render a fresh URL.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", JsonSchema: tenantSchema},
				},
			},
		})
		conn.cv = NewTestConnectorVersion(cschema.Connector{
			Auth: &cschema.Auth{InnerVal: &cschema.AuthOAuth2{Type: cschema.AuthTypeOAuth2}},
			SetupFlow: &cschema.SetupFlow{
				Preconnect: &cschema.SetupFlowPhase{
					Steps: []cschema.SetupFlowStep{{Id: "tenant", JsonSchema: tenantSchema}},
				},
			},
		})
		conn.ConnectorId = conn.cv.GetId()
		conn.ConnectorVersion = conn.cv.GetVersion()
		step := cschema.MustNewSetupStep(oauth2.OAuth2AuthorizeStepId)
		conn.SetupStep = &step

		resp, err := conn.GetCurrentSetupStepResponse(context.Background())
		require.NoError(t, err)
		assert.Equal(t, iface.ConnectionSetupResponseTypeRedirect, resp.GetType())
		// Resume mode: URL is intentionally empty.
		assert.Empty(t, resp.(*iface.ConnectionSetupRedirect).RedirectUrl)
	})

	t.Run("returns form for configure step", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sf := &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "workspace", Title: "Select Workspace", JsonSchema: workspaceSchema},
				},
			},
		}
		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, sf)
		step := cschema.MustNewSetupStep("workspace")
		conn.SetupStep = &step

		resp, err := conn.GetCurrentSetupStepResponse(context.Background())
		require.NoError(t, err)
		require.IsType(t, &iface.ConnectionSetupForm{}, resp)

		form := resp.(*iface.ConnectionSetupForm)
		assert.Equal(t, "workspace", form.StepId)
		assert.Equal(t, "Select Workspace", form.StepTitle)
	})

	t.Run("returns verifying for verify step", func(t *testing.T) {
		// apxy:verify is only addressable in the manifest when the connector
		// has probes — that's the only way the connection ends up there.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})
		conn.cv.GetDefinition().Probes = []cschema.Probe{{Id: "ping"}}
		step := cschema.SetupStepVerify
		conn.SetupStep = &step

		resp, err := conn.GetCurrentSetupStepResponse(context.Background())
		require.NoError(t, err)
		require.IsType(t, &iface.ConnectionSetupVerifying{}, resp)
		assert.Equal(t, iface.ConnectionSetupResponseTypeVerifying, resp.GetType())
	})

	t.Run("returns error for verify_failed step with setup_error populated", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})
		step := cschema.SetupStepVerifyFailed
		conn.SetupStep = &step
		errMsg := `probe "ping" failed: 401 unauthorized`
		conn.SetupError = &errMsg

		resp, err := conn.GetCurrentSetupStepResponse(context.Background())
		require.NoError(t, err)
		require.IsType(t, &iface.ConnectionSetupError{}, resp)
		errResp := resp.(*iface.ConnectionSetupError)
		assert.Equal(t, errMsg, errResp.Error)
		assert.True(t, errResp.CanRetry)
	})
}

package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	mockAsynq "github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
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

	t.Run("returns error when connector has no setup flow", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, nil)
		step := "preconnect:0"
		conn.SetupStep = &step

		_, err := conn.SubmitForm(context.Background(), iface.SubmitConnectionRequest{
			StepId: "tenant",
			Data:   json.RawMessage(`{"key":"value"}`),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no setup flow")
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
		step := "preconnect:0"
		conn.SetupStep = &step

		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil)
		nextStep := "preconnect:1"
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &nextStep).Return(nil)

		resp, err := conn.SubmitForm(context.Background(), iface.SubmitConnectionRequest{
			StepId: "tenant",
			Data:   json.RawMessage(`{"tenant":"acme"}`),
		})
		require.NoError(t, err)
		require.IsType(t, &iface.InitiateConnectionForm{}, resp)

		form := resp.(*iface.InitiateConnectionForm)
		assert.Equal(t, "region", form.StepId)
		assert.Equal(t, "Region", form.StepTitle)
		assert.Equal(t, 1, form.CurrentStep)
		assert.Equal(t, 2, form.TotalSteps)
	})

	t.Run("last preconnect step transitions to auth requires return_to_url", func(t *testing.T) {
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
		step := "preconnect:0"
		conn.SetupStep = &step

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
		step := "configure:0"
		conn.SetupStep = &step

		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil)
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*string)(nil)).Return(nil)
		db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateReady).Return(nil)

		resp, err := conn.SubmitForm(context.Background(), iface.SubmitConnectionRequest{
			StepId: "workspace",
			Data:   json.RawMessage(`{"workspace_id":"ws-123"}`),
		})
		require.NoError(t, err)
		require.IsType(t, &iface.InitiateConnectionComplete{}, resp)
		assert.Equal(t, iface.PreconnectionResponseTypeComplete, resp.GetType())
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
		step := "configure:0"
		conn.SetupStep = &step

		// First submit — sets config with tenant
		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil)
		nextStep := "configure:1"
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &nextStep).Return(nil)

		resp, err := conn.SubmitForm(context.Background(), iface.SubmitConnectionRequest{
			StepId: "step1",
			Data:   json.RawMessage(`{"tenant":"acme"}`),
		})
		require.NoError(t, err)
		require.IsType(t, &iface.InitiateConnectionForm{}, resp)

		// Second submit — merges workspace into existing config
		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil)
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*string)(nil)).Return(nil)
		db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateReady).Return(nil)

		resp, err = conn.SubmitForm(context.Background(), iface.SubmitConnectionRequest{
			StepId: "step2",
			Data:   json.RawMessage(`{"workspace":"main"}`),
		})
		require.NoError(t, err)
		require.IsType(t, &iface.InitiateConnectionComplete{}, resp)

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
		assert.Equal(t, iface.PreconnectionResponseTypeComplete, resp.GetType())
	})

	t.Run("returns form for preconnect step", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sf := &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", Title: "Enter Tenant", JsonSchema: tenantSchema},
				},
			},
		}
		conn, db := newTestConnectionWithSetupFlow(t, ctrl, sf)
		step := "preconnect:0"
		conn.SetupStep = &step

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &step).Return(nil)

		resp, err := conn.GetCurrentSetupStepResponse(context.Background())
		require.NoError(t, err)
		require.IsType(t, &iface.InitiateConnectionForm{}, resp)

		form := resp.(*iface.InitiateConnectionForm)
		assert.Equal(t, "tenant", form.StepId)
		assert.Equal(t, "Enter Tenant", form.StepTitle)
		assert.Equal(t, 0, form.CurrentStep)
		assert.Equal(t, 1, form.TotalSteps)
	})

	t.Run("returns redirect for auth step", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", JsonSchema: tenantSchema},
				},
			},
		})
		step := "auth"
		conn.SetupStep = &step

		resp, err := conn.GetCurrentSetupStepResponse(context.Background())
		require.NoError(t, err)
		assert.Equal(t, iface.PreconnectionResponseTypeRedirect, resp.GetType())
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
		conn, db := newTestConnectionWithSetupFlow(t, ctrl, sf)
		step := "configure:0"
		conn.SetupStep = &step

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &step).Return(nil)

		resp, err := conn.GetCurrentSetupStepResponse(context.Background())
		require.NoError(t, err)
		require.IsType(t, &iface.InitiateConnectionForm{}, resp)

		form := resp.(*iface.InitiateConnectionForm)
		assert.Equal(t, "workspace", form.StepId)
		assert.Equal(t, "Select Workspace", form.StepTitle)
	})

	t.Run("returns verifying for verify step", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})
		step := cschema.SetupStepVerify
		conn.SetupStep = &step

		resp, err := conn.GetCurrentSetupStepResponse(context.Background())
		require.NoError(t, err)
		require.IsType(t, &iface.InitiateConnectionVerifying{}, resp)
		assert.Equal(t, iface.PreconnectionResponseTypeVerifying, resp.GetType())
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
		require.IsType(t, &iface.InitiateConnectionError{}, resp)
		errResp := resp.(*iface.InitiateConnectionError)
		assert.Equal(t, errMsg, errResp.Error)
		assert.True(t, errResp.CanRetry)
	})
}

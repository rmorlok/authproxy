package core

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconfigure(t *testing.T) {
	t.Run("returns error when connection not in ready state", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "workspace", JsonSchema: workspaceSchema},
				},
			},
		})
		conn.State = database.ConnectionStateSetup

		_, err := conn.Reconfigure(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be configured")
	})

	t.Run("returns error when connector has no configure steps", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", JsonSchema: tenantSchema},
				},
			},
		})
		conn.State = database.ConnectionStateConfigured

		_, err := conn.Reconfigure(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no configure steps")
	})

	t.Run("returns error when connector has no setup flow", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, nil)
		conn.State = database.ConnectionStateConfigured

		_, err := conn.Reconfigure(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no configure steps")
	})

	t.Run("returns first configure step form on success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "workspace", Title: "Select Workspace", JsonSchema: workspaceSchema},
				},
			},
		})
		conn.State = database.ConnectionStateConfigured

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewSetupStep("workspace"))).Return(nil)

		resp, err := conn.Reconfigure(context.Background())
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, iface.ConnectionSetupResponseTypeForm, resp.GetType())
	})

	t.Run("returns first configure step when preconnect steps also exist", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", JsonSchema: tenantSchema},
				},
			},
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "workspace", Title: "Select Workspace", JsonSchema: workspaceSchema},
					{Id: "settings", Title: "Settings", JsonSchema: workspaceSchema},
				},
			},
		})
		conn.State = database.ConnectionStateConfigured

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewSetupStep("workspace"))).Return(nil)

		resp, err := conn.Reconfigure(context.Background())
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, iface.ConnectionSetupResponseTypeForm, resp.GetType())
	})

	t.Run("skips ineligible configure steps", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{
						Id:         "us_only",
						JsonSchema: workspaceSchema,
						If:         &common.Predicate{Javascript: `cfg.region === "us"`},
					},
					{Id: "workspace", Title: "Select Workspace", JsonSchema: workspaceSchema},
				},
			},
		})
		conn.State = database.ConnectionStateConfigured
		setConnectionConfigFixture(t, conn, map[string]any{"region": "eu"})

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewSetupStep("workspace"))).Return(nil)

		resp, err := conn.Reconfigure(context.Background())
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.IsType(t, &iface.ConnectionSetupForm{}, resp)
		assert.Equal(t, "workspace", resp.(*iface.ConnectionSetupForm).StepId)
	})
}

func ptrStr(s string) *string {
	return &s
}

func ptrStep(s cschema.SetupStep) *cschema.SetupStep {
	return &s
}

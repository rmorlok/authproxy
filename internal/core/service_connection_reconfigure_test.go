package core

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
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
		conn.State = database.ConnectionStateCreated

		_, err := conn.Reconfigure(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ready state")
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
		conn.State = database.ConnectionStateReady

		_, err := conn.Reconfigure(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no configure steps")
	})

	t.Run("returns error when connector has no setup flow", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, nil)
		conn.State = database.ConnectionStateReady

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
		conn.State = database.ConnectionStateReady

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewIndexedSetupStep(cschema.SetupPhaseConfigure, 0))).Return(nil)

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
		conn.State = database.ConnectionStateReady

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewIndexedSetupStep(cschema.SetupPhaseConfigure, 0))).Return(nil)

		resp, err := conn.Reconfigure(context.Background())
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, iface.ConnectionSetupResponseTypeForm, resp.GetType())
	})
}

func ptrStr(s string) *string {
	return &s
}

func ptrStep(s cschema.SetupStep) *cschema.SetupStep {
	return &s
}

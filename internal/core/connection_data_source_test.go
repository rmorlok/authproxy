package core

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDataSource(t *testing.T) {
	t.Run("returns error when no setup step", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{
						Id:         "workspace",
						JsonSchema: workspaceSchema,
						DataSources: map[string]cschema.DataSourceDef{
							"workspaces": {
								ProxyRequest: &cschema.DataSourceProxyRequest{
									Method: "GET",
									Url:    "https://api.example.com/workspaces",
								},
								Transform: "data",
							},
						},
					},
				},
			},
		})

		_, err := conn.GetDataSource(context.Background(), "workspaces")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no active setup step")
	})

	t.Run("returns error when in preconnect phase", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", JsonSchema: tenantSchema},
				},
			},
		})
		step := cschema.MustNewIndexedSetupStep(cschema.SetupPhasePreconnect, 0)
		conn.SetupStep = &step

		_, err := conn.GetDataSource(context.Background(), "some_source")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only available during configure steps")
	})

	t.Run("returns error when in auth phase", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "workspace", JsonSchema: workspaceSchema},
				},
			},
		})
		step := cschema.SetupStepAuth
		conn.SetupStep = &step

		_, err := conn.GetDataSource(context.Background(), "some_source")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only available during configure steps")
	})

	t.Run("returns error when no setup flow on connector", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, nil)
		step := cschema.MustNewIndexedSetupStep(cschema.SetupPhaseConfigure, 0)
		conn.SetupStep = &step

		_, err := conn.GetDataSource(context.Background(), "workspaces")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no setup flow")
	})

	t.Run("returns error when data source not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{
						Id: "workspace",
						JsonSchema: common.RawJSON(`{
							"type": "object",
							"properties": {
								"workspace_id": {"type": "string"}
							}
						}`),
					},
				},
			},
		})
		step := cschema.MustNewIndexedSetupStep(cschema.SetupPhaseConfigure, 0)
		conn.SetupStep = &step

		_, err := conn.GetDataSource(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found in current step")
	})

	t.Run("returns error when data source has no proxy request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{
						Id: "workspace",
						JsonSchema: common.RawJSON(`{
							"type": "object",
							"properties": {
								"workspace_id": {"type": "string"}
							}
						}`),
						DataSources: map[string]cschema.DataSourceDef{
							"workspaces": {
								Transform: "data",
							},
						},
					},
				},
			},
		})
		step := cschema.MustNewIndexedSetupStep(cschema.SetupPhaseConfigure, 0)
		conn.SetupStep = &step

		_, err := conn.GetDataSource(context.Background(), "workspaces")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no proxy_request defined")
	})
}

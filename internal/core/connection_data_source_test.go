package core

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDataSource(t *testing.T) {
	t.Run("transforms response with connector javascript context", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, proxy := newProbeTestConnection(t, ctrl, cschema.Connector{
			Javascript: `
				function workspaceOptions(items) {
					return items.map(function(item) {
						return {
							value: labels.env + ":" + item.id,
							label: cfg.region + "/" + annotations.prefix + "/" + item.summary
						};
					});
				}
			`,
			SetupFlow: &cschema.SetupFlow{
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
									Transform: "workspaceOptions(data.items)",
								},
							},
						},
					},
				},
			},
		})
		step := cschema.MustNewSetupStep("workspace")
		conn.SetupStep = &step
		conn.s.encrypt = encrypt.NewFakeEncryptService(false)
		setConnectionConfigFixture(t, conn, map[string]any{"region": "us-east"})
		conn.Labels = map[string]string{"env": "prod"}
		conn.Annotations = map[string]string{"prefix": "team"}
		proxy.resp = &iface.ProxyResponse{
			StatusCode: 200,
			BodyJson: map[string]any{
				"items": []any{
					map[string]any{"id": "primary", "summary": "Primary"},
				},
			},
		}

		options, err := conn.GetDataSource(context.Background(), "workspaces")
		require.NoError(t, err)
		require.Len(t, options, 1)
		assert.Equal(t, "prod:primary", options[0].Value)
		assert.Equal(t, "us-east/team/Primary", options[0].Label)
		require.Len(t, proxy.calls, 1)
		assert.Equal(t, "https://api.example.com/workspaces", proxy.calls[0].URL)
	})

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
		step := cschema.MustNewSetupStep("tenant")
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
		// A pseudo-step (or any non-configure step) gates data-source access.
		step := cschema.SetupStepVerify
		conn.SetupStep = &step

		_, err := conn.GetDataSource(context.Background(), "some_source")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only available during configure steps")
	})

	t.Run("returns error when no setup flow on connector", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, nil)
		step := cschema.MustNewSetupStep("workspace")
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
		step := cschema.MustNewSetupStep("workspace")
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
		step := cschema.MustNewSetupStep("workspace")
		conn.SetupStep = &step

		_, err := conn.GetDataSource(context.Background(), "workspaces")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no proxy_request defined")
	})
}

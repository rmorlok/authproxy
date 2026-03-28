package connectors

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSetupFlowValidation(t *testing.T) {
	vc := &common.ValidationContext{Path: "setup_flow"}

	t.Run("nil setup flow is valid", func(t *testing.T) {
		var sf *SetupFlow
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("empty setup flow is valid", func(t *testing.T) {
		sf := &SetupFlow{}
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("valid preconnect only", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "tenant",
						Title:      "Enter tenant",
						JsonSchema: common.RawJSON(`{"type":"object","properties":{"tenant":{"type":"string"}}}`),
					},
				},
			},
		}
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("valid configure only", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "workspace",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"workspaces": {
								ProxyRequest: &DataSourceProxyRequest{
									Method: "GET",
									Url:    "https://api.example.com/workspaces",
								},
								Transform: "data.map(w => ({value: w.id, label: w.name}))",
							},
						},
					},
				},
			},
		}
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("valid preconnect and configure", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "tenant",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
				},
			},
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "workspace",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
				},
			},
		}
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("duplicate step id across phases", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "same-id",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
				},
			},
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "same-id",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate step id")
	})

	t.Run("duplicate step id within phase", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate step id")
	})

	t.Run("empty phase has error", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have at least one step")
	})

	t.Run("step missing id", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "step id is required")
	})

	t.Run("step missing json_schema", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id: "step1",
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "json_schema is required")
	})

	t.Run("step with invalid json_schema", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{not valid json}`),
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "json_schema is not valid JSON")
	})

	t.Run("step with invalid ui_schema", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						UiSchema:   common.RawJSON(`{bad json`),
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ui_schema is not valid JSON")
	})

	t.Run("data_sources not allowed in preconnect", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								ProxyRequest: &DataSourceProxyRequest{Method: "GET", Url: "https://api.example.com"},
								Transform:    "data",
							},
						},
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "data_sources are not allowed in preconnect")
	})

	t.Run("data_sources allowed in configure", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								ProxyRequest: &DataSourceProxyRequest{Method: "GET", Url: "https://api.example.com"},
								Transform:    "data",
							},
						},
					},
				},
			},
		}
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("data source missing proxy_request", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								Transform: "data",
							},
						},
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "proxy_request is required")
	})

	t.Run("data source missing transform", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								ProxyRequest: &DataSourceProxyRequest{Method: "GET", Url: "https://api.example.com"},
							},
						},
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transform expression is required")
	})

	t.Run("proxy request missing method", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								ProxyRequest: &DataSourceProxyRequest{Url: "https://api.example.com"},
								Transform:    "data",
							},
						},
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "method is required")
	})

	t.Run("proxy request missing url", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								ProxyRequest: &DataSourceProxyRequest{Method: "GET"},
								Transform:    "data",
							},
						},
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "url is required")
	})
}

func TestSetupFlowHelpers(t *testing.T) {
	t.Run("HasPreconnect", func(t *testing.T) {
		assert.False(t, (*SetupFlow)(nil).HasPreconnect())
		assert.False(t, (&SetupFlow{}).HasPreconnect())
		assert.False(t, (&SetupFlow{Preconnect: &SetupFlowPhase{}}).HasPreconnect())
		assert.True(t, (&SetupFlow{Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "s"}}}}).HasPreconnect())
	})

	t.Run("HasConfigure", func(t *testing.T) {
		assert.False(t, (*SetupFlow)(nil).HasConfigure())
		assert.False(t, (&SetupFlow{}).HasConfigure())
		assert.False(t, (&SetupFlow{Configure: &SetupFlowPhase{}}).HasConfigure())
		assert.True(t, (&SetupFlow{Configure: &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "s"}}}}).HasConfigure())
	})
}

func TestSetupFlowYAMLRoundtrip(t *testing.T) {
	t.Run("parse connector with setup_flow from YAML", func(t *testing.T) {
		data, err := os.ReadFile("test_data/valid-oauth-connector-with-setup-flow.yaml")
		require.NoError(t, err)

		var connector Connector
		err = yaml.Unmarshal(data, &connector)
		require.NoError(t, err)

		require.NotNil(t, connector.SetupFlow)

		// Preconnect
		require.True(t, connector.SetupFlow.HasPreconnect())
		require.Len(t, connector.SetupFlow.Preconnect.Steps, 1)
		assert.Equal(t, "tenant", connector.SetupFlow.Preconnect.Steps[0].Id)
		assert.Equal(t, "Enter your Acme tenant", connector.SetupFlow.Preconnect.Steps[0].Title)
		assert.NotEmpty(t, connector.SetupFlow.Preconnect.Steps[0].JsonSchema)
		assert.NotEmpty(t, connector.SetupFlow.Preconnect.Steps[0].UiSchema)
		assert.Empty(t, connector.SetupFlow.Preconnect.Steps[0].DataSources)

		// Configure
		require.True(t, connector.SetupFlow.HasConfigure())
		require.Len(t, connector.SetupFlow.Configure.Steps, 2)

		step1 := connector.SetupFlow.Configure.Steps[0]
		assert.Equal(t, "select_workspace", step1.Id)
		assert.Equal(t, "Select Workspace", step1.Title)
		require.Len(t, step1.DataSources, 1)
		ds := step1.DataSources["workspaces"]
		require.NotNil(t, ds.ProxyRequest)
		assert.Equal(t, "GET", ds.ProxyRequest.Method)
		assert.Equal(t, "https://api.acme.com/v1/workspaces", ds.ProxyRequest.Url)
		assert.NotEmpty(t, ds.Transform)

		step2 := connector.SetupFlow.Configure.Steps[1]
		assert.Equal(t, "sync_options", step2.Id)
		assert.Empty(t, step2.DataSources)

		// Validate
		vc := &common.ValidationContext{Path: "connector"}
		assert.NoError(t, connector.Validate(vc))
	})

	t.Run("json roundtrip preserves setup_flow", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "tenant",
						Title:      "Enter tenant",
						JsonSchema: common.RawJSON(`{"type":"object","properties":{"tenant":{"type":"string"}}}`),
						UiSchema:   common.RawJSON(`{"type":"VerticalLayout"}`),
					},
				},
			},
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "workspace",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								ProxyRequest: &DataSourceProxyRequest{
									Method:  "GET",
									Url:     "https://api.example.com/items",
									Headers: map[string]string{"Accept": "application/json"},
								},
								Transform: "data.map(i => ({value: i.id, label: i.name}))",
							},
						},
					},
				},
			},
		}

		jsonBytes, err := json.Marshal(sf)
		require.NoError(t, err)

		var parsed SetupFlow
		err = json.Unmarshal(jsonBytes, &parsed)
		require.NoError(t, err)

		require.True(t, parsed.HasPreconnect())
		assert.Equal(t, "tenant", parsed.Preconnect.Steps[0].Id)
		require.True(t, parsed.HasConfigure())
		assert.Equal(t, "workspace", parsed.Configure.Steps[0].Id)
		require.NotNil(t, parsed.Configure.Steps[0].DataSources["items"].ProxyRequest)
		assert.Equal(t, "GET", parsed.Configure.Steps[0].DataSources["items"].ProxyRequest.Method)
		assert.Equal(t, "application/json", parsed.Configure.Steps[0].DataSources["items"].ProxyRequest.Headers["Accept"])
	})
}

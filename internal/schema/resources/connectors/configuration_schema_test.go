package connectors

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

func TestConnectionConfigurationJSONSchema(t *testing.T) {
	definition := &ConnectorDefinition{SetupFlow: &SetupFlow{
		Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{
			{
				Id: "tenant",
				JsonSchema: common.RawJSON(`{
					"type":"object",
					"required":["tenant"],
					"properties":{"tenant":{"type":"string","minLength":1}}
				}`),
			},
			{
				Id:       "redirect",
				Type:     SetupFlowStepTypeRedirect,
				Redirect: &SetupFlowStepRedirect{URL: "https://example.com"},
			},
		}},
		Configure: &SetupFlowPhase{Steps: []SetupFlowStep{
			{
				Id: "workspace",
				JsonSchema: common.RawJSON(`{
					"type":"object",
					"required":["workspace"],
					"properties":{"workspace":{"type":"string","enum":["sales","support"]}}
				}`),
			},
		}},
	}}

	schemaJSON, err := definition.ConnectionConfigurationJSONSchema()
	require.NoError(t, err)
	require.JSONEq(t, `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"required":["tenant","workspace"],
		"properties":{
			"tenant":{"type":"string","minLength":1},
			"workspace":{"type":"string","enum":["sales","support"]}
		},
		"additionalProperties":true
	}`, string(schemaJSON))

	compiled, err := jsonschemav5.CompileString("connection-configuration.json", string(schemaJSON))
	require.NoError(t, err)
	require.Error(t, compiled.Validate(map[string]any{"tenant": "acme"}))
	require.Error(t, compiled.Validate(map[string]any{"workspace": "sales"}))
	require.NoError(t, compiled.Validate(map[string]any{"tenant": "acme", "workspace": "sales"}))
	require.NoError(t, compiled.Validate(map[string]any{
		"tenant": "acme", "workspace": "sales", "unknown": true,
	}))
}

func TestConnectionConfigurationJSONSchemaOmitsConditionalRequirements(t *testing.T) {
	definition := &ConnectorDefinition{SetupFlow: &SetupFlow{
		Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{
			{
				Id:         "tenant",
				JsonSchema: common.RawJSON(`{"type":"object","required":["tenant"],"properties":{"tenant":{"type":"string"}}}`),
			},
			{
				Id:         "region",
				If:         &common.Predicate{Javascript: `cfg.tenant === "regional"`},
				JsonSchema: common.RawJSON(`{"type":"object","required":["region"],"properties":{"region":{"type":"string"}}}`),
			},
		}},
	}}

	schemaJSON, err := definition.ConnectionConfigurationJSONSchema()
	require.NoError(t, err)
	require.JSONEq(t, `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"required":["tenant"],
		"properties":{
			"tenant":{"type":"string"},
			"region":{"type":"string"}
		},
		"additionalProperties":true
	}`, string(schemaJSON))
}

func TestConnectionConfigurationJSONSchemaCombinesDuplicateFields(t *testing.T) {
	definition := &ConnectorDefinition{SetupFlow: &SetupFlow{
		Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{
			Id:         "account-name",
			JsonSchema: common.RawJSON(`{"type":"object","properties":{"account":{"type":"string"}}}`),
		}}},
		Configure: &SetupFlowPhase{Steps: []SetupFlowStep{{
			Id:         "account-number",
			JsonSchema: common.RawJSON(`{"type":"object","properties":{"account":{"type":"integer"}}}`),
		}}},
	}}

	schemaJSON, err := definition.ConnectionConfigurationJSONSchema()
	require.NoError(t, err)

	var schema struct {
		Properties map[string]struct {
			AnyOf []json.RawMessage `json:"anyOf"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(schemaJSON, &schema))
	require.Len(t, schema.Properties["account"].AnyOf, 2)

	compiled, err := jsonschemav5.CompileString("connection-configuration.json", string(schemaJSON))
	require.NoError(t, err)
	require.NoError(t, compiled.Validate(map[string]any{"account": "acme"}))
	require.NoError(t, compiled.Validate(map[string]any{"account": float64(42)}))
}

func TestConnectionConfigurationJSONSchemaWithoutSetupFlow(t *testing.T) {
	schemaJSON, err := (&ConnectorDefinition{}).ConnectionConfigurationJSONSchema()
	require.NoError(t, err)
	require.JSONEq(t, `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"properties":{},
		"additionalProperties":true
	}`, string(schemaJSON))
}

func TestConnectionConfigurationMatchesJSONSchema(t *testing.T) {
	schema := common.RawJSON(`{
		"type":"object",
		"required":["tenant"],
		"properties":{"tenant":{"type":"string"}}
	}`)

	matches, err := ConnectionConfigurationMatchesJSONSchema(schema, map[string]any{"tenant": "acme"})
	require.NoError(t, err)
	require.True(t, matches)

	matches, err = ConnectionConfigurationMatchesJSONSchema(schema, map[string]any{})
	require.NoError(t, err)
	require.False(t, matches)

	matches, err = ConnectionConfigurationMatchesJSONSchema(schema, map[string]any{"tenant": 42})
	require.NoError(t, err)
	require.False(t, matches)
}

func TestConnectionConfigurationMatchesJSONSchemaTreatsNilAsEmptyObject(t *testing.T) {
	matches, err := ConnectionConfigurationMatchesJSONSchema(
		common.RawJSON(`{"type":"object","properties":{}}`),
		nil,
	)
	require.NoError(t, err)
	require.True(t, matches)
}

func TestConnectionConfigurationMatchesJSONSchemaRejectsInvalidSchema(t *testing.T) {
	matches, err := ConnectionConfigurationMatchesJSONSchema(common.RawJSON(`{"type":`), nil)
	require.ErrorContains(t, err, "compile connection configuration schema")
	require.False(t, matches)
}

package connectors

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

func TestSynthesizeApiKeyPreconnectStep_Bearer(t *testing.T) {
	step := SynthesizeApiKeyPreconnectStep(&ApiKeyPlacement{Type: ApiKeyPlacementBearer})
	require.NotNil(t, step)
	require.Equal(t, SynthesizedApiKeyPreconnectStepId, step.Id)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(step.JsonSchema, &schema))
	props := schema["properties"].(map[string]any)
	require.Contains(t, props, "api_key")
	require.NotContains(t, props, "username") // No username for bearer

	required := toStringSlice(schema["required"])
	require.ElementsMatch(t, []string{"api_key"}, required)

	require.False(t, schema["additionalProperties"].(bool))
}

func TestSynthesizeApiKeyPreconnectStep_Header(t *testing.T) {
	// header placement renders the same form as bearer at this layer — the
	// header_name + prefix only affect the proxy, not the user-input form.
	step := SynthesizeApiKeyPreconnectStep(&ApiKeyPlacement{
		Type:       ApiKeyPlacementHeader,
		HeaderName: "X-API-Key",
	})
	require.NotNil(t, step)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(step.JsonSchema, &schema))
	props := schema["properties"].(map[string]any)
	require.Contains(t, props, "api_key")
	require.NotContains(t, props, "X-API-Key")
}

func TestSynthesizeApiKeyPreconnectStep_Query(t *testing.T) {
	step := SynthesizeApiKeyPreconnectStep(&ApiKeyPlacement{
		Type:      ApiKeyPlacementQuery,
		ParamName: "api_key",
	})
	require.NotNil(t, step)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(step.JsonSchema, &schema))
	props := schema["properties"].(map[string]any)
	require.Contains(t, props, "api_key")
}

func TestSynthesizeApiKeyPreconnectStep_Basic(t *testing.T) {
	step := SynthesizeApiKeyPreconnectStep(&ApiKeyPlacement{
		Type:          ApiKeyPlacementBasic,
		UsernameField: "account_id",
	})
	require.NotNil(t, step)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(step.JsonSchema, &schema))
	props := schema["properties"].(map[string]any)
	require.Contains(t, props, "api_key")
	require.Contains(t, props, "account_id")

	required := toStringSlice(schema["required"])
	require.ElementsMatch(t, []string{"api_key", "account_id"}, required)
}

func TestSynthesizeApiKeyPreconnectStep_Nil(t *testing.T) {
	require.Nil(t, SynthesizeApiKeyPreconnectStep(nil))
}

func TestApiKeyPlacement_CredentialFieldNames(t *testing.T) {
	require.ElementsMatch(t, []string{"api_key"}, (&ApiKeyPlacement{Type: ApiKeyPlacementBearer}).CredentialFieldNames())
	require.ElementsMatch(t, []string{"api_key"}, (&ApiKeyPlacement{Type: ApiKeyPlacementHeader, HeaderName: "X-K"}).CredentialFieldNames())
	require.ElementsMatch(t, []string{"api_key"}, (&ApiKeyPlacement{Type: ApiKeyPlacementQuery, ParamName: "k"}).CredentialFieldNames())
	require.ElementsMatch(t, []string{"api_key", "account_id"}, (&ApiKeyPlacement{
		Type:          ApiKeyPlacementBasic,
		UsernameField: "account_id",
	}).CredentialFieldNames())

	// nil receiver returns nil — defensive guard
	var p *ApiKeyPlacement
	require.Nil(t, p.CredentialFieldNames())
}

func TestConnectorNormalize_SynthesizesApiKeyPreconnectWhenMissing(t *testing.T) {
	c := &Connector{
		Auth: &Auth{InnerVal: &AuthApiKey{
			Type:      AuthTypeAPIKey,
			Placement: &ApiKeyPlacement{Type: ApiKeyPlacementBearer},
		}},
	}
	c.Normalize()

	require.NotNil(t, c.SetupFlow)
	require.True(t, c.SetupFlow.HasPreconnect())
	require.Len(t, c.SetupFlow.Preconnect.Steps, 1)
	require.Equal(t, SynthesizedApiKeyPreconnectStepId, c.SetupFlow.Preconnect.Steps[0].Id)
}

func TestConnectorNormalize_LeavesExplicitPreconnectAlone(t *testing.T) {
	explicit := SetupFlowStep{
		Id:         "custom-step",
		Title:      "Custom",
		JsonSchema: common.RawJSON(`{"type":"object","properties":{"api_key":{"type":"string"}},"required":["api_key"],"additionalProperties":false}`),
	}
	c := &Connector{
		Auth: &Auth{InnerVal: &AuthApiKey{
			Type:      AuthTypeAPIKey,
			Placement: &ApiKeyPlacement{Type: ApiKeyPlacementBearer},
		}},
		SetupFlow: &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{explicit}},
		},
	}
	c.Normalize()

	require.Len(t, c.SetupFlow.Preconnect.Steps, 1)
	require.Equal(t, "custom-step", c.SetupFlow.Preconnect.Steps[0].Id)
}

func TestConnectorNormalize_Idempotent(t *testing.T) {
	c := &Connector{
		Auth: &Auth{InnerVal: &AuthApiKey{
			Type:      AuthTypeAPIKey,
			Placement: &ApiKeyPlacement{Type: ApiKeyPlacementBearer},
		}},
	}
	c.Normalize()
	first := c.SetupFlow.Preconnect.Steps[0].Id
	c.Normalize()
	require.Len(t, c.SetupFlow.Preconnect.Steps, 1)
	require.Equal(t, first, c.SetupFlow.Preconnect.Steps[0].Id)
}

func TestConnectorNormalize_NoOpForOAuth2(t *testing.T) {
	c := &Connector{
		Auth: &Auth{InnerVal: &AuthOAuth2{Type: AuthTypeOAuth2}},
	}
	c.Normalize()
	// OAuth2 connectors should not have a preconnect synthesized.
	require.False(t, c.SetupFlow != nil && c.SetupFlow.HasPreconnect())
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

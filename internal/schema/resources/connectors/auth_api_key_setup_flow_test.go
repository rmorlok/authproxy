package connectors

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

func TestSynthesizeApiKeyCredentialsStep_Bearer(t *testing.T) {
	step := SynthesizeApiKeyCredentialsStep(&ApiKeyPlacement{Type: ApiKeyPlacementBearer})
	require.NotNil(t, step)
	require.Equal(t, SynthesizedApiKeyCredentialsStepId, step.Id)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(step.JsonSchema, &schema))
	props := schema["properties"].(map[string]any)
	require.Contains(t, props, "api_key")
	require.NotContains(t, props, "username") // No username for bearer

	required := toStringSlice(schema["required"])
	require.ElementsMatch(t, []string{"api_key"}, required)

	require.False(t, schema["additionalProperties"].(bool))
}

func TestSynthesizeApiKeyCredentialsStep_Header(t *testing.T) {
	// header placement renders the same form as bearer at this layer — the
	// header_name + prefix only affect the proxy, not the user-input form.
	step := SynthesizeApiKeyCredentialsStep(&ApiKeyPlacement{
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

func TestSynthesizeApiKeyCredentialsStep_Query(t *testing.T) {
	step := SynthesizeApiKeyCredentialsStep(&ApiKeyPlacement{
		Type:      ApiKeyPlacementQuery,
		ParamName: "api_key",
	})
	require.NotNil(t, step)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(step.JsonSchema, &schema))
	props := schema["properties"].(map[string]any)
	require.Contains(t, props, "api_key")
}

func TestSynthesizeApiKeyCredentialsStep_Basic(t *testing.T) {
	step := SynthesizeApiKeyCredentialsStep(&ApiKeyPlacement{
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

func TestSynthesizeApiKeyCredentialsStep_Nil(t *testing.T) {
	require.Nil(t, SynthesizeApiKeyCredentialsStep(nil))
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

func TestConnectorNormalize_SynthesizesApiKeyCredentialsWhenMissing(t *testing.T) {
	c := &Connector{
		Auth: &Auth{InnerVal: &AuthApiKey{
			Type:      AuthTypeAPIKey,
			Placement: &ApiKeyPlacement{Type: ApiKeyPlacementBearer},
		}},
	}
	c.Normalize()

	require.NotNil(t, c.SetupFlow)
	require.True(t, c.SetupFlow.HasCredentials())
	require.Len(t, c.SetupFlow.Credentials.Steps, 1)
	require.Equal(t, SynthesizedApiKeyCredentialsStepId, c.SetupFlow.Credentials.Steps[0].Id)
}

func TestConnectorNormalize_LeavesExplicitCredentialsAlone(t *testing.T) {
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
			Credentials: &SetupFlowPhase{Steps: []SetupFlowStep{explicit}},
		},
	}
	c.Normalize()

	require.Len(t, c.SetupFlow.Credentials.Steps, 1)
	require.Equal(t, "custom-step", c.SetupFlow.Credentials.Steps[0].Id)
}

func TestConnectorNormalize_LeavesPreconnectAlone(t *testing.T) {
	// A connector author's preconnect step (collecting non-credential prerequisites
	// like a tenant) must not be touched by Normalize — credentials phase is
	// independent of preconnect.
	preconnect := SetupFlowStep{
		Id:         "tenant",
		JsonSchema: common.RawJSON(`{"type":"object","properties":{"tenant":{"type":"string"}},"required":["tenant"],"additionalProperties":false}`),
	}
	c := &Connector{
		Auth: &Auth{InnerVal: &AuthApiKey{
			Type:      AuthTypeAPIKey,
			Placement: &ApiKeyPlacement{Type: ApiKeyPlacementBearer},
		}},
		SetupFlow: &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{preconnect}},
		},
	}
	c.Normalize()

	// Preconnect is unchanged...
	require.Len(t, c.SetupFlow.Preconnect.Steps, 1)
	require.Equal(t, "tenant", c.SetupFlow.Preconnect.Steps[0].Id)
	// ...AND a credentials step is synthesized alongside it.
	require.True(t, c.SetupFlow.HasCredentials())
	require.Equal(t, SynthesizedApiKeyCredentialsStepId, c.SetupFlow.Credentials.Steps[0].Id)
}

func TestConnectorNormalize_Idempotent(t *testing.T) {
	c := &Connector{
		Auth: &Auth{InnerVal: &AuthApiKey{
			Type:      AuthTypeAPIKey,
			Placement: &ApiKeyPlacement{Type: ApiKeyPlacementBearer},
		}},
	}
	c.Normalize()
	first := c.SetupFlow.Credentials.Steps[0].Id
	c.Normalize()
	require.Len(t, c.SetupFlow.Credentials.Steps, 1)
	require.Equal(t, first, c.SetupFlow.Credentials.Steps[0].Id)
}

func TestConnectorNormalize_NoOpForOAuth2(t *testing.T) {
	c := &Connector{
		Auth: &Auth{InnerVal: &AuthOAuth2{Type: AuthTypeOAuth2}},
	}
	c.Normalize()
	// OAuth2 connectors should not have a credentials phase synthesized — they
	// use the auth phase (redirect) instead.
	require.False(t, c.SetupFlow != nil && c.SetupFlow.HasCredentials())
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

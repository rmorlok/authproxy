package apserde

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type apiSecretPayload struct {
	Public string `json:"public" yaml:"public"`
	Secret string `json:"secret,omitempty" yaml:"secret,omitempty" apiredact:"secret"`
	Empty  string `json:"empty,omitempty" yaml:"empty,omitempty" apiredact:"secret"`
}

type apiNestedPayload struct {
	Items []apiSecretPayload          `json:"items" yaml:"items"`
	Map   map[string]apiSecretPayload `json:"map" yaml:"map"`
}

type apiInnerSecret struct {
	Type   string `json:"type" yaml:"type"`
	Secret string `json:"client_secret" yaml:"client_secret" apiredact:"secret"`
}

type apiWrapper struct {
	InnerVal any `json:"-" yaml:"-"`
}

func (w apiWrapper) MarshalJSON() ([]byte, error) {
	return json.Marshal(w.InnerVal)
}

func (w apiWrapper) MarshalYAML() (any, error) {
	return w.InnerVal, nil
}

func TestMarshalJSONForAPI_RedactsSecretFields(t *testing.T) {
	payload := apiNestedPayload{
		Items: []apiSecretPayload{{Public: "shown", Secret: "secret"}},
		Map:   map[string]apiSecretPayload{"one": {Public: "visible", Secret: "sëcret"}},
	}

	data, report, err := MarshalJSONForAPI(context.Background(), payload)
	require.NoError(t, err)
	require.True(t, report.Redacted)
	require.JSONEq(t, `{
		"items": [{"public": "shown", "secret": "******"}],
		"map": {"one": {"public": "visible", "secret": "******"}}
	}`, string(data))

	require.Equal(t, "secret", payload.Items[0].Secret)
	require.Equal(t, "sëcret", payload.Map["one"].Secret)
}

func TestStandardJSONMarshalIgnoresRedactionTags(t *testing.T) {
	data, err := json.Marshal(apiSecretPayload{Public: "shown", Secret: "secret"})
	require.NoError(t, err)
	require.JSONEq(t, `{"public":"shown","secret":"secret"}`, string(data))
}

func TestMarshalJSONForAPI_ReplayLeavesSecretsUnchanged(t *testing.T) {
	ctx := WithSecretReplay(context.Background(), true)
	payload := apiSecretPayload{Public: "shown", Secret: "secret"}

	data, report, err := MarshalJSONForAPI(ctx, payload)
	require.NoError(t, err)
	require.False(t, report.Redacted)
	require.JSONEq(t, `{"public":"shown","secret":"secret"}`, string(data))
}

func TestMarshalJSONForAPI_RedactsStringValueShapes(t *testing.T) {
	type payload struct {
		Inline *common.StringValue `json:"inline,omitempty" yaml:"inline,omitempty" apiredact:"secret"`
		Object *common.StringValue `json:"object,omitempty" yaml:"object,omitempty" apiredact:"secret"`
	}

	data, report, err := MarshalJSONForAPI(context.Background(), payload{
		Inline: common.NewStringValueDirectInline("token"),
		Object: common.NewStringValueDirect("secret"),
	})
	require.NoError(t, err)
	require.True(t, report.Redacted)
	require.JSONEq(t, `{
		"inline": "*****",
		"object": {"value": "******"}
	}`, string(data))
}

func TestMarshalJSONForAPI_UnwrapsPolymorphicStructsWithRedaction(t *testing.T) {
	type payload struct {
		Auth apiWrapper `json:"auth" yaml:"auth"`
	}

	data, report, err := MarshalJSONForAPI(context.Background(), payload{
		Auth: apiWrapper{InnerVal: apiInnerSecret{Type: "OAuth2", Secret: "client-secret"}},
	})
	require.NoError(t, err)
	require.True(t, report.Redacted)
	require.JSONEq(t, `{"auth":{"type":"OAuth2","client_secret":"*************"}}`, string(data))
}

func TestMarshalYAMLForAPI_RedactsSecretFields(t *testing.T) {
	data, report, err := MarshalYAMLForAPI(context.Background(), apiSecretPayload{
		Public: "shown",
		Secret: "secret",
	})
	require.NoError(t, err)
	require.True(t, report.Redacted)

	var decoded map[string]string
	require.NoError(t, yaml.Unmarshal(data, &decoded))
	require.Equal(t, map[string]string{
		"public": "shown",
		"secret": "******",
	}, decoded)
}

func TestValidateNoRedactedPlaceholders(t *testing.T) {
	type payload struct {
		Secret *common.StringValue `json:"secret,omitempty" yaml:"secret,omitempty" apiredact:"secret"`
		Public string              `json:"public" yaml:"public"`
	}

	require.NoError(t, ValidateNoRedactedPlaceholders(payload{
		Secret: common.NewStringValueDirectInline("real-secret"),
		Public: "***",
	}))

	err := ValidateNoRedactedPlaceholders(payload{
		Secret: common.NewStringValueDirectInline("***"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.secret")
}

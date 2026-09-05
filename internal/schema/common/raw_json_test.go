package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRawJSONYAMLRoundTrip(t *testing.T) {
	original := RawJSON(`{"type":"object","properties":{"tenant":{"type":"string"}}}`)

	encoded, err := yaml.Marshal(original)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "type: object")
	require.Contains(t, string(encoded), "tenant:")

	var decoded RawJSON
	require.NoError(t, yaml.Unmarshal(encoded, &decoded))
	require.JSONEq(t, string(original), string(decoded))
}

func TestRawJSONMarshalYAMLRejectsInvalidJSON(t *testing.T) {
	_, err := yaml.Marshal(RawJSON(`{invalid`))
	require.ErrorContains(t, err, "invalid raw JSON")
}

func TestRawJSONNilMarshalYAML(t *testing.T) {
	encoded, err := yaml.Marshal(RawJSON(nil))
	require.NoError(t, err)
	require.Equal(t, "null\n", string(encoded))

	encodedJSON, err := json.Marshal(RawJSON(nil))
	require.NoError(t, err)
	require.JSONEq(t, "null", string(encodedJSON))
}

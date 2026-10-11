package toolsets

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestOpenAPISecurityConfigValidation checks exact authored mappings without
// looking up provider schemes or claiming the connection satisfies them.
func TestOpenAPISecurityConfigValidation(t *testing.T) {
	for _, config := range []*OpenAPISecurityConfig{
		nil, {}, {ConnectionSchemes: []string{}},
		{ConnectionSchemes: []string{"OAuth", "oauth", " padded identifier ", "./oauth", "https://example.test/security#/Bearer"}},
	} {
		before := config.Clone()
		require.NoError(t, config.Validate(nil))
		require.Equal(t, before, config, "validation preserves exact identifiers and order")
	}
	for _, test := range []struct {
		values []string
		path   string
	}{
		{[]string{""}, "connectionSchemes[0]"},
		{[]string{"OAuth", " \t\n"}, "connectionSchemes[1]"},
		{[]string{"OAuth", "OAuth"}, "connectionSchemes[1]"},
	} {
		config := &OpenAPISecurityConfig{ConnectionSchemes: test.values}
		require.ErrorContains(t, config.Validate(&common.ValidationContext{Path: "source.openapi.security"}), "source.openapi.security."+test.path)
	}
}

// TestOpenAPISecurityConfigRoundTrip preserves scheme names and URI references
// as written, including case, fragments, and identifiers resembling templates.
func TestOpenAPISecurityConfigRoundTrip(t *testing.T) {
	original := &OpenAPISecurityConfig{ConnectionSchemes: []string{
		"Bearer", "bearer", "./security.yaml#/Bearer", "https://EXAMPLE.test/security#/Bearer", "{{literal-name}}",
	}}
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{{"JSON", json.Marshal, util.DecodeJSONStrict}, {"YAML", yaml.Marshal, util.DecodeYAMLStrict}} {
		t.Run(format.name, func(t *testing.T) {
			encoded, err := format.encode(original)
			require.NoError(t, err)
			var decoded OpenAPISecurityConfig
			require.NoError(t, format.decode(encoded, &decoded))
			require.NoError(t, decoded.Validate(nil))
			require.Equal(t, original, &decoded)
			for _, input := range []string{`{}`, `{"connectionSchemes":[]}`} {
				require.NoError(t, format.decode([]byte(input), &decoded))
				require.Empty(t, decoded.ConnectionSchemes)
				require.NoError(t, decoded.Validate(nil))
			}
		})
	}
}

// TestOpenAPISecurityConfigStrictDecode rejects nulls and unexpected controls
// atomically instead of silently clearing an existing mapping on malformed input.
func TestOpenAPISecurityConfigStrictDecode(t *testing.T) {
	for _, format := range []struct {
		name   string
		decode func([]byte, any) error
	}{{"JSON", util.DecodeJSONStrict}, {"YAML", util.DecodeYAMLStrict}} {
		t.Run(format.name, func(t *testing.T) {
			for _, input := range []string{
				`[]`, `true`, `{"unknown":true}`, `{"ConnectionSchemes":["Bearer"]}`,
				`{"connectionSchemes":null}`, `{"connectionSchemes":"Bearer"}`, `{"connectionSchemes":{}}`,
				`{"connectionSchemes":["Bearer",null]}`, `{"connectionSchemes":[true]}`, `{"connectionSchemes":[7]}`,
				`{"connectionSchemes":["Bearer"],"connectionRef":{}}`,
				`{"connectionSchemes":["Bearer"],"scopes":["read"]}`,
			} {
				config := &OpenAPISecurityConfig{ConnectionSchemes: []string{"original"}}
				before := config.Clone()
				require.Error(t, format.decode([]byte(input), config), input)
				require.Equal(t, before, config)
			}
			var source OpenAPISource
			require.Error(t, format.decode([]byte(`{"document":{"inline":{}},"security":null}`), &source))
		})
	}
	var config OpenAPISecurityConfig
	require.Error(t, util.DecodeJSONStrict([]byte(`null`), &config))
	// yaml.v3 bypasses custom decoding for a root null. The nested source field
	// above is the authored boundary and must reject null in both formats.
	require.NoError(t, util.DecodeYAMLStrict([]byte(`null`), &config))
	require.NoError(t, config.Validate(nil))
}

// TestOpenAPISecurityConfigYAMLComposition resolves merged and aliased values
// before validating presence, so neither mechanism can hide a null mapping.
func TestOpenAPISecurityConfigYAMLComposition(t *testing.T) {
	var config OpenAPISecurityConfig
	require.NoError(t, util.DecodeYAMLStrict([]byte("<<: {connectionSchemes: &schemes [Bearer, OAuth]}\nconnectionSchemes: *schemes"), &config))
	require.Equal(t, []string{"Bearer", "OAuth"}, config.ConnectionSchemes)
	require.NoError(t, config.Validate(nil))
	for _, input := range []string{
		"<<: {connectionSchemes: null}",
		"<<: {connectionSchemes: &nothing null}\nconnectionSchemes: *nothing",
		"connectionSchemes: [&nothing null, *nothing]",
	} {
		before := config.Clone()
		require.Error(t, util.DecodeYAMLStrict([]byte(input), &config), input)
		require.Equal(t, before, &config)
	}
}

// TestOpenAPISecurityConfigCheckedFields validates the final JSON field value
// rather than merging an earlier mapping or accepting a final hidden null.
func TestOpenAPISecurityConfigCheckedFields(t *testing.T) {
	var config OpenAPISecurityConfig
	require.NoError(t, util.DecodeJSONStrict([]byte(`{"connectionSchemes":[null],"connectionSchemes":["Bearer"]}`), &config))
	require.Equal(t, []string{"Bearer"}, config.ConnectionSchemes)
	before := config.Clone()
	require.Error(t, util.DecodeJSONStrict([]byte(`{"connectionSchemes":["OAuth"],"connectionSchemes":null}`), &config))
	require.Equal(t, before, &config)
}

// TestOpenAPISecurityConfigClone detaches the list while preserving omitted,
// empty, and invalid values needed for later diagnostics.
func TestOpenAPISecurityConfigClone(t *testing.T) {
	require.Nil(t, (*OpenAPISecurityConfig)(nil).Clone())
	for _, schemes := range [][]string{nil, {}, {"", "Bearer", "Bearer"}} {
		original := &OpenAPISecurityConfig{ConnectionSchemes: schemes}
		clone := original.Clone()
		require.NotSame(t, original, clone)
		require.Equal(t, original, clone)
		if len(schemes) > 0 {
			clone.ConnectionSchemes[0] = "changed"
			require.Equal(t, "", original.ConnectionSchemes[0])
		}
	}
}

package toolsets

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestOpenAPIServerConfigValidation separates authored selection rules from
// later document lookup, variable enumeration, and URL template compilation.
func TestOpenAPIServerConfigValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		server *OpenAPIServerConfig
		valid  bool
		field  string
	}{
		{"omitted", nil, true, ""},
		{"URL", &OpenAPIServerConfig{URL: util.ToPtr("https://example.test/v1")}, true, ""},
		{"whole URL template", &OpenAPIServerConfig{URL: util.ToPtr("{{cfg.serverUrl}}")}, true, ""},
		{"URL variable template", &OpenAPIServerConfig{URL: util.ToPtr("https://{{cfg.apiHost}}/v1")}, true, ""},
		{"first index", &OpenAPIServerConfig{Index: util.ToPtr(0)}, true, ""},
		{"no document bounds check", &OpenAPIServerConfig{Index: util.ToPtr(1000000)}, true, ""},
		{"empty variables", &OpenAPIServerConfig{Index: util.ToPtr(0), Variables: util.ToPtr(map[string]string{})}, true, ""},
		{"literal variables", &OpenAPIServerConfig{Index: util.ToPtr(1), Variables: util.ToPtr(map[string]string{" version ": "v1", "empty": "", "spaces": " \t"})}, true, ""},
		{"template-looking literal", &OpenAPIServerConfig{Index: util.ToPtr(0), Variables: util.ToPtr(map[string]string{"host": "{{cfg.host}}", "opening": "v{{1", "closing": "v1}}"})}, true, ""},
		{"missing selection", &OpenAPIServerConfig{}, false, ""},
		{"two selections", &OpenAPIServerConfig{URL: util.ToPtr("https://example.test"), Index: util.ToPtr(0)}, false, ""},
		{"negative index", &OpenAPIServerConfig{Index: util.ToPtr(-1)}, false, "index"},
		{"variables without selection", &OpenAPIServerConfig{Variables: util.ToPtr(map[string]string{})}, false, ""},
		{"URL with variables", &OpenAPIServerConfig{URL: util.ToPtr("https://example.test"), Variables: util.ToPtr(map[string]string{})}, false, "variables"},
		{"nil variables map", &OpenAPIServerConfig{Index: util.ToPtr(0), Variables: util.ToPtr(map[string]string(nil))}, false, "variables"},
		{"empty URL", &OpenAPIServerConfig{URL: util.ToPtr("")}, false, "url"},
		{"blank URL", &OpenAPIServerConfig{URL: util.ToPtr(" \t")}, false, "url"},
		{"padded URL", &OpenAPIServerConfig{URL: util.ToPtr(" https://example.test ")}, false, "url"},
		{"URL line feed", &OpenAPIServerConfig{URL: util.ToPtr("https://example.test/\nv1")}, false, "url"},
		{"URL carriage return", &OpenAPIServerConfig{URL: util.ToPtr("https://example.test/\rv1")}, false, "url"},
		{"blank variable name", &OpenAPIServerConfig{Index: util.ToPtr(0), Variables: util.ToPtr(map[string]string{" \t": "v1"})}, false, "variables"},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := test.server.Clone()
			err := test.server.Validate(&common.ValidationContext{Path: "source.openapi.server"})
			if test.valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "source.openapi.server")
				if test.field != "" {
					require.ErrorContains(t, err, test.field)
				}
			}
			require.Equal(t, before, test.server, "validation preserves exact authored values")
		})
	}
}

// TestOpenAPIServerConfigRoundTrip protects zero-index and map presence in both
// formats, including invalid unions that export must not silently repair.
func TestOpenAPIServerConfigRoundTrip(t *testing.T) {
	for _, test := range []struct {
		name   string
		server *OpenAPIServerConfig
		wire   string
		valid  bool
	}{
		{"URL", &OpenAPIServerConfig{URL: util.ToPtr("{{cfg.serverUrl}}")}, `{"url":"{{cfg.serverUrl}}"}`, true},
		{"index zero", &OpenAPIServerConfig{Index: util.ToPtr(0)}, `{"index":0}`, true},
		{"empty variables", &OpenAPIServerConfig{Index: util.ToPtr(0), Variables: util.ToPtr(map[string]string{})}, `{"index":0,"variables":{}}`, true},
		{"exact variables", &OpenAPIServerConfig{Index: util.ToPtr(2), Variables: util.ToPtr(map[string]string{" version ": "v1", "empty": "", "spaces": " \t"})}, `{"index":2,"variables":{" version ":"v1","empty":"","spaces":" \t"}}`, true},
		{"template-looking literals", &OpenAPIServerConfig{Index: util.ToPtr(0), Variables: util.ToPtr(map[string]string{"host": "{{cfg.host}}", "opening": "v{{1", "closing": "v1}}"})}, `{"index":0,"variables":{"host":"{{cfg.host}}","opening":"v{{1","closing":"v1}}"}}`, true},
		{"empty selection", &OpenAPIServerConfig{}, `{}`, false},
		{"both selections", &OpenAPIServerConfig{URL: util.ToPtr("https://example.test"), Index: util.ToPtr(0)}, `{"url":"https://example.test","index":0}`, false},
		{"URL and empty variables", &OpenAPIServerConfig{URL: util.ToPtr("https://example.test"), Variables: util.ToPtr(map[string]string{})}, `{"url":"https://example.test","variables":{}}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, format := range []struct {
				name   string
				encode func(any) ([]byte, error)
				decode func([]byte, any) error
			}{{"JSON", json.Marshal, util.DecodeJSONStrict}, {"YAML", yaml.Marshal, util.DecodeYAMLStrict}} {
				t.Run(format.name, func(t *testing.T) {
					encoded, err := format.encode(test.server)
					require.NoError(t, err)
					var raw common.RawJSON
					require.NoError(t, format.decode(encoded, &raw))
					require.JSONEq(t, test.wire, string(raw))
					var decoded OpenAPIServerConfig
					require.NoError(t, format.decode(encoded, &decoded))
					require.Equal(t, test.server, &decoded)
					if test.valid {
						require.NoError(t, decoded.Validate(nil))
					} else {
						require.Error(t, decoded.Validate(nil))
					}
				})
			}
		})
	}
}

// TestOpenAPIServerConfigStrictDecoding rejects lossy null decoding and owned
// field misspellings while leaving selection completeness to validation.
func TestOpenAPIServerConfigStrictDecoding(t *testing.T) {
	for _, input := range []string{
		`[]`, `false`, `{"unknown":true}`, `{"URL":"https://example.test"}`, `{"Index":0}`, `{"Variables":{}}`,
		`{"url":null}`, `{"index":null}`, `{"variables":null}`,
		`{"url":4}`, `{"url":false}`, `{"index":"0"}`, `{"index":0.5}`, `{"index":false}`,
		`{"variables":[]}`, `{"variables":{"version":null}}`, `{"variables":{"version":1}}`,
		`{"variables":{"version":true}}`, `{"variables":{"version":{}}}`,
	} {
		for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
			var server OpenAPIServerConfig
			require.Error(t, decode([]byte(input), &server), input)
		}
	}
	var server OpenAPIServerConfig
	require.Error(t, util.DecodeJSONStrict([]byte(`null`), &server))
	require.Error(t, util.DecodeJSONStrict([]byte(`{} {}`), &server))
	// yaml.v3 skips custom unmarshalling for root null; the fresh zero value
	// still fails required-selection validation.
	require.NoError(t, util.DecodeYAMLStrict([]byte(`null`), &server))
	require.Error(t, server.Validate(nil))
}

// TestOpenAPIServerConfigYAMLComposition resolves aliases and merges before
// checking raw values, so indirect nulls cannot become empty strings or index 0.
func TestOpenAPIServerConfigYAMLComposition(t *testing.T) {
	var server OpenAPIServerConfig
	require.NoError(t, util.DecodeYAMLStrict([]byte(`
<<: {index: 0}
variables:
  <<: {version: &version v1}
  region: *version
`), &server))
	require.NoError(t, server.Validate(nil))
	require.Equal(t, 0, *server.Index)
	require.Equal(t, map[string]string{"version": "v1", "region": "v1"}, *server.Variables)
	for _, input := range []string{
		"<<: {index: null}",
		"<<: {url: null}",
		"index: 0\n<<: {variables: null}",
		"index: 0\nvariables: {version: &none null, region: *none}",
		"index: 0\nvariables:\n  <<: {version: null}",
		"<<: {Index: 0}",
	} {
		before := server.Clone()
		require.Error(t, util.DecodeYAMLStrict([]byte(input), &server), input)
		require.Equal(t, before, &server, "failed YAML decoding is atomic")
	}
}

// TestOpenAPIServerConfigCloneAndDecodeOwnership checks detached mutable data,
// invalid shape retention, and replacement rather than incremental decoding.
func TestOpenAPIServerConfigCloneAndDecodeOwnership(t *testing.T) {
	require.Nil(t, (*OpenAPIServerConfig)(nil).Clone())
	for _, variables := range []*map[string]string{nil, util.ToPtr(map[string]string(nil)), util.ToPtr(map[string]string{}), util.ToPtr(map[string]string{"version": "v1"})} {
		original := &OpenAPIServerConfig{URL: util.ToPtr("https://example.test"), Index: util.ToPtr(0), Variables: variables}
		clone := original.Clone()
		require.Equal(t, original, clone, "clone retains invalid unions and map presence")
		*clone.URL = "changed"
		*clone.Index = 7
		require.Equal(t, "https://example.test", *original.URL)
		require.Equal(t, 0, *original.Index)
		if clone.Variables != nil {
			if *clone.Variables != nil {
				(*clone.Variables)["version"] = "changed"
				require.NotEqual(t, "changed", (*original.Variables)["version"])
			}
			*clone.Variables = map[string]string{"replacement": "value"}
			require.NotEqual(t, *original.Variables, *clone.Variables, "map pointers must also be detached")
		}
	}
	for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
		server := &OpenAPIServerConfig{Index: util.ToPtr(1), Variables: util.ToPtr(map[string]string{"version": "v1"})}
		before := server.Clone()
		require.Error(t, decode([]byte(`{"index":0,"variables":{"version":null}}`), server))
		require.Equal(t, before, server, "failed decoding preserves the receiver")
		require.NoError(t, decode([]byte(`{"url":"https://example.test"}`), server))
		require.Equal(t, &OpenAPIServerConfig{URL: util.ToPtr("https://example.test")}, server)
		require.NoError(t, decode([]byte(`{"index":0}`), server))
		require.Equal(t, &OpenAPIServerConfig{Index: util.ToPtr(0)}, server)
		require.NoError(t, decode([]byte(`{}`), server))
		require.Equal(t, &OpenAPIServerConfig{}, server)
		require.Error(t, server.Validate(nil))
	}
}

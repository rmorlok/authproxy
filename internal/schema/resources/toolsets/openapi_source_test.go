package toolsets

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// openAPISourceForTest supplies every source setting without parsing or fetching
// the referenced document, keeping these tests focused on authored contracts.
func openAPISourceForTest() *OpenAPISource {
	url := "https://example.test/openapi.json"
	return &OpenAPISource{
		Document: &OpenAPIDocument{URL: &url},
		Operations: &OpenAPIOperationFilter{
			IncludeOperationIDs: &[]string{"listCalendars", "createEvent"},
			ExcludeOperationIDs: &[]string{"createEvent"},
		},
		Server: &OpenAPIServerConfig{URL: util.ToPtr("https://{{cfg.apiHost}}/v1")},
	}
}

// TestOpenAPISourceValidation checks complete authored settings and nested
// diagnostics, keeping document discovery and server compilation out of scope.
func TestOpenAPISourceValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*OpenAPISource)
		field  string
	}{
		{"valid", func(s *OpenAPISource) {}, ""},
		{"optional omitted", func(s *OpenAPISource) { s.Operations = nil; s.Server = nil }, ""},
		{"whole URL template", func(s *OpenAPISource) { *s.Server.URL = "{{cfg.serverUrl}}" }, ""},
		{"document server selection", func(s *OpenAPISource) { s.Server = &OpenAPIServerConfig{Index: util.ToPtr(0)} }, ""},
		{"negative server index", func(s *OpenAPISource) { s.Server = &OpenAPIServerConfig{Index: util.ToPtr(-1)} }, "server.index"},
		{"missing document", func(s *OpenAPISource) { s.Document = nil }, "document"},
		{"invalid document", func(s *OpenAPISource) { s.Document = &OpenAPIDocument{} }, "document"},
		{"empty inclusion", func(s *OpenAPISource) { s.Operations.IncludeOperationIDs = &[]string{} }, "operations.includeOperationIds"},
		{"blank ID", func(s *OpenAPISource) { s.Operations.IncludeOperationIDs = &[]string{" \t"} }, "operations.includeOperationIds[0]"},
		{"duplicate include", func(s *OpenAPISource) { s.Operations.IncludeOperationIDs = &[]string{"same", "same"} }, "operations.includeOperationIds[1]"},
		{"duplicate exclude", func(s *OpenAPISource) { s.Operations.ExcludeOperationIDs = &[]string{"same", "same"} }, "operations.excludeOperationIds[1]"},
		{"empty server URL", func(s *OpenAPISource) { *s.Server.URL = "" }, "server.url"},
		{"blank server URL", func(s *OpenAPISource) { *s.Server.URL = " \t" }, "server.url"},
		{"padded URL", func(s *OpenAPISource) { *s.Server.URL = " https://example.test " }, "server.url"},
		{"line feed", func(s *OpenAPISource) { *s.Server.URL = "https://example.test/\nv1" }, "server.url"},
		{"carriage return", func(s *OpenAPISource) { *s.Server.URL = "https://example.test/\rv1" }, "server.url"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := openAPISourceForTest()
			test.change(source)
			err := source.Validate(&common.ValidationContext{Path: "source.openapi"})
			if test.field == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "source.openapi."+test.field)
			}
		})
	}
	require.Error(t, (*OpenAPISource)(nil).Validate(nil))
	require.NoError(t, (*OpenAPIOperationFilter)(nil).Validate(nil))
	require.NoError(t, (*OpenAPIServerConfig)(nil).Validate(nil))
	filter := &OpenAPIOperationFilter{
		IncludeOperationIDs: &[]string{"read", "Read", " read "},
		ExcludeOperationIDs: &[]string{"read"},
	}
	require.NoError(t, filter.Validate(nil), "IDs remain exact; exclusion wins intentional overlap")
	require.Equal(t, " read ", (*filter.IncludeOperationIDs)[2])
}

// TestOpenAPISourceRoundTrip preserves templates and explicit empty filter
// lists, including an invalid inclusion that must not become unrestricted.
func TestOpenAPISourceRoundTrip(t *testing.T) {
	for _, format := range []struct {
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{{json.Marshal, util.DecodeJSONStrict}, {yaml.Marshal, util.DecodeYAMLStrict}} {
		source := openAPISourceForTest()
		encoded, err := format.encode(source)
		require.NoError(t, err)
		var decoded OpenAPISource
		require.NoError(t, format.decode(encoded, &decoded))
		require.NoError(t, decoded.Validate(nil))
		require.Equal(t, source, &decoded)
		for _, filter := range []*OpenAPIOperationFilter{{}, {ExcludeOperationIDs: &[]string{}}, {IncludeOperationIDs: &[]string{}}} {
			encoded, err := format.encode(filter)
			require.NoError(t, err)
			var decoded OpenAPIOperationFilter
			require.NoError(t, format.decode(encoded, &decoded))
			require.Equal(t, filter, &decoded)
			if filter.IncludeOperationIDs != nil {
				require.ErrorContains(t, decoded.Validate(nil), "includeOperationIds")
			} else {
				require.NoError(t, decoded.Validate(nil))
			}
		}
	}
}

// TestOpenAPISourceStrictDecoding rejects nulls and unsupported configuration
// fields rather than silently accepting deferred importer capabilities.
func TestOpenAPISourceStrictDecoding(t *testing.T) {
	for _, input := range []string{
		`{"Document":{}}`, `{"document":null}`, `{"operations":null}`, `{"server":null}`,
		`{"operations":{"includeOperationIds":null}}`, `{"operations":{"excludeOperationIds":null}}`,
		`{"operations":{"includeOperationIds":[null]}}`, `{"operations":{"excludeOperationIds":[true]}}`,
		`{"operations":{"includeOperationIds":"read"}}`, `{"operations":{"includeOperationIDs":["read"]}}`,
		`{"operations":{"includeMethods":["GET"]}}`,
		`{"server":{"url":null}}`, `{"server":{"URL":"https://example.test"}}`, `{"server":{"url":5}}`,
		`{"server":{"index":null}}`, `{"server":{"variables":{"version":null}}}`,
		`{"security":{}}`, `{"references":[]}`, `{"skipUnsupported":true}`, `[]`,
	} {
		for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
			var source OpenAPISource
			require.Error(t, decode([]byte(input), &source), input)
		}
	}
	var source OpenAPISource
	require.Error(t, util.DecodeJSONStrict([]byte(`null`), &source))
	require.Error(t, util.DecodeJSONStrict([]byte(`{} {}`), &source))
	// yaml.v3 bypasses custom unmarshalling at root null; a fresh decoded value
	// remains invalid because the required document was never supplied.
	var nullSource OpenAPISource
	require.NoError(t, util.DecodeYAMLStrict([]byte(`null`), &nullSource))
	require.ErrorContains(t, nullSource.Validate(nil), "document")
}

// TestOpenAPISourceYAMLComposition resolves valid aliases and merges and keeps
// indirect nulls visible to the strict nested source/operation/server decoders.
func TestOpenAPISourceYAMLComposition(t *testing.T) {
	var source OpenAPISource
	input := `
document: {url: https://example.test/openapi.json}
operations:
  <<: {includeOperationIds: [&name read, create]}
  excludeOperationIds: [*name]
server:
  <<: {url: "https://{{cfg.apiHost}}/v1"}
`
	require.NoError(t, util.DecodeYAMLStrict([]byte(input), &source))
	require.NoError(t, source.Validate(nil))
	require.Equal(t, []string{"read"}, *source.Operations.ExcludeOperationIDs)
	for _, input := range []string{
		"<<: {document: null}",
		"operations: &none null\nserver: *none",
		"operations:\n  <<: {includeOperationIds: null}",
		"operations:\n  includeOperationIds: [&none null, *none]",
		"server:\n  <<: {url: null}",
	} {
		require.Error(t, util.DecodeYAMLStrict([]byte(input), &source), input)
	}
}

// TestOpenAPISourceCloneAndDecodeOwnership protects nested source settings from
// mutation through clones or partially decoded replacements.
func TestOpenAPISourceCloneAndDecodeOwnership(t *testing.T) {
	source := openAPISourceForTest()
	clone := source.Clone()
	require.Equal(t, source, clone)
	*clone.Document.URL = "https://changed.test/openapi.json"
	(*clone.Operations.IncludeOperationIDs)[0] = "changed"
	(*clone.Operations.ExcludeOperationIDs)[0] = "changed"
	*clone.Server.URL = "changed"
	require.Equal(t, openAPISourceForTest(), source)
	require.Nil(t, (*OpenAPISource)(nil).Clone())
	require.Nil(t, (*OpenAPIOperationFilter)(nil).Clone())
	require.Nil(t, (*OpenAPIServerConfig)(nil).Clone())
	require.Equal(t, &OpenAPISource{}, (&OpenAPISource{}).Clone())
	filter := (&OpenAPIOperationFilter{IncludeOperationIDs: &[]string{}}).Clone()
	require.NotNil(t, filter.IncludeOperationIDs)
	require.Nil(t, filter.ExcludeOperationIDs)

	require.Error(t, json.Unmarshal([]byte(`{"server":{"url":"changed"},"operations":{"includeOperationIds":[null]}}`), source))
	require.Equal(t, openAPISourceForTest(), source, "source decoding is atomic")
	require.Error(t, json.Unmarshal([]byte(`{"url":"changed","unknown":true}`), source.Server))
	require.Equal(t, openAPISourceForTest(), source, "server decoding is atomic")
	require.NoError(t, json.Unmarshal([]byte(`{"document":{"url":"https://example.test/openapi.json"}}`), source))
	require.Nil(t, source.Operations)
	require.Nil(t, source.Server)
}

// TestOpenAPIOperationFilterListOwnership preserves supplied list pointers and independently
// clones their slice headers, even for empty or invalid nil-slice values.
func TestOpenAPIOperationFilterListOwnership(t *testing.T) {
	for _, values := range [][]string{nil, {}, {"read"}} {
		filter := &OpenAPIOperationFilter{IncludeOperationIDs: util.ToPtr(values), ExcludeOperationIDs: util.ToPtr(values)}
		clone := filter.Clone()
		require.Equal(t, filter, clone)
		require.NotSame(t, filter.IncludeOperationIDs, clone.IncludeOperationIDs)
		require.NotSame(t, filter.ExcludeOperationIDs, clone.ExcludeOperationIDs)
		if values == nil {
			require.ErrorContains(t, filter.Validate(nil), "includeOperationIds: must not be null")
			require.ErrorContains(t, filter.Validate(nil), "excludeOperationIds: must not be null")
		}
		*clone.IncludeOperationIDs = append(*clone.IncludeOperationIDs, "added")
		*clone.ExcludeOperationIDs = append(*clone.ExcludeOperationIDs, "added")
		require.Equal(t, values, *filter.IncludeOperationIDs)
		require.Equal(t, values, *filter.ExcludeOperationIDs)
	}
}

// TestOpenAPIOperationFilterDecodeState checks atomic failure and replacement of previously
// supplied lists through both decoders, independently of source-level decoding.
func TestOpenAPIOperationFilterDecodeState(t *testing.T) {
	for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
		for _, input := range []string{
			`{"includeOperationIds":["changed"],"excludeOperationIds":[null]}`,
			`{"includeOperationIds":[null],"excludeOperationIds":["changed"]}`,
			`{"includeOperationIds":null}`, `{"excludeOperationIds":null}`,
			`{"unknown":[],"includeOperationIds":["changed"]}`,
		} {
			filter := openAPISourceForTest().Operations
			require.Error(t, decode([]byte(input), filter), input)
			require.Equal(t, openAPISourceForTest().Operations, filter, input)
		}
		filter := openAPISourceForTest().Operations
		require.NoError(t, decode([]byte(`{"excludeOperationIds":[]}`), filter))
		require.Equal(t, &OpenAPIOperationFilter{ExcludeOperationIDs: &[]string{}}, filter)
		require.NoError(t, decode([]byte(`{}`), filter))
		require.Equal(t, &OpenAPIOperationFilter{}, filter)
	}
}

package toolsets

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// mcpSourceContractForTest supplies a valid source with every optional field so
// tests can check omission, invalid replacements, and nested ownership separately.
func mcpSourceContractForTest() *MCPSource {
	interval := "1.5s"
	return &MCPSource{
		Endpoint: "https://{{cfg.apiHost}}/mcp", Transport: MCPTransportStreamableHTTP,
		RefreshInterval: &interval,
		Tools:           &MCPToolFilter{IncludeNames: []string{"list_calendars", "create_event"}, ExcludeNames: []string{"create_event"}},
	}
}

// TestMCPSourceValidation checks the contract boundary without parsing endpoint
// templates or normalizing accepted strings into a different authored value.
func TestMCPSourceValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*MCPSource)
		field  string
	}{
		{"valid", func(s *MCPSource) {}, ""},
		{"optional fields omitted", func(s *MCPSource) { s.RefreshInterval = nil; s.Tools = nil }, ""},
		{"whole endpoint template", func(s *MCPSource) { s.Endpoint = "{{cfg.mcpEndpoint}}" }, ""},
		{"empty endpoint", func(s *MCPSource) { s.Endpoint = "" }, "endpoint"},
		{"blank endpoint", func(s *MCPSource) { s.Endpoint = " \t" }, "endpoint"},
		{"surrounding whitespace", func(s *MCPSource) { s.Endpoint = " https://example.test/mcp" }, "endpoint"},
		{"newline", func(s *MCPSource) { s.Endpoint = "https://example.test/\nmcp" }, "endpoint"},
		{"missing transport", func(s *MCPSource) { s.Transport = "" }, "transport"},
		{"unsupported transport", func(s *MCPSource) { s.Transport = "stdio" }, "transport"},
		{"empty inclusion", func(s *MCPSource) { s.Tools.IncludeNames = []string{} }, "tools.includeNames"},
		{"blank name", func(s *MCPSource) { s.Tools.IncludeNames = []string{" \n"} }, "tools.includeNames[0]"},
		{"duplicate include", func(s *MCPSource) { s.Tools.IncludeNames = []string{"same", "same"} }, "tools.includeNames[1]"},
		{"duplicate exclude", func(s *MCPSource) { s.Tools.ExcludeNames = []string{"same", "same"} }, "tools.excludeNames[1]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := mcpSourceContractForTest()
			test.change(source)
			err := source.Validate(&common.ValidationContext{Path: "source.mcp"})
			if test.field == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "source.mcp."+test.field)
			}
		})
	}
	for _, interval := range []string{"", "0s", "-1s", "1d", "1.5", " 5m", "999999999999999999999h"} {
		source := mcpSourceContractForTest()
		source.RefreshInterval = &interval
		require.ErrorContains(t, source.Validate(nil), "refreshInterval", interval)
	}
	require.Error(t, (*MCPSource)(nil).Validate(nil))
	require.NoError(t, (*MCPToolFilter)(nil).Validate(nil))
	filter := &MCPToolFilter{
		IncludeNames: []string{"list_calendars", " list_calendars", "LIST_CALENDARS"},
		ExcludeNames: []string{"list_calendars"},
	}
	require.NoError(t, filter.Validate(nil), "names remain exact; overlap is valid because exclusions take precedence")
	require.Equal(t, " list_calendars", filter.IncludeNames[1])
	require.NoError(t, (&MCPToolFilter{ExcludeNames: []string{}}).Validate(nil))
}

// TestMCPSourceRoundTrip preserves fractional and compound duration text along
// with exact upstream names across both supported authoring formats.
func TestMCPSourceRoundTrip(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"JSON", json.Marshal, util.DecodeJSONStrict},
		{"YAML", yaml.Marshal, util.DecodeYAMLStrict},
	} {
		t.Run(format.name, func(t *testing.T) {
			for _, interval := range []string{"1.5s", "1m30.5s", "+5m", ".5s", "250µs"} {
				source := mcpSourceContractForTest()
				source.RefreshInterval = &interval
				require.NoError(t, source.Validate(nil))
				encoded, err := format.encode(source)
				require.NoError(t, err)
				var decoded MCPSource
				require.NoError(t, format.decode(encoded, &decoded))
				require.NoError(t, decoded.Validate(nil))
				require.Equal(t, source, &decoded)
			}
			for _, filter := range []*MCPToolFilter{{}, {ExcludeNames: []string{}}, {IncludeNames: []string{}}} {
				encoded, err := format.encode(filter)
				require.NoError(t, err)
				var decoded MCPToolFilter
				require.NoError(t, format.decode(encoded, &decoded))
				require.Equal(t, filter, &decoded, "invalid empty inclusion must not serialize as unrestricted omission")
				if filter.IncludeNames != nil {
					require.ErrorContains(t, decoded.Validate(nil), "includeNames")
				}
			}
		})
	}
}

// TestMCPSourceStrictDecoding rejects malformed owned fields at decoding time,
// while semantic validation separately rejects blank, duplicate, or empty names.
func TestMCPSourceStrictDecoding(t *testing.T) {
	for _, input := range []string{
		`{"Endpoint":"https://example.test"}`, `{"endpoint":null}`, `{"endpoint":5}`,
		`{"transport":null}`, `{"refreshInterval":null}`, `{"refreshInterval":5}`,
		`{"tools":null}`, `{"tools":[]}`, `{"protocolVersion":"2026-07-28"}`,
		`{"tools":{"includeNames":null}}`, `{"tools":{"excludeNames":null}}`,
		`{"tools":{"includeNames":[null]}}`, `{"tools":{"excludeNames":[true]}}`,
		`{"tools":{"includeNames":"one"}}`, `{"tools":{"IncludeNames":["one"]}}`,
		`{"tools":{"namePatterns":["*"]}}`, `[]`,
	} {
		for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
			var source MCPSource
			require.Error(t, decode([]byte(input), &source), input)
		}
	}
	var source MCPSource
	require.Error(t, util.DecodeJSONStrict([]byte(`null`), &source))
	// yaml.v3 skips custom unmarshalling for a root null node. A freshly decoded
	// source still fails required-field validation; nested nulls are caught above.
	require.NoError(t, util.DecodeYAMLStrict([]byte(`null`), &source))
	require.Error(t, source.Validate(nil))
	require.Error(t, util.DecodeJSONStrict([]byte(`{} {}`), &source))
}

// TestMCPSourceYAMLComposition resolves ordinary aliases and merge keys without
// allowing null values introduced indirectly to disappear into optional fields.
func TestMCPSourceYAMLComposition(t *testing.T) {
	var source MCPSource
	input := `
<<: {endpoint: "https://{{cfg.apiHost}}/mcp", transport: streamableHttp, refreshInterval: "1.5s"}
tools:
  includeNames: [&name list_calendars, create_event]
  excludeNames: [*name]
`
	require.NoError(t, util.DecodeYAMLStrict([]byte(input), &source))
	require.NoError(t, source.Validate(nil))
	require.Equal(t, []string{"list_calendars"}, source.Tools.ExcludeNames)
	for _, input := range []string{
		"<<: {refreshInterval: null}\n",
		"refreshInterval: &none null\ntools: *none\n",
		"tools:\n  <<: {includeNames: null}\n",
		"tools:\n  includeNames: [&none null, *none]\n",
	} {
		require.Error(t, util.DecodeYAMLStrict([]byte(input), &source), input)
	}
}

// TestMCPSourceCloneAndDecodeOwnership protects pinned source settings from
// changes through clones, failed decodes, and the native YAML marshal result.
func TestMCPSourceCloneAndDecodeOwnership(t *testing.T) {
	source := mcpSourceContractForTest()
	clone := source.Clone()
	require.Equal(t, source, clone)
	clone.Endpoint = "changed"
	*clone.RefreshInterval = "5m"
	clone.Tools.IncludeNames[0] = "changed"
	clone.Tools.ExcludeNames[0] = "changed"
	require.Equal(t, mcpSourceContractForTest(), source)
	require.Nil(t, (*MCPSource)(nil).Clone())
	require.Nil(t, (*MCPToolFilter)(nil).Clone())
	require.Equal(t, &MCPSource{}, (&MCPSource{}).Clone())
	filter := (&MCPToolFilter{IncludeNames: []string{}}).Clone()
	require.NotNil(t, filter.IncludeNames)
	require.Nil(t, filter.ExcludeNames)
	value, err := source.Tools.MarshalYAML()
	require.NoError(t, err)
	value.(map[string][]string)["includeNames"][0] = "changed"
	require.Equal(t, mcpSourceContractForTest(), source)

	require.Error(t, util.DecodeJSONStrict([]byte(`{"endpoint":"changed","tools":{"includeNames":[null]}}`), source))
	require.Equal(t, mcpSourceContractForTest(), source, "failed decoding is transactional")
	require.NoError(t, util.DecodeJSONStrict([]byte(`{"endpoint":"https://example.test/mcp","transport":"streamableHttp"}`), source))
	require.Nil(t, source.RefreshInterval)
	require.Nil(t, source.Tools)
}

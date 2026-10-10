package toolsets

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestFilterWireShapes fixes the authored keys and list shapes independently of
// the filter decoders, so encoding and decoding cannot hide the same regression.
func TestFilterWireShapes(t *testing.T) {
	for _, test := range []struct {
		name   string
		filter any
		want   string
	}{
		{"MCP omitted", MCPToolFilter{}, `{}`},
		{"MCP empty inclusion", MCPToolFilter{IncludeNames: &[]string{}}, `{"includeNames":[]}`},
		{"MCP empty exclusion", MCPToolFilter{ExcludeNames: &[]string{}}, `{"excludeNames":[]}`},
		{"MCP populated", MCPToolFilter{IncludeNames: &[]string{"read", "write"}, ExcludeNames: &[]string{"write"}}, `{"includeNames":["read","write"],"excludeNames":["write"]}`},
		{"OpenAPI omitted", OpenAPIOperationFilter{}, `{}`},
		{"OpenAPI empty inclusion", OpenAPIOperationFilter{IncludeOperationIDs: &[]string{}}, `{"includeOperationIds":[]}`},
		{"OpenAPI empty exclusion", OpenAPIOperationFilter{ExcludeOperationIDs: &[]string{}}, `{"excludeOperationIds":[]}`},
		{"OpenAPI populated", OpenAPIOperationFilter{IncludeOperationIDs: &[]string{"read", "write"}, ExcludeOperationIDs: &[]string{"write"}}, `{"includeOperationIds":["read","write"],"excludeOperationIds":["write"]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, format := range []struct {
				encode func(any) ([]byte, error)
				decode func([]byte, any) error
			}{{json.Marshal, util.DecodeJSONStrict}, {yaml.Marshal, util.DecodeYAMLStrict}} {
				encoded, err := format.encode(test.filter)
				require.NoError(t, err)
				var actual map[string]any
				require.NoError(t, format.decode(encoded, &actual))
				actualJSON, err := json.Marshal(actual)
				require.NoError(t, err)
				require.JSONEq(t, test.want, string(actualJSON))
			}
		})
	}
}

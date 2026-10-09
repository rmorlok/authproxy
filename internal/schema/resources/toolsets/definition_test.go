package toolsets

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// explicitDefinitionForTest provides a complete, connection-independent
// inventory whose metadata and definition can be changed independently.
func explicitDefinitionForTest() *ToolSetDefinition {
	return &ToolSetDefinition{
		Source: ToolSetSource{
			Explicit: &ExplicitSource{
				Tools: []ToolTemplate{
					{
						Key: "list-records",
						Metadata: &ToolTemplateMetadata{
							Name: "list-records", Labels: map[string]string{
								"capability": "records",
							},
							Annotations: map[string]string{
								"owner": "platform",
							},
						},
						Spec: tools.ToolDefinition{
							Description: "List records",
							Verbs: []string{
								"tool:records.list",
							},
							InputSchema: common.RawJSON(`{"type":"object"}`),
							ProxyHTTP: &tools.ProxyHTTP{
								Method: "GET",
								URL:    "https://{{cfg.host}}/records",
							},
						},
					},
				},
			},
		},
	}
}

// TestExplicitDefinitionValidation checks stable source identity and complete
// template validation, with paths rooted at the caller's generation definition.
func TestExplicitDefinitionValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ToolSetDefinition)
		field  string
	}{
		{"valid", func(d *ToolSetDefinition) {}, ""},
		{"absent source", func(d *ToolSetDefinition) { d.Source.Explicit = nil }, "source.explicit"},
		{"missing inventory", func(d *ToolSetDefinition) { d.Source.Explicit.Tools = nil }, "source.explicit.tools"},
		{"empty inventory", func(d *ToolSetDefinition) { d.Source.Explicit.Tools = []ToolTemplate{} }, ""},
		{"blank key", func(d *ToolSetDefinition) { d.Source.Explicit.Tools[0].Key = " \n" }, "source.explicit.tools[0].key"},
		{"duplicate key", func(d *ToolSetDefinition) {
			d.Source.Explicit.Tools = append(d.Source.Explicit.Tools, d.Source.Explicit.Tools[0])
		}, "source.explicit.tools[1].key"},
		{"no display metadata", func(d *ToolSetDefinition) { d.Source.Explicit.Tools[0].Metadata = nil }, ""},
		{"invalid display name", func(d *ToolSetDefinition) { d.Source.Explicit.Tools[0].Metadata.Name = "not a name" }, "source.explicit.tools[0].metadata.name"},
		{"reserved authored label", func(d *ToolSetDefinition) {
			d.Source.Explicit.Tools[0].Metadata.Labels = map[string]string{"apxy/cxn/-/id": "cxn_example"}
		}, "source.explicit.tools[0].metadata.labels"},
		{"invalid annotation", func(d *ToolSetDefinition) {
			d.Source.Explicit.Tools[0].Metadata.Annotations = map[string]string{"bad key": "value"}
		}, "source.explicit.tools[0].metadata.annotations"},
		{"missing executor", func(d *ToolSetDefinition) { d.Source.Explicit.Tools[0].Spec.ProxyHTTP = nil }, "source.explicit.tools[0].spec"},
		{"missing verbs", func(d *ToolSetDefinition) { d.Source.Explicit.Tools[0].Spec.Verbs = nil }, "source.explicit.tools[0].spec.verbs"},
		{"invalid native schema", func(d *ToolSetDefinition) {
			d.Source.Explicit.Tools[0].Spec.InputSchema = common.RawJSON(`{"type":"array"}`)
		}, "source.explicit.tools[0].spec.inputSchema"},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := explicitDefinitionForTest()
			test.change(definition)
			err := definition.Validate(&common.ValidationContext{Path: "definition"})
			if test.field == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "definition."+test.field)
			}
		})
	}

	definition := explicitDefinitionForTest()
	definition.Source.Explicit.Tools[0].Key = "GET /records/{id} "
	definition.Source.Explicit.Tools = append(definition.Source.Explicit.Tools, definition.Source.Explicit.Tools[0])
	definition.Source.Explicit.Tools[1].Key = "GET /records/{id}"

	require.NoError(t, definition.Validate(nil), "keys are exact opaque identities; duplicate naming stems are allowed")
	require.Equal(t, "GET /records/{id} ", definition.Source.Explicit.Tools[0].Key)
	require.Error(t, (*ToolSetDefinition)(nil).Validate(nil))
	require.Error(t, (*ToolSetSource)(nil).Validate(nil))
	require.Error(t, (*ExplicitSource)(nil).Validate(nil))
	require.Error(t, (*ToolTemplate)(nil).Validate(nil))
	require.NoError(t, (*ToolTemplateMetadata)(nil).Validate(nil))
}

// TestExplicitDefinitionWireBoundary reuses strict authored ToolDefinition
// decoding, excluding controller identity and connection binding in templates.
func TestExplicitDefinitionWireBoundary(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"JSON", json.Marshal, util.DecodeJSONStrict},
		{"YAML", yaml.Marshal, util.DecodeYAMLStrict},
	} {
		t.Run(format.name, func(t *testing.T) {
			original := explicitDefinitionForTest()
			data, err := format.encode(original)
			require.NoError(t, err)
			var decoded ToolSetDefinition
			require.NoError(t, format.decode(data, &decoded))
			require.NoError(t, decoded.Validate(nil))
			require.Equal(t, original, &decoded)
			for _, field := range []string{"id", "namespace", "generation", "managedBy", "createdAt", "updatedAt"} {
				input := []byte(`{"source":{"explicit":{"tools":[{"key":"k","metadata":{"` + field + `":"server-owned"},"spec":{}}]}}}`)
				require.Error(t, format.decode(input, &decoded), field)
			}
			for _, input := range []string{
				`{"source":{"openapi":{}}}`, `{"source":{"mcp":{}}}`,
				`{"source":{"explicit":{"tools":[]}},"connectionSelector":{}}`,
				`{"source":{"explicit":{"tools":[{"key":"k","spec":{"connectionRef":{}}}]}}}`,
				`{"source":{"explicit":{"tools":[{"key":"k","spec":{"openapiOperation":{}}}]}}}`,
				`{"source":{"explicit":{"tools":[{"key":"k","spec":{"mcpCall":{}}}]}}}`,
			} {
				require.Error(t, format.decode([]byte(input), &decoded), input)
			}
			for _, input := range []string{`{}`, `{"source":{}}`, `{"source":{"explicit":null}}`, `{"source":{"explicit":{}}}`, `{"source":{"explicit":{"tools":null}}}`} {
				var invalid ToolSetDefinition
				require.NoError(t, format.decode([]byte(input), &invalid))
				require.Error(t, invalid.Validate(nil), input)
			}
		})
	}
}

// TestExplicitDefinitionCloneIsDetached protects the source inventory from
// template and metadata mutations made while preparing a candidate generation.
func TestExplicitDefinitionCloneIsDetached(t *testing.T) {
	original := explicitDefinitionForTest()
	original.Source.Explicit.Tools[0].Spec.ProxyHTTP.Query = map[string]common.RawJSON{"q": common.RawJSON(`null`)}
	original.Source.Explicit.Tools[0].Spec.InputSchema = common.RawJSON(" { malformed ")
	clone := original.Clone()
	require.Equal(t, original, clone)
	template := &clone.Source.Explicit.Tools[0]
	template.Key = "other"
	template.Metadata.Name = "other"
	template.Metadata.Labels["capability"] = "other"
	template.Metadata.Annotations["owner"] = "other"
	template.Spec.Verbs[0] = "tool:other"
	template.Spec.InputSchema[0] = '['
	template.Spec.ProxyHTTP.Query["q"][0] = 'N'
	unchanged := &original.Source.Explicit.Tools[0]
	require.Equal(t, "list-records", unchanged.Key)
	require.Equal(t, common.ResourceName("list-records"), unchanged.Metadata.Name)
	require.Equal(t, "records", unchanged.Metadata.Labels["capability"])
	require.Equal(t, "platform", unchanged.Metadata.Annotations["owner"])
	require.Equal(t, []string{"tool:records.list"}, unchanged.Spec.Verbs)
	require.Equal(t, common.RawJSON(" { malformed "), unchanged.Spec.InputSchema)
	require.Equal(t, common.RawJSON(`null`), unchanged.Spec.ProxyHTTP.Query["q"])
	require.Nil(t, (*ToolSetDefinition)(nil).Clone())
	for _, definition := range []*ToolSetDefinition{
		{}, {Source: ToolSetSource{Explicit: &ExplicitSource{}}},
		{Source: ToolSetSource{Explicit: &ExplicitSource{Tools: []ToolTemplate{}}}},
		{Source: ToolSetSource{Explicit: &ExplicitSource{Tools: []ToolTemplate{{Key: "incomplete"}}}}},
	} {
		require.Equal(t, definition, definition.Clone(), "clone must retain empty and invalid shapes for validation")
	}
}

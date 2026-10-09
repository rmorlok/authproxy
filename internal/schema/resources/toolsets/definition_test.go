package toolsets

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
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
		{"absent source", func(d *ToolSetDefinition) { d.Source.Explicit = nil }, "source"},
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
				`{"source":{"unknown":{}}}`, `{"source":{"explicit":null}}`,
				`{"source":{"explicit":{"tools":[]}},"connectionSelector":{}}`,
				`{"source":{"explicit":{"tools":[{"key":"k","spec":{"connectionRef":{}}}]}}}`,
				`{"source":{"explicit":{"tools":[{"key":"k","spec":{"openapiOperation":{}}}]}}}`,
				`{"source":{"explicit":{"tools":[{"key":"k","spec":{"mcpCall":{}}}]}}}`,
			} {
				require.Error(t, format.decode([]byte(input), &decoded), input)
			}
			for _, input := range []string{`{}`, `{"source":{}}`, `{"source":{"mcp":{}}}`, `{"source":{"openapi":{}}}`, `{"source":{"explicit":{}}}`, `{"source":{"explicit":{"tools":null}}}`} {
				var invalid ToolSetDefinition
				require.NoError(t, format.decode([]byte(input), &invalid))
				require.Error(t, invalid.Validate(nil), input)
			}
		})
	}
}

// importedDefinitionForTest includes every mutable imported-policy field while
// keeping source names exact and publication independent of a sample catalog.
func importedDefinitionForTest() *ToolSetDefinition {
	return &ToolSetDefinition{
		Source: ToolSetSource{MCP: &MCPSource{
			Endpoint: "https://{{cfg.apiHost}}/mcp", Transport: MCPTransportStreamableHTTP,
			RefreshInterval: util.ToPtr("1.5s"),
			Tools:           &MCPToolFilter{IncludeNames: []string{"list_calendars", "admin_reset"}, ExcludeNames: []string{"admin_reset"}},
		}},
		PermissionMappings: []PermissionMapping{{
			Match:    SourceKeyMatch{SourceKeys: []string{"list_calendars"}, SourceKeyPatterns: []string{"calendar_.*"}},
			AddVerbs: []string{"tool:calendar.list", "tool:readonly"},
		}},
	}
}

// TestImportedDefinitionBoundary validates source exclusivity and policy
// ownership without requiring a provider catalog or performing any network I/O.
func TestImportedDefinitionBoundary(t *testing.T) {
	definition := importedDefinitionForTest()
	require.NoError(t, definition.Validate(nil))
	definition.Source.Explicit = &ExplicitSource{Tools: []ToolTemplate{}}
	require.ErrorContains(t, definition.Validate(nil), "exactly one")
	definition.Source.MCP = nil
	require.ErrorContains(t, definition.Validate(nil), "permissionMappings")
	definition.PermissionMappings = []PermissionMapping{}
	require.ErrorContains(t, definition.Validate(nil), "permissionMappings", "even an empty imported policy is invalid on an explicit source")
	encoded, err := json.Marshal(definition)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"permissionMappings":[]`)
	var decoded ToolSetDefinition
	require.NoError(t, util.DecodeJSONStrict(encoded, &decoded))
	require.ErrorContains(t, decoded.Validate(nil), "permissionMappings")
	definition.PermissionMappings = nil
	require.NoError(t, definition.Validate(nil))

	definition = importedDefinitionForTest()
	definition.PermissionMappings[0].Match = SourceKeyMatch{}
	require.ErrorContains(t, definition.Validate(&common.ValidationContext{Path: "definition"}), "definition.permissionMappings[0].match")

	for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
		for _, input := range []string{
			`{"source":null}`, `{"Source":{}}`,
			`{"source":{"MCP":{}}}`, `{"source":{"unknown":{}}}`,
			`{"source":{"explicit":{"tools":[]},"mcp":null}}`,
			`{"source":{"mcp":{"endpoint":"https://example.com/mcp","transport":"streamableHttp"},"explicit":null}}`,
			`{"permissionMappings":null}`, `{"permissionMappings":[null]}`,
			`{"defaults":{}}`,
		} {
			require.Error(t, decode([]byte(input), &decoded), input)
		}
	}
	require.Error(t, json.Unmarshal([]byte(`null`), &decoded))
	// yaml.v3 does not call custom unmarshalers for a document-level null.
	// Input boundaries decode fresh values and then validate required shape.
	var empty ToolSetDefinition
	require.NoError(t, util.DecodeYAMLStrict([]byte("null"), &empty))
	require.Error(t, empty.Validate(nil))
	for _, input := range []string{
		"source:\n  <<: {mcp: null}\n  explicit: {tools: []}",
		"source: {explicit: {tools: []}}\n<<: {permissionMappings: null}",
		"source: {explicit: {tools: []}}\npermissionMappings: [ &empty null, *empty ]",
	} {
		require.Error(t, util.DecodeYAMLStrict([]byte(input), &decoded), input)
	}
	original := importedDefinitionForTest()
	decoded = *original.Clone()
	require.Error(t, json.Unmarshal([]byte(`{"source":{"unknown":{}}}`), &decoded))
	require.Equal(t, original, &decoded, "failed definition decoding is atomic")
}

// TestImportedDefinitionRoundTrip checks complete resources and definition
// replacements, including empty imported policy and excluded-name lists.
func TestImportedDefinitionRoundTrip(t *testing.T) {
	for _, emptyPolicies := range []bool{false, true} {
		resource := authoredToolSetForResourceTest()
		resource.Spec.Definition = *importedDefinitionForTest()
		if emptyPolicies {
			resource.Spec.Definition.PermissionMappings = []PermissionMapping{}
			resource.Spec.Definition.Source.MCP.Tools.ExcludeNames = []string{}
		}
		for _, format := range []struct {
			encode func(any) ([]byte, error)
			decode func([]byte, any) error
		}{{json.Marshal, util.DecodeJSONStrict}, {yaml.Marshal, util.DecodeYAMLStrict}} {
			data, err := format.encode(resource)
			require.NoError(t, err)
			var decoded ToolSet
			require.NoError(t, format.decode(data, &decoded))
			require.NoError(t, decoded.Validate(nil))
			require.Equal(t, resource, &decoded)
			patch := NewToolSetPatch()
			patch.Spec.Definition = resource.Spec.Definition.Clone()
			data, err = format.encode(patch)
			require.NoError(t, err)
			var decodedPatch ToolSetPatch
			require.NoError(t, format.decode(data, &decodedPatch))
			require.NoError(t, decodedPatch.ValidateFor(meta.ValidationModeUpdate, nil))
			require.Equal(t, patch.Spec.Definition, decodedPatch.Spec.Definition)
			require.True(t, decodedPatch.Spec.HasDefinition())
		}
	}
}

// TestImportedDefinitionCandidateOwnership carries imported policy through the
// existing patch/clone path while preserving the current generation and status.
func TestImportedDefinitionCandidateOwnership(t *testing.T) {
	current := storedToolSetForResourceTest()
	patch := NewToolSetPatch()
	patch.Spec.Definition = importedDefinitionForTest()
	beforeCurrent, beforePatch := current.Clone(), patch.Clone()
	require.True(t, GenerationPolicy().ChangesGeneration(patch))
	candidate, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	require.Nil(t, candidate.Spec.Definition.Source.Explicit)
	require.Equal(t, current.Status, candidate.Status)
	definition := &candidate.Spec.Definition
	*definition.Source.MCP.RefreshInterval = "1h"
	definition.Source.MCP.Tools.IncludeNames[0] = "other"
	definition.Source.MCP.Tools.ExcludeNames[0] = "other"
	definition.PermissionMappings[0].Match.SourceKeys[0] = "other"
	definition.PermissionMappings[0].Match.SourceKeyPatterns[0] = "other.*"
	definition.PermissionMappings[0].AddVerbs[0] = "tool:other"
	require.Equal(t, beforeCurrent, current)
	require.Equal(t, beforePatch, patch)

	// Invalid/empty shapes remain inspectable when cloned instead of being
	// normalized through serialization or treated as default policy.
	for _, mappings := range [][]PermissionMapping{nil, {}, {{}}} {
		definition := &ToolSetDefinition{PermissionMappings: mappings}
		require.Equal(t, definition, definition.Clone())
	}
}

// openAPIDefinitionForTest mirrors the authored source example and supplies
// opaque document content without depending on an OpenAPI parser or importer.
func openAPIDefinitionForTest(inline bool) *ToolSetDefinition {
	document := &OpenAPIDocument{URL: util.ToPtr("https://example.com/openapi.json")}
	if inline {
		document.URL = nil
		document.Inline = common.RawJSON(`{"openapi":"3.1.0","paths":{},"x-provider":{"example":9007199254740993,"schema":{"$ref":"https://never-fetch.invalid/schema.json"}}}`)
	} else {
		// Acquisition authority is checked later; this independent connection
		// intentionally belongs to a different namespace than the ToolSet.
		document.FetchConnectionRef = &meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1, Kind: "Connection", Namespace: "root.documents", Name: "spec-reader",
		}
	}
	return &ToolSetDefinition{
		Source: ToolSetSource{OpenAPI: &OpenAPISource{
			Document:   document,
			Operations: &OpenAPIOperationFilter{IncludeOperationIDs: []string{"listCalendars"}, ExcludeOperationIDs: []string{}},
			Server:     &OpenAPIServerOverride{URL: "https://{{cfg.apiHost}}"},
		}},
		PermissionMappings: []PermissionMapping{{Match: SourceKeyMatch{SourceKeys: []string{"listCalendars"}}, AddVerbs: []string{"tool:calendar.list"}}},
	}
}

// TestOpenAPIDefinitionIntegration exercises the complete resource and patch
// boundaries for URL and inline sources, including imported permission mappings.
func TestOpenAPIDefinitionIntegration(t *testing.T) {
	for _, inline := range []bool{false, true} {
		current := storedToolSetForResourceTest()
		current.Spec.Definition = *openAPIDefinitionForTest(inline)
		require.NoError(t, current.ValidateFor(meta.ValidationModeResponse, nil))
		for _, format := range []struct {
			encode func(any) ([]byte, error)
			decode func([]byte, any) error
		}{{json.Marshal, util.DecodeJSONStrict}, {yaml.Marshal, util.DecodeYAMLStrict}} {
			encoded, err := format.encode(current)
			require.NoError(t, err)
			var decoded ToolSet
			require.NoError(t, format.decode(encoded, &decoded))
			require.NoError(t, decoded.ValidateFor(meta.ValidationModeResponse, nil))
			before, err := json.Marshal(current)
			require.NoError(t, err)
			after, err := json.Marshal(decoded)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
			if inline {
				require.Contains(t, string(decoded.Spec.Definition.Source.OpenAPI.Document.Inline), "9007199254740993")
			}

			patch := NewToolSetPatch()
			patch.Spec.Definition = current.Spec.Definition.Clone()
			encoded, err = format.encode(patch)
			require.NoError(t, err)
			var decodedPatch ToolSetPatch
			require.NoError(t, format.decode(encoded, &decodedPatch))
			candidate, err := decodedPatch.ApplyTo(storedToolSetForResourceTest(), nil)
			require.NoError(t, err)
			require.NotNil(t, candidate.Spec.Definition.Source.OpenAPI)
			require.Nil(t, candidate.Spec.Definition.Source.Explicit)
			require.True(t, GenerationPolicy().ChangesGeneration(&decodedPatch))
		}
	}
}

// TestOpenAPISourceUnion rejects every pair of source kinds and prevents null
// branches from disappearing during JSON or YAML definition decoding.
func TestOpenAPISourceUnion(t *testing.T) {
	definition := openAPIDefinitionForTest(true)
	definition.Source.Explicit = &ExplicitSource{Tools: []ToolTemplate{}}
	require.ErrorContains(t, definition.Validate(nil), "exactly one")
	definition.Source.Explicit = nil
	definition.Source.MCP = importedDefinitionForTest().Source.MCP
	require.ErrorContains(t, definition.Validate(nil), "exactly one")
	definition.Source.Explicit = &ExplicitSource{Tools: []ToolTemplate{}}
	require.ErrorContains(t, definition.Validate(nil), "exactly one")

	for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
		for _, input := range []string{
			`{"source":{"explicit":{"tools":[]},"openapi":null}}`,
			`{"source":{"openapi":{"document":{"inline":{}}},"explicit":null}}`,
			`{"source":{"openapi":{"document":{"inline":{}}},"mcp":null}}`,
			`{"source":{"OpenAPI":{}}}`,
		} {
			var decoded ToolSetDefinition
			require.Error(t, decode([]byte(input), &decoded), input)
		}
	}
	var decoded ToolSetDefinition
	require.Error(t, util.DecodeYAMLStrict([]byte("source:\n  <<: {openapi: null}\n  explicit: {tools: []}"), &decoded))
}

// TestOpenAPIDefinitionClone isolates source acquisition settings, raw document
// bytes, filters, overrides, and aliases across generation candidates.
func TestOpenAPIDefinitionClone(t *testing.T) {
	for _, inline := range []bool{false, true} {
		original := openAPIDefinitionForTest(inline)
		before := original.Clone()
		clone := original.Clone()
		if inline {
			clone.Source.OpenAPI.Document.Inline[0] = '['
		} else {
			*clone.Source.OpenAPI.Document.URL = "https://other.example/spec.json"
			clone.Source.OpenAPI.Document.FetchConnectionRef.Namespace = "root.other"
		}
		clone.Source.OpenAPI.Operations.IncludeOperationIDs[0] = "other"
		clone.Source.OpenAPI.Operations.ExcludeOperationIDs = append(clone.Source.OpenAPI.Operations.ExcludeOperationIDs, "other")
		clone.Source.OpenAPI.Server.URL = "https://other.example"
		clone.PermissionMappings[0].AddVerbs[0] = "tool:other"
		require.Equal(t, before, original)
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

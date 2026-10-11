package toolsets

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// openAPIOperationPoliciesForTest includes an explicit operation selection and
// a stable identity that differs from its current provider operationId.
func openAPIOperationPoliciesForTest() *OpenAPISource {
	return &OpenAPISource{
		Document:              fetchedDocumentForTest(),
		Operations:            &OpenAPIOperationFilter{IncludeOperationIDs: util.ToPtr([]string{"listItemsV2"})},
		SourceKeyOverrides:    []OpenAPISourceKeyOverride{{Method: "GET", Path: "/items", SourceKey: "listItems"}},
		UnsupportedOperations: util.ToPtr(OpenAPIUnsupportedOperationPolicy("skip")),
	}
}

// TestOpenAPISourceKeyOverrideValidation preserves wire-method case and exact
// document path/key identities while leaving document compatibility to import.
func TestOpenAPISourceKeyOverrideValidation(t *testing.T) {
	require.Error(t, (*OpenAPISourceKeyOverride)(nil).Validate(nil))
	for _, override := range []OpenAPISourceKeyOverride{
		{Method: "GET", Path: "/", SourceKey: "root"},
		{Method: "get", Path: "/items/{id}", SourceKey: "GET /items/{id}"},
		{Method: "Custom!#$%&'*+-.^_`|~", Path: "//Items/%2f/{id}", SourceKey: " padded key "},
	} {
		before := override
		require.NoError(t, override.Validate(nil))
		require.Equal(t, before, override)
	}
	for _, test := range []struct {
		field string
		edit  func(*OpenAPISourceKeyOverride)
	}{
		{"method", func(v *OpenAPISourceKeyOverride) { v.Method = "" }},
		{"method", func(v *OpenAPISourceKeyOverride) { v.Method = "GET POST" }},
		{"method", func(v *OpenAPISourceKeyOverride) { v.Method = "GET\n" }},
		{"method", func(v *OpenAPISourceKeyOverride) { v.Method = "méthod" }},
		{"path", func(v *OpenAPISourceKeyOverride) { v.Path = "" }},
		{"path", func(v *OpenAPISourceKeyOverride) { v.Path = "items" }},
		{"path", func(v *OpenAPISourceKeyOverride) { v.Path = "https://example.test/items" }},
		{"sourceKey", func(v *OpenAPISourceKeyOverride) { v.SourceKey = " \t\n" }},
	} {
		override := OpenAPISourceKeyOverride{Method: "GET", Path: "/items", SourceKey: "listItems"}
		test.edit(&override)
		require.ErrorContains(t, override.Validate(&common.ValidationContext{Path: "override"}), "override."+test.field)
	}
}

// TestOpenAPIOperationPoliciesValidation rejects conflicting identities without
// selecting operations or applying unsupported-feature handling prematurely.
func TestOpenAPIOperationPoliciesValidation(t *testing.T) {
	for _, policy := range []*OpenAPIUnsupportedOperationPolicy{nil, util.ToPtr(OpenAPIUnsupportedOperationPolicy("error")), util.ToPtr(OpenAPIUnsupportedOperationPolicy("skip"))} {
		source := openAPIOperationPoliciesForTest()
		source.UnsupportedOperations = policy
		source.SourceKeyOverrides = append(source.SourceKeyOverrides,
			OpenAPISourceKeyOverride{Method: "get", Path: "/items", SourceKey: "lowercaseMethod"},
			OpenAPISourceKeyOverride{Method: "GET", Path: "/Items", SourceKey: "uppercasePath"})
		before := source.Clone()
		require.NoError(t, source.Validate(nil), "skip can coexist with explicit inclusion")
		require.Equal(t, before, source)
	}
	for _, overrides := range [][]OpenAPISourceKeyOverride{nil, {}} {
		source := openAPIOperationPoliciesForTest()
		source.SourceKeyOverrides = overrides
		require.NoError(t, source.Validate(nil))
	}
	for _, duplicate := range []OpenAPISourceKeyOverride{
		{Method: "GET", Path: "/items", SourceKey: "anotherKey"},
		{Method: "POST", Path: "/other", SourceKey: "listItems"},
	} {
		source := openAPIOperationPoliciesForTest()
		source.SourceKeyOverrides = append(source.SourceKeyOverrides, duplicate)
		require.ErrorContains(t, source.Validate(nil), "sourceKeyOverrides[1]")
	}
	for _, policy := range []OpenAPIUnsupportedOperationPolicy{"", "Skip", "ignore"} {
		source := openAPIOperationPoliciesForTest()
		source.UnsupportedOperations = &policy
		require.ErrorContains(t, source.Validate(nil), "unsupportedOperations")
	}
}

// TestOpenAPIOperationPoliciesCodecs preserves authored settings and rejects
// malformed replacements atomically, including null values inside override lists.
func TestOpenAPIOperationPoliciesCodecs(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{{"JSON", json.Marshal, util.DecodeJSONStrict}, {"YAML", yaml.Marshal, util.DecodeYAMLStrict}} {
		t.Run(format.name, func(t *testing.T) {
			original := openAPIOperationPoliciesForTest()
			encoded, err := format.encode(original)
			require.NoError(t, err)
			var decoded OpenAPISource
			require.NoError(t, format.decode(encoded, &decoded))
			require.Equal(t, original, &decoded)
			for _, fields := range []string{
				`"sourceKeyOverrides":null`, `"sourceKeyOverrides":{}`, `"sourceKeyOverrides":[null]`,
				`"sourceKeyOverrides":[{"method":null}]`, `"sourceKeyOverrides":[{"path":null}]`,
				`"sourceKeyOverrides":[{"sourceKey":null}]`, `"sourceKeyOverrides":[{"method":3}]`,
				`"sourceKeyOverrides":[{"Method":"GET"}]`, `"sourceKeyOverrides":[{"unknown":true}]`,
				`"SourceKeyOverrides":[]`, `"unsupportedOperations":null`, `"unsupportedOperations":true`,
				`"UnsupportedOperations":"skip"`,
			} {
				before := decoded.Clone()
				require.Error(t, format.decode([]byte(`{"document":{"inline":{}},`+fields+`}`), &decoded), fields)
				require.Equal(t, before, &decoded)
			}
			override := original.SourceKeyOverrides[0]
			require.Error(t, format.decode([]byte(`{"method":"POST","path":"/changed","sourceKey":null}`), &override))
			require.Equal(t, original.SourceKeyOverrides[0], override)
			require.NoError(t, format.decode([]byte(`{"document":{"inline":{}}}`), &decoded))
			require.Nil(t, decoded.SourceKeyOverrides)
			require.Nil(t, decoded.UnsupportedOperations)
		})
	}
}

// TestOpenAPIOperationPoliciesYAMLComposition retains aliased exact identities
// while ensuring merged and aliased null configuration cannot disappear.
func TestOpenAPIOperationPoliciesYAMLComposition(t *testing.T) {
	var source OpenAPISource
	require.NoError(t, util.DecodeYAMLStrict([]byte(`
document: {inline: {}}
<<: {unsupportedOperations: skip}
sourceKeyOverrides:
  - method: &method Custom
    path: /items/{id}
    sourceKey: *method
`), &source))
	require.NoError(t, source.Validate(nil))
	require.Equal(t, "Custom", source.SourceKeyOverrides[0].SourceKey)
	for _, input := range []string{
		"<<: {sourceKeyOverrides: null}",
		"<<: {unsupportedOperations: null}",
		"sourceKeyOverrides: [{method: &nothing null, path: /items, sourceKey: *nothing}]",
	} {
		before := source.Clone()
		require.Error(t, util.DecodeYAMLStrict([]byte("document: {inline: {}}\n"+input), &source), input)
		require.Equal(t, before, &source)
	}
}

// TestOpenAPIOperationPoliciesClone detaches override storage and the policy
// pointer without hiding omitted, empty, or invalid authored values.
func TestOpenAPIOperationPoliciesClone(t *testing.T) {
	for _, overrides := range [][]OpenAPISourceKeyOverride{nil, {}, {{}}} {
		source := &OpenAPISource{SourceKeyOverrides: overrides}
		require.Equal(t, source, source.Clone())
	}
	source := openAPIOperationPoliciesForTest()
	clone := source.Clone()
	require.Equal(t, source, clone)
	clone.SourceKeyOverrides[0].Method = "POST"
	clone.SourceKeyOverrides[0].Path = "/changed"
	clone.SourceKeyOverrides[0].SourceKey = "changed"
	*clone.UnsupportedOperations = OpenAPIUnsupportedOperationPolicy("invalid")
	require.Equal(t, openAPIOperationPoliciesForTest(), source)
}

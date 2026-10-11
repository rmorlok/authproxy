package toolsets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// importedDefaultsForTest includes every limit pointer so clone tests exercise
// the complete shared limit contract alongside independently owned metadata.
func importedDefaultsForTest() *ImportedToolDefaults {
	return &ImportedToolDefaults{
		Labels:      map[string]string{"capability": "calendar", "empty": ""},
		Annotations: map[string]string{"owner": "platform", "example": `{"literal":"{{cfg.tenant}}"}`},
		Limits: &tools.ExecutionLimits{
			TimeoutMillis: util.ToPtr(int64(1500)), MaxRequests: util.ToPtr(2),
			MaxInputBytes: util.ToPtr(int64(1024)), MaxOutputBytes: util.ToPtr(int64(2048)),
			MaxResponseBytes: util.ToPtr(int64(4096)), MaxEncodedResultBytes: util.ToPtr(int64(8192)),
			MaxStackDepth: util.ToPtr(10),
		},
	}
}

// TestImportedToolDefaultsValidation checks metadata ownership and delegates
// limit validation without materializing generated tools or resolving policy.
func TestImportedToolDefaultsValidation(t *testing.T) {
	for _, defaults := range []*ImportedToolDefaults{nil, {}, importedDefaultsForTest(), {Labels: map[string]string{}, Annotations: map[string]string{}, Limits: &tools.ExecutionLimits{}}} {
		before := defaults.Clone()
		require.NoError(t, defaults.Validate(nil))
		require.Equal(t, before, defaults)
	}
	for _, test := range []struct {
		field string
		edit  func(*ImportedToolDefaults)
	}{
		{"labels", func(d *ImportedToolDefaults) { d.Labels["apxy/cxn/-/id"] = "cxn_example" }},
		{"labels", func(d *ImportedToolDefaults) { d.Labels["bad key"] = "value" }},
		{"annotations", func(d *ImportedToolDefaults) { d.Annotations["bad key"] = "value" }},
		{"annotations", func(d *ImportedToolDefaults) {
			d.Annotations["large"] = strings.Repeat("x", meta.AnnotationsTotalMaxSize)
		}},
		{"limits.maxRequests", func(d *ImportedToolDefaults) { *d.Limits.MaxRequests = 0 }},
		{"limits.timeoutMillis", func(d *ImportedToolDefaults) { *d.Limits.TimeoutMillis = -1 }},
	} {
		defaults := importedDefaultsForTest()
		test.edit(defaults)
		require.ErrorContains(t, defaults.Validate(&common.ValidationContext{Path: "defaults"}), "defaults."+test.field)
	}
}

// TestImportedToolDefaultsSourceOwnership limits defaults to imported sources,
// including an explicitly empty object that must not silently disappear.
func TestImportedToolDefaultsSourceOwnership(t *testing.T) {
	for _, definition := range []*ToolSetDefinition{importedDefinitionForTest(), openAPIDefinitionForTest(true)} {
		definition.Defaults = importedDefaultsForTest()
		require.NoError(t, definition.Validate(nil))
	}
	for _, defaults := range []*ImportedToolDefaults{{}, importedDefaultsForTest()} {
		definition := explicitDefinitionForTest()
		definition.Defaults = defaults
		require.ErrorContains(t, definition.Validate(nil), "defaults")
		data, err := json.Marshal(definition)
		require.NoError(t, err)
		var decoded ToolSetDefinition
		require.NoError(t, util.DecodeJSONStrict(data, &decoded))
		require.NotNil(t, decoded.Defaults)
		require.ErrorContains(t, decoded.Validate(nil), "defaults")
	}
}

// TestImportedToolDefaultsCodecs preserves literal metadata and positive caps,
// rejects malformed replacements atomically, and clears omitted old defaults.
func TestImportedToolDefaultsCodecs(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{{"JSON", json.Marshal, util.DecodeJSONStrict}, {"YAML", yaml.Marshal, util.DecodeYAMLStrict}} {
		t.Run(format.name, func(t *testing.T) {
			original := importedDefaultsForTest()
			data, err := format.encode(original)
			require.NoError(t, err)
			var decoded ImportedToolDefaults
			require.NoError(t, format.decode(data, &decoded))
			require.NoError(t, decoded.Validate(nil))
			require.Equal(t, original, &decoded)
			for _, input := range []string{
				`[]`, `true`, `{"Labels":{}}`, `{"labels":null}`, `{"annotations":null}`, `{"limits":null}`,
				`{"labels":[]}`, `{"annotations":true}`, `{"labels":{"capability":null}}`, `{"annotations":{"owner":null}}`,
				`{"labels":{"capability":7}}`, `{"annotations":{"owner":{}}}`, `{"limits":{"maxRequests":null}}`,
				`{"limits":{"MaxRequests":2}}`, `{"limits":{"unknown":2}}`, `{"limits":{"maxRequests":"2"}}`,
				`{"name":"shared-name"}`, `{"metadata":{}}`, `{"hints":{"readOnly":true}}`,
				`{"verbs":["tool:read"]}`, `{"connectionRef":{}}`, `{"sourceKey":"read"}`, `{"status":{}}`,
				`{"inputSchema":{}}`, `{"javascript":"execute"}`, `{"namespace":"root"}`, `{"generation":1}`,
			} {
				before := decoded.Clone()
				require.Error(t, format.decode([]byte(input), &decoded), input)
				require.Equal(t, before, &decoded)
			}
			require.NoError(t, format.decode([]byte(`{"labels":{},"annotations":{},"limits":{}}`), &decoded))
			require.NoError(t, decoded.Validate(nil))
			require.Empty(t, decoded.Labels)
			require.Empty(t, decoded.Annotations)
			require.Equal(t, &tools.ExecutionLimits{}, decoded.Limits)
			require.NoError(t, format.decode([]byte(`{}`), &decoded))
			require.Equal(t, ImportedToolDefaults{}, decoded)
			var definition ToolSetDefinition
			require.Error(t, format.decode([]byte(`{"defaults":null}`), &definition))
		})
	}
}

// TestImportedToolDefaultsYAMLComposition accepts aliases and merges while
// keeping null metadata and limit fields visible to strict validation.
func TestImportedToolDefaultsYAMLComposition(t *testing.T) {
	var defaults ImportedToolDefaults
	require.NoError(t, util.DecodeYAMLStrict([]byte("<<: {limits: {maxRequests: 2}}\nlabels: &labels {capability: calendar}\nannotations: *labels"), &defaults))
	require.NoError(t, defaults.Validate(nil))
	require.Equal(t, defaults.Labels, defaults.Annotations)
	for _, input := range []string{
		"<<: {labels: null}", "labels: &nothing null\nannotations: *nothing",
		"labels: {capability: &nothing null}\nannotations: {owner: *nothing}",
		"limits: {<<: {maxRequests: null}}",
	} {
		before := defaults.Clone()
		require.Error(t, util.DecodeYAMLStrict([]byte(input), &defaults), input)
		require.Equal(t, before, &defaults)
	}
}

// TestImportedToolDefaultsClone detaches maps and every limit pointer, retaining
// nil, empty, and invalid values for subsequent diagnostics.
func TestImportedToolDefaultsClone(t *testing.T) {
	require.Nil(t, (*ImportedToolDefaults)(nil).Clone())
	for _, defaults := range []*ImportedToolDefaults{{}, {Labels: map[string]string{}, Annotations: map[string]string{}, Limits: &tools.ExecutionLimits{MaxRequests: util.ToPtr(0)}}} {
		require.Equal(t, defaults, defaults.Clone())
	}
	original := importedDefaultsForTest()
	clone := original.Clone()
	clone.Labels["capability"] = "changed"
	clone.Annotations["owner"] = "changed"
	*clone.Limits.TimeoutMillis = 1
	*clone.Limits.MaxRequests = 1
	*clone.Limits.MaxInputBytes = 1
	*clone.Limits.MaxOutputBytes = 1
	*clone.Limits.MaxResponseBytes = 1
	*clone.Limits.MaxEncodedResultBytes = 1
	*clone.Limits.MaxStackDepth = 1
	require.Equal(t, importedDefaultsForTest(), original)
}

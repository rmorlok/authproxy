package toolsets

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// sourceDiagnosticForTest includes an exact source key and a pointer whose
// escaped slash and tilde must remain intact when stored or copied.
func sourceDiagnosticForTest() SourceDiagnostic {
	return SourceDiagnostic{
		Severity:  SourceDiagnosticSeverityWarning,
		Code:      "unsupported-parameter",
		Message:   "The parameter serialization is unsupported.",
		SourceKey: "GET /Items/{id} ",
		Location: &SourceDiagnosticLocation{
			DocumentURI: "urn:example:openapi",
			JSONPointer: util.ToPtr("/paths/~1Items~1{id}/get/parameters/~0name"),
		},
	}
}

// sourceDiagnosticResourceForTest attaches an import observation to a complete
// persisted OpenAPI generation without coupling it to compilation readiness.
func sourceDiagnosticResourceForTest() *ToolSet {
	resource := storedToolSetForResourceTest()
	resource.Spec.Definition = *openAPIDefinitionForTest(true)
	resource.Status.ImportDiagnostics = []SourceDiagnostic{sourceDiagnosticForTest()}
	return resource
}

// TestSourceDiagnosticValidation preserves extensible codes and exact identities
// while rejecting missing required fields and reporting nested field paths.
func TestSourceDiagnosticValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*SourceDiagnostic)
		field  string
	}{
		{"warning", func(d *SourceDiagnostic) {}, ""},
		{"error before identity", func(d *SourceDiagnostic) {
			d.Severity, d.SourceKey, d.Location = SourceDiagnosticSeverityError, "", nil
			d.Code = "future.reason/v2"
		}, ""},
		{"missing severity", func(d *SourceDiagnostic) { d.Severity = "" }, "severity"},
		{"unknown severity", func(d *SourceDiagnostic) { d.Severity = "Warning" }, "severity"},
		{"blank code", func(d *SourceDiagnostic) { d.Code = " \n" }, "code"},
		{"blank message", func(d *SourceDiagnostic) { d.Message = "\t" }, "message"},
		{"blank source key", func(d *SourceDiagnostic) { d.SourceKey = " " }, "sourceKey"},
		{"location pointer", func(d *SourceDiagnostic) { d.Location.JSONPointer = util.ToPtr("#/paths") }, "location.jsonPointer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			diagnostic := sourceDiagnosticForTest()
			test.change(&diagnostic)
			before := diagnostic.Clone()
			err := diagnostic.Validate(&common.ValidationContext{Path: "report"})
			if test.field == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "report."+test.field)
			}
			require.Equal(t, before, &diagnostic, "validation must not rewrite source data")
		})
	}
	require.Error(t, (*SourceDiagnostic)(nil).Validate(nil))
}

// TestSourceDiagnosticLocation distinguishes the root pointer from omission and
// accepts literal RFC 6901 tokens without URI-fragment decoding or resolution.
func TestSourceDiagnosticLocation(t *testing.T) {
	require.NoError(t, (*SourceDiagnosticLocation)(nil).Validate(nil))
	require.Error(t, (&SourceDiagnosticLocation{}).Validate(nil))
	for _, pointer := range []string{"", "/", "/a~1b/~0name", "/~01", "/literal%2F", "/a b"} {
		location := &SourceDiagnosticLocation{JSONPointer: util.ToPtr(pointer)}
		require.NoError(t, location.Validate(nil), pointer)
		require.Equal(t, pointer, *location.JSONPointer)
	}
	for _, pointer := range []string{"paths", "#/paths", "/~", "/~2", "/a~x"} {
		require.ErrorContains(t, (&SourceDiagnosticLocation{JSONPointer: util.ToPtr(pointer)}).Validate(nil), "jsonPointer", pointer)
	}
	for _, uri := range []string{"urn:example:spec", "file:///spec/openapi.yaml", "https://API.example.test/spec?version=2"} {
		location := &SourceDiagnosticLocation{DocumentURI: uri}
		require.NoError(t, location.Validate(nil), uri)
		require.Equal(t, uri, location.DocumentURI)
	}
	for _, uri := range []string{"./spec.yaml", "https://user:secret@example.test/spec", "https://example.test/spec#/paths", "https://{{cfg.host}}/spec", " "} {
		require.ErrorContains(t, (&SourceDiagnosticLocation{DocumentURI: uri}).Validate(nil), "documentUri")
	}
}

// TestSourceDiagnosticResourceWireBoundary exercises stored responses in both
// formats and keeps malformed observations separate from authored definitions.
func TestSourceDiagnosticResourceWireBoundary(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"JSON", json.Marshal, util.DecodeJSONStrict},
		{"YAML", yaml.Marshal, util.DecodeYAMLStrict},
	} {
		t.Run(format.name, func(t *testing.T) {
			original := sourceDiagnosticResourceForTest()
			original.Status.ImportDiagnostics = append(original.Status.ImportDiagnostics, SourceDiagnostic{
				Severity: SourceDiagnosticSeverityError, Code: "invalid-document", Message: "A document root must be an object.",
				Location: &SourceDiagnosticLocation{JSONPointer: util.ToPtr("")},
			})
			data, err := format.encode(original)
			require.NoError(t, err)
			var decoded ToolSet
			require.NoError(t, format.decode(data, &decoded))
			require.Equal(t, original.Status, decoded.Status)
			require.NoError(t, decoded.ValidateFor(meta.ValidationModeResponse, nil))
			for _, input := range []string{
				`{"severity":"warning","code":"x","message":"m","unknown":1}`,
				`{"severity":"warning","code":"x","message":"m","location":{"unknown":1}}`,
				`{"severity":"warning","code":{},"message":"m"}`,
			} {
				var diagnostic SourceDiagnostic
				require.Error(t, format.decode([]byte(input), &diagnostic), input)
			}
			invalidEntries := []string{`{}`, `{"severity":"error","code":null,"message":"m"}`}
			if format.name == "JSON" {
				// YAML's ordinary decoder omits null struct-slice entries;
				// JSON retains a zero value for required-field validation.
				invalidEntries = append(invalidEntries, `null`)
			}
			for _, input := range invalidEntries {
				var status ToolSetStatus
				require.NoError(t, format.decode([]byte(`{"release":{"state":"draft"},"importDiagnostics":[`+input+`]}`), &status))
				invalid := sourceDiagnosticResourceForTest()
				invalid.Status = &status
				require.ErrorContains(t, invalid.ValidateFor(meta.ValidationModeResponse, nil), "status.importDiagnostics[0].code", input)
			}
			var definition ToolSetDefinition
			require.Error(t, format.decode([]byte(`{"source":{"explicit":{"tools":[]}},"importDiagnostics":[]}`), &definition))
			var patch ToolSetPatch
			require.ErrorContains(t, format.decode([]byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"ToolSet","metadata":{},"spec":{},"status":{"importDiagnostics":[]}}`), &patch), "status")
		})
	}
}

// TestSourceDiagnosticOwnership requires generation observations to describe
// OpenAPI imports and leaves MCP catalogs on their connection bindings.
func TestSourceDiagnosticOwnership(t *testing.T) {
	resource := sourceDiagnosticResourceForTest()
	for _, mode := range []meta.ValidationMode{meta.ValidationModePersistence, meta.ValidationModeResponse} {
		require.NoError(t, resource.ValidateFor(mode, nil))
	}
	for _, mode := range []meta.ValidationMode{meta.ValidationModeCreate, meta.ValidationModeConfig, meta.ValidationModeUpdate} {
		require.ErrorContains(t, resource.ValidateFor(mode, nil), "status")
	}
	for _, definition := range []*ToolSetDefinition{explicitDefinitionForTest(), importedDefinitionForTest()} {
		resource.Spec.Definition = *definition
		require.ErrorContains(t, resource.ValidateFor(meta.ValidationModePersistence, nil), "status.importDiagnostics")
		for _, diagnostics := range [][]SourceDiagnostic{nil, {}} {
			resource.Status.ImportDiagnostics = diagnostics
			require.NoError(t, resource.ValidateFor(meta.ValidationModePersistence, nil))
		}
		resource.Status.ImportDiagnostics = []SourceDiagnostic{sourceDiagnosticForTest()}
	}
}

// TestSourceDiagnosticCloneOwnership prevents nested observation edits from
// mutating a retained resource or even an invalid programmatic patch.
func TestSourceDiagnosticCloneOwnership(t *testing.T) {
	require.Nil(t, (*SourceDiagnostic)(nil).Clone())
	require.Nil(t, (*ToolSetStatus)(nil).Clone())
	for _, diagnostics := range [][]SourceDiagnostic{nil, {}} {
		status := &ToolSetStatus{ImportDiagnostics: diagnostics}
		require.Equal(t, status, status.Clone(), "preserve absent and explicit empty lists")
	}
	original := sourceDiagnosticResourceForTest()
	patch := NewToolSetPatch()
	patch.Status = original.Status
	require.ErrorContains(t, patch.ValidateFor(meta.ValidationModeUpdate, nil), "status")
	for _, copied := range []*ToolSetStatus{original.Status.Clone(), original.Clone().Status, patch.Clone().Status} {
		require.Equal(t, original.Status, copied)
		copied.ImportDiagnostics[0].Code = "changed"
		copied.ImportDiagnostics[0].Location.DocumentURI = "urn:changed"
		*copied.ImportDiagnostics[0].Location.JSONPointer = ""
		require.Equal(t, sourceDiagnosticForTest(), original.Status.ImportDiagnostics[0])
	}
}

// TestSourceDiagnosticPatchReplacement retains detached observations for the
// service to clear when a definition changes to a different source family.
func TestSourceDiagnosticPatchReplacement(t *testing.T) {
	current := sourceDiagnosticResourceForTest()
	patch := decodeToolSetPatch(t, `{"definition":{"source":{"explicit":{"tools":[]}}}}`)
	for _, encode := range []func(any) ([]byte, error){json.Marshal, yaml.Marshal} {
		data, err := encode(patch)
		require.NoError(t, err)
		var decoded ToolSetPatch
		require.NoError(t, util.DecodeYAMLStrict(data, &decoded))
		candidate, err := decoded.ApplyTo(current, nil)
		require.NoError(t, err)
		require.Equal(t, current.Status, candidate.Status)
		require.NotSame(t, current.Status.ImportDiagnostics[0].Location.JSONPointer, candidate.Status.ImportDiagnostics[0].Location.JSONPointer)
		require.ErrorContains(t, candidate.ValidateFor(meta.ValidationModePersistence, nil), "status.importDiagnostics")
		candidate.Status.ImportDiagnostics = nil
		require.NoError(t, candidate.ValidateFor(meta.ValidationModePersistence, nil))
		require.ErrorContains(t, ValidateUpdate(current, candidate, nil), "status")
		require.Len(t, current.Status.ImportDiagnostics, 1)
	}
}

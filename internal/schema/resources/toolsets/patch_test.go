package toolsets

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// toolSetPatchDocument wraps a spec in the canonical required update envelope.
func toolSetPatchDocument(spec string) []byte {
	return []byte(fmt.Sprintf(`{"apiVersion":%q,"kind":"ToolSet","metadata":{},"spec":%s}`, meta.APIVersionV1Alpha1, spec))
}

// decodeToolSetPatch uses the strict API boundary while keeping cases readable.
func decodeToolSetPatch(t *testing.T, spec string) *ToolSetPatch {
	t.Helper()
	var patch ToolSetPatch
	require.NoError(t, util.DecodeJSONStrict(toolSetPatchDocument(spec), &patch))
	return &patch
}

// TestToolSetPatchOmissionAndReleaseIntent distinguishes an empty release object
// from publication intent and verifies observed status remains service-owned.
func TestToolSetPatchOmissionAndReleaseIntent(t *testing.T) {
	current := storedToolSetForResourceTest()
	for _, patch := range []*ToolSetPatch{NewToolSetPatch(), decodeToolSetPatch(t, `{"release":{}}`)} {
		candidate, err := patch.ApplyTo(current, nil)
		require.NoError(t, err)
		require.Equal(t, current, candidate)
		require.NotSame(t, current, candidate)
		require.NotSame(t, current.Status, candidate.Status)
		require.NotSame(t, current.Spec.ConnectionSelector, candidate.Spec.ConnectionSelector)
	}
	patch := decodeToolSetPatch(t, `{"release":{"desiredState":"primary"}}`)
	candidate, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	require.Equal(t, ToolSetReleaseStatePrimary, candidate.Spec.Release.DesiredState)
	require.Equal(t, ToolSetReleaseStateDraft, candidate.Status.Release.State)
	require.Equal(t, ToolSetReleaseStateDraft, current.Spec.Release.DesiredState)
	require.ErrorContains(t, candidate.ValidateFor(meta.ValidationModePersistence, nil), "status.release.state",
		"the service must publish before this candidate becomes a stored snapshot")

	// Merging itself does not apply endpoint-specific draft restrictions. A
	// logical update may select or create a draft after preparing this candidate.
	current.Spec.Release.DesiredState = ToolSetReleaseStatePrimary
	current.Status.Release.State = ToolSetReleaseStateArchived
	patch.Spec.Release.DesiredState = util.ToPtr(ToolSetReleaseStateDraft)
	candidate, err = patch.ApplyTo(current, nil)
	require.NoError(t, err)
	require.Equal(t, ToolSetReleaseStateArchived, candidate.Status.Release.State)
}

// TestToolSetPatchSelectorReplacement prevents old selector labels or a narrow
// scope from being silently retained when a complete replacement is provided.
func TestToolSetPatchSelectorReplacement(t *testing.T) {
	current := storedToolSetForResourceTest()
	current.Spec.ConnectionSelector.Namespace = "root.product.child"
	patch := decodeToolSetPatch(t, `{"connectionSelector":{"matchLabels":{"new":"replacement"}}}`)
	require.NoError(t, patch.ValidateFor(meta.ValidationModeUpdate, nil))
	candidate, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	require.Empty(t, candidate.Spec.ConnectionSelector.Namespace)
	ns, labels, err := candidate.Spec.ConnectionSelector.Compile(candidate.Metadata.Namespace)
	require.NoError(t, err)
	require.Equal(t, "root.product.**", ns)
	require.Equal(t, "new=replacement", labels)
	require.Equal(t, "root.product.child", current.Spec.ConnectionSelector.Namespace)

	patch = decodeToolSetPatch(t, `{"connectionSelector":{"namespace":"root.product.other.**","matchLabels":{}}}`)
	candidate, err = patch.ApplyTo(current, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]string{}, candidate.Spec.ConnectionSelector.MatchLabels)
	require.Equal(t, "root.product.other.**", candidate.Spec.ConnectionSelector.Namespace)

	for _, scope := range []string{"root.**", "root.sibling", "root.product-other.**"} {
		patch = decodeToolSetPatch(t, fmt.Sprintf(`{"connectionSelector":{"namespace":%q,"matchLabels":{}}}`, scope))
		require.NoError(t, patch.ValidateFor(meta.ValidationModeUpdate, nil), "root-only shape checking has no current resource")
		candidate, err = patch.ApplyTo(current, &common.ValidationContext{Path: "candidate"})
		require.ErrorContains(t, err, "candidate.spec.connectionSelector.namespace")
		require.Nil(t, candidate)
	}
}

// TestToolSetPatchCompleteDefinitionReplacement permits an explicit empty
// inventory but rejects incomplete replacements instead of retaining old parts.
func TestToolSetPatchCompleteDefinitionReplacement(t *testing.T) {
	current := storedToolSetForResourceTest()
	patch := decodeToolSetPatch(t, `{"definition":{"source":{"explicit":{"tools":[]}}}}`)
	candidate, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	require.NotNil(t, candidate.Spec.Definition.Source.Explicit.Tools)
	require.Empty(t, candidate.Spec.Definition.Source.Explicit.Tools)
	require.Len(t, current.Spec.Definition.Source.Explicit.Tools, 1)

	for _, spec := range []string{
		`{"definition":{}}`, `{"definition":{"source":{}}}`,
		`{"definition":{"source":{"explicit":{}}}}`,
		`{"definition":{"source":{"explicit":{"tools":null}}}}`,
		`{"definition":{"source":{"explicit":{"tools":[{"key":"replacement","spec":{}}]}}}}`,
	} {
		patch := decodeToolSetPatch(t, spec)
		require.ErrorContains(t, patch.ValidateFor(meta.ValidationModeUpdate, nil), "spec.definition.source")
		candidate, err := patch.ApplyTo(current, nil)
		require.Error(t, err)
		require.Nil(t, candidate)
	}
}

// TestToolSetPatchNullPresence checks JSON, aliases, and merge keys together;
// invalid nulls must survive reserialization and cloning for later validation.
func TestToolSetPatchNullPresence(t *testing.T) {
	for _, test := range []struct {
		name string
		spec string
		yaml string
		path string
	}{
		{"selector", `{"connectionSelector":null}`, "<<: {connectionSelector: &clear null}\n  connectionSelector: *clear", "connectionSelector"},
		{"definition", `{"definition":null}`, "<<: {definition: &clear null}\n  definition: *clear", "definition"},
		{"release", `{"release":null}`, "<<: {release: &clear null}\n  release: *clear", "release"},
		{"desired state", `{"release":{"desiredState":null}}`, "release:\n    <<: {desiredState: &clear null}\n    desiredState: *clear", "release.desiredState"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, input := range [][]byte{
				toolSetPatchDocument(test.spec),
				[]byte(fmt.Sprintf("apiVersion: %s\nkind: ToolSet\nmetadata: {}\nspec:\n  %s\n", meta.APIVersionV1Alpha1, test.yaml)),
				[]byte(fmt.Sprintf("apiVersion: %s\nkind: ToolSet\nmetadata: {}\nspec:\n  <<: %s\n", meta.APIVersionV1Alpha1, test.spec)),
			} {
				var patch ToolSetPatch
				require.NoError(t, util.DecodeYAMLStrict(input, &patch))
				// Also exercise direct JSON rather than only YAML's JSON subset.
				var fromJSON ToolSetPatch
				require.NoError(t, util.DecodeJSONStrict(toolSetPatchDocument(test.spec), &fromJSON))
				require.Equal(t, fromJSON, patch)
				for _, value := range []*ToolSetPatch{&patch, patch.Clone()} {
					require.ErrorContains(t, value.ValidateFor(meta.ValidationModeUpdate, &common.ValidationContext{Path: "request"}), "request.spec."+test.path)
					candidate, err := value.ApplyTo(storedToolSetForResourceTest(), nil)
					require.ErrorContains(t, err, "must not be null")
					require.Nil(t, candidate)
					raw, err := json.Marshal(value.Spec)
					require.NoError(t, err)
					require.JSONEq(t, test.spec, string(raw))
					emitted, err := yaml.Marshal(value)
					require.NoError(t, err)
					var roundTrip ToolSetPatch
					require.NoError(t, util.DecodeYAMLStrict(emitted, &roundTrip))
					require.Equal(t, value, &roundTrip)
				}
			}
		})
	}
}

// TestToolSetPatchValidationAndStrictDecoding rejects malformed envelopes,
// unsupported fields, null map entries, and invalid complete merged state.
func TestToolSetPatchValidationAndStrictDecoding(t *testing.T) {
	for _, input := range []string{
		fmt.Sprintf(`{"apiVersion":%q,"kind":"ToolSet","spec":{}}`, meta.APIVersionV1Alpha1),
		fmt.Sprintf(`{"apiVersion":%q,"kind":"ToolSet","metadata":null,"spec":{}}`, meta.APIVersionV1Alpha1),
		fmt.Sprintf(`{"apiVersion":%q,"kind":"ToolSet","metadata":{},"spec":null}`, meta.APIVersionV1Alpha1),
		`{"metadata":{},"spec":{}}`,
	} {
		var patch ToolSetPatch
		require.NoError(t, util.DecodeJSONStrict([]byte(input), &patch))
		require.Error(t, patch.ValidateFor(meta.ValidationModeUpdate, nil))
	}
	for _, spec := range []string{
		`{"unknown":1}`, `{"Definition":{}}`, `{"release":{"DesiredState":"draft"}}`,
		`{"release":{"state":"draft"}}`, `{"definition":{"unknown":1}}`,
		`{"connectionSelector":{"matchExpressions":[]}}`,
		`{"connectionSelector":{"namespace":null,"matchLabels":{}}}`,
		`{"connectionSelector":{"matchLabels":{"key":null}}}`,
	} {
		for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
			var patch ToolSetPatch
			require.Error(t, decode(toolSetPatchDocument(spec), &patch), spec)
		}
	}
	for _, test := range []struct{ spec, path string }{
		{`{"release":{"desiredState":""}}`, "release.desiredState"},
		{`{"release":{"desiredState":"active"}}`, "release.desiredState"},
		{`{"release":{"desiredState":"archived"}}`, "release.desiredState"},
		{`{"connectionSelector":{}}`, "connectionSelector.matchLabels"},
		{`{"connectionSelector":{"matchLabels":null}}`, "connectionSelector.matchLabels"},
		{`{"connectionSelector":{"namespace":"bad","matchLabels":{}}}`, "connectionSelector.namespace"},
	} {
		require.ErrorContains(t, decodeToolSetPatch(t, test.spec).ValidateFor(meta.ValidationModeUpdate, nil), "spec."+test.path)
	}
	for _, change := range []func(*ToolSet){
		func(v *ToolSet) { v.Spec.ConnectionSelector = nil },
		func(v *ToolSet) { v.Spec.Definition.Source.Explicit = nil },
		func(v *ToolSet) { v.Spec.Release.DesiredState = "unknown" },
	} {
		current := storedToolSetForResourceTest()
		change(current)
		candidate, err := NewToolSetPatch().ApplyTo(current, nil)
		require.Error(t, err)
		require.Nil(t, candidate)
	}
	var absent *ToolSetPatch
	require.Error(t, absent.ValidateFor(meta.ValidationModeUpdate, nil))
	_, err := absent.ApplyTo(storedToolSetForResourceTest(), nil)
	require.Error(t, err)
	_, err = NewToolSetPatch().ApplyTo(nil, nil)
	require.Error(t, err)
	require.Error(t, NewToolSetPatch().ValidateFor(meta.ValidationModeCreate, nil))
}

// TestToolSetPatchRejectsServerOwnedPresence keeps null status and timestamps
// from disappearing through pointer decoding, including YAML aliases and merges.
func TestToolSetPatchRejectsServerOwnedPresence(t *testing.T) {
	for _, field := range []string{"status", "createdAt", "updatedAt"} {
		for _, value := range []string{"null", "{}", `"2026-10-08T12:00:00Z"`} {
			metadata, status := `{}`, ""
			if field == "status" {
				status = fmt.Sprintf(`,"status":%s`, value)
			} else {
				metadata = fmt.Sprintf(`{%q:%s}`, field, value)
			}
			input := []byte(fmt.Sprintf(`{"apiVersion":%q,"kind":"ToolSet","metadata":%s,"spec":{}%s}`, meta.APIVersionV1Alpha1, metadata, status))
			for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
				var patch ToolSetPatch
				require.ErrorContains(t, decode(input, &patch), field)
			}
		}
	}
	for _, metadata := range []string{`{"CreatedAt":null}`, `{"UPDATEDAT":null}`, `{"Labels":{}}`} {
		var patch ToolSetPatch
		input := []byte(fmt.Sprintf(`{"apiVersion":%q,"kind":"ToolSet","metadata":%s,"spec":{}}`, meta.APIVersionV1Alpha1, metadata))
		require.ErrorContains(t, util.DecodeJSONStrict(input, &patch), "unknown tool set metadata patch field")
		require.ErrorContains(t, util.DecodeYAMLStrict(input, &patch), "unknown tool set metadata patch field")
	}
	for _, body := range []string{
		"metadata: {}\nspec: {}\n<<: {status: null}",
		"metadata: {labels: &clear null}\nspec: {}\nstatus: *clear",
		"metadata:\n  <<: {createdAt: null}\nspec: {}",
		"metadata:\n  labels: &clear null\n  updatedAt: *clear\nspec: {}",
	} {
		var patch ToolSetPatch
		input := []byte(fmt.Sprintf("apiVersion: %s\nkind: ToolSet\n%s\n", meta.APIVersionV1Alpha1, body))
		require.ErrorContains(t, util.DecodeYAMLStrict(input, &patch), "server-owned")
	}
	patch := NewToolSetPatch()
	before := patch.Clone()
	require.Error(t, json.Unmarshal([]byte(`{"status":null}`), patch))
	require.Equal(t, before, patch, "failed envelope decoding is atomic")
	require.Error(t, json.Unmarshal([]byte(`null`), patch))
}

// TestToolSetPatchMetadataAndIdentity follows shared whole-map patch semantics
// and rejects identity or server-owned writes even when the value is unchanged.
func TestToolSetPatchMetadataAndIdentity(t *testing.T) {
	for _, test := range []struct {
		metadata string
		want     map[string]string
	}{
		{`{}`, map[string]string{"old": "preserved"}},
		{`{"labels":null,"annotations":null}`, map[string]string{"old": "preserved"}},
		{`{"labels":{},"annotations":{}}`, map[string]string{}},
		{`{"labels":{"new":"value"},"annotations":{"new":"value"}}`, map[string]string{"new": "value"}},
	} {
		current := storedToolSetForResourceTest()
		current.Metadata.Labels = map[string]string{"old": "preserved"}
		current.Metadata.Annotations = map[string]string{"old": "preserved"}
		var patch ToolSetPatch
		require.NoError(t, util.DecodeJSONStrict([]byte(fmt.Sprintf(`{"apiVersion":%q,"kind":"ToolSet","metadata":%s,"spec":{}}`, meta.APIVersionV1Alpha1, test.metadata)), &patch))
		candidate, err := patch.ApplyTo(current, nil)
		require.NoError(t, err)
		require.Equal(t, test.want, candidate.Metadata.Labels)
		require.Equal(t, test.want, candidate.Metadata.Annotations)
	}
	current := storedToolSetForResourceTest()
	stamp := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	current.Metadata.CreatedAt, current.Metadata.UpdatedAt = &stamp, &stamp
	for _, test := range []struct {
		name string
		edit func(*ToolSetPatch)
	}{
		{"id", func(p *ToolSetPatch) { p.Metadata.ID = util.ToPtr("tls_01example0000002") }},
		{"namespace", func(p *ToolSetPatch) { p.Metadata.Namespace = util.ToPtr("root.other") }},
		{"generation", func(p *ToolSetPatch) { p.Metadata.Generation = util.ToPtr(uint64(4)) }},
		{"zero generation", func(p *ToolSetPatch) { p.Metadata.Generation = util.ToPtr(uint64(0)) }},
		{"createdAt", func(p *ToolSetPatch) { p.Metadata.CreatedAt = &stamp }},
		{"updatedAt", func(p *ToolSetPatch) { p.Metadata.UpdatedAt = &stamp }},
		{"empty status", func(p *ToolSetPatch) { p.Status = &ToolSetStatus{} }},
		{"unchanged status", func(p *ToolSetPatch) { p.Status = current.Status }},
	} {
		t.Run(test.name, func(t *testing.T) {
			patch := NewToolSetPatch()
			test.edit(patch)
			candidate, err := patch.ApplyTo(current, nil)
			require.Error(t, err)
			require.Nil(t, candidate)
		})
	}
	patch := NewToolSetPatch()
	patch.Metadata.ID, patch.Metadata.Namespace = &current.Metadata.ID, &current.Metadata.Namespace
	patch.Metadata.Generation = &current.Metadata.Generation
	patch.Metadata.Name = util.ToPtr(common.ResourceName("new-name"))
	candidate, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	require.Equal(t, common.ResourceName("new-name"), candidate.Metadata.Name)
	require.Equal(t, current.Metadata.Generation, candidate.Metadata.Generation)

	for _, change := range []func(*ToolSet){
		func(v *ToolSet) { v.Kind = "Tool" },
		func(v *ToolSet) { v.APIVersion = "authproxy.net/v2" },
		func(v *ToolSet) { v.Metadata.CreatedAt = nil },
		func(v *ToolSet) { v.Metadata.UpdatedAt = nil },
		func(v *ToolSet) { v.Status = nil },
		func(v *ToolSet) { v.Status.Release.State = ToolSetReleaseStatePrimary },
	} {
		candidate := current.Clone()
		change(candidate)
		require.Error(t, ValidateUpdate(current, candidate, nil))
	}
	require.Error(t, ValidateUpdate(nil, current, nil))
	require.Error(t, ValidateUpdate(current, nil, nil))
}

// TestToolSetPatchCandidateOwnership exercises supplied and inherited mutable
// state so neither an admitted snapshot nor the authored patch can be rewritten.
func TestToolSetPatchCandidateOwnership(t *testing.T) {
	current := storedToolSetForResourceTest()
	stamp := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	current.Metadata.CreatedAt, current.Metadata.UpdatedAt = &stamp, &stamp
	current.Metadata.Annotations = map[string]string{"old": "inherited"}
	patch := NewToolSetPatch()
	patch.Metadata.Labels = util.ToPtr(map[string]string{"new": "replacement"})
	patch.Spec.ConnectionSelector = &ConnectionSelector{MatchLabels: map[string]string{"new": "selected"}}
	patch.Spec.Definition = current.Spec.Definition.Clone()
	beforeCurrent, beforePatch := current.Clone(), patch.Clone()
	candidate, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	candidate.Metadata.Labels["new"] = "changed"
	candidate.Metadata.Annotations["old"] = "changed"
	*candidate.Metadata.CreatedAt = stamp.Add(time.Hour)
	*candidate.Metadata.UpdatedAt = stamp.Add(time.Hour)
	candidate.Spec.ConnectionSelector.MatchLabels["new"] = "changed"
	candidate.Status.Release.State = ToolSetReleaseStatePrimary
	template := &candidate.Spec.Definition.Source.Explicit.Tools[0]
	template.Key = "changed"
	template.Metadata.Labels["capability"] = "changed"
	template.Metadata.Annotations["description"] = "changed"
	template.Spec.Verbs[0] = "tool:changed"
	template.Spec.InputSchema[0] = '['
	template.Spec.ProxyHTTP.Headers["Accept"] = "changed"
	template.Spec.ProxyHTTP.Query["limit"][0] = '2'
	require.Equal(t, beforeCurrent, current)
	require.Equal(t, beforePatch, patch)
}

// TestToolSetPatchClone preserves every pointer and invalid presence flag while
// detaching the result, including fields that normal authoring must reject.
func TestToolSetPatchClone(t *testing.T) {
	require.Nil(t, (*ToolSetPatch)(nil).Clone())
	require.Equal(t, &ToolSetPatch{}, (&ToolSetPatch{}).Clone())
	current := storedToolSetForResourceTest()
	patch := NewToolSetPatch()
	stamp := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	patch.Metadata = &meta.ObjectMetaPatch{
		ID: &current.Metadata.ID, Name: &current.Metadata.Name,
		Namespace: &current.Metadata.Namespace, Generation: &current.Metadata.Generation,
		Labels: util.ToPtr(map[string]string{"key": "label"}), Annotations: util.ToPtr(map[string]string{"key": "annotation"}),
		CreatedAt: &stamp, UpdatedAt: &stamp,
	}
	patch.Status = current.Status
	patch.Spec.ConnectionSelector = current.Spec.ConnectionSelector
	patch.Spec.Definition = &current.Spec.Definition
	patch.Spec.Release = &ToolSetReleaseSpecPatch{DesiredState: &current.Spec.Release.DesiredState}
	before, err := json.Marshal(patch)
	require.NoError(t, err)
	clone := patch.Clone()
	require.Equal(t, patch, clone)
	*clone.Metadata.ID = "changed"
	*clone.Metadata.Name = "changed"
	*clone.Metadata.Namespace = "root.changed"
	*clone.Metadata.Generation = 99
	(*clone.Metadata.Labels)["key"] = "changed"
	(*clone.Metadata.Annotations)["key"] = "changed"
	*clone.Metadata.CreatedAt = stamp.Add(time.Hour)
	*clone.Metadata.UpdatedAt = stamp.Add(time.Hour)
	clone.Status.Release.State = ToolSetReleaseStatePrimary
	clone.Spec.ConnectionSelector.MatchLabels["provider"] = "changed"
	clone.Spec.Definition.Source.Explicit.Tools[0].Spec.InputSchema[0] = '['
	*clone.Spec.Release.DesiredState = ToolSetReleaseStatePrimary
	after, err := json.Marshal(patch)
	require.NoError(t, err)
	require.Equal(t, before, after)

	for _, value := range []map[string]string{nil, {}} {
		patch.Metadata.Labels, patch.Metadata.Annotations = &value, &value
		clone := patch.Clone()
		require.NotNil(t, clone.Metadata.Labels)
		require.Equal(t, value, *clone.Metadata.Labels)
		require.Equal(t, value, *clone.Metadata.Annotations)
	}
}

// TestToolSetPatchDecodeReset keeps failed decodes atomic and successful reused
// decodes free of stale null-presence flags, including nested release state.
func TestToolSetPatchDecodeReset(t *testing.T) {
	patch := decodeToolSetPatch(t, `{"connectionSelector":null,"definition":null,"release":{"desiredState":null}}`)
	require.True(t, patch.Spec.HasConnectionSelector())
	require.True(t, patch.Spec.HasDefinition())
	require.True(t, patch.Spec.HasRelease())
	require.True(t, patch.Spec.Release.HasDesiredState())
	before := patch.Clone()
	require.Error(t, json.Unmarshal([]byte(`{"unknown":{}}`), patch.Spec))
	require.Equal(t, before, patch)
	require.Error(t, json.Unmarshal([]byte(`null`), patch.Spec))
	require.Error(t, json.Unmarshal([]byte(`{"unknown":"draft"}`), patch.Spec.Release))
	require.Equal(t, before, patch)
	require.NoError(t, json.Unmarshal([]byte(`{}`), patch.Spec.Release))
	require.False(t, patch.Spec.Release.HasDesiredState())
	require.NoError(t, json.Unmarshal([]byte(`{}`), patch.Spec))
	require.False(t, patch.Spec.HasConnectionSelector())
	require.False(t, patch.Spec.HasDefinition())
	require.False(t, patch.Spec.HasRelease())
	require.False(t, (*ToolSetSpecPatch)(nil).HasDefinition())
	require.False(t, (*ToolSetSpecPatch)(nil).HasRelease())
	require.False(t, (*ToolSetSpecPatch)(nil).HasConnectionSelector())
	require.False(t, (*ToolSetReleaseSpecPatch)(nil).HasDesiredState())
	require.Error(t, json.Unmarshal([]byte(`null`), &ToolSetReleaseSpecPatch{}))
}

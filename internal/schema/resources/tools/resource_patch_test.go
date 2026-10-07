package tools

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// toolPatchDocument wraps one spec patch in the required update envelope.
func toolPatchDocument(spec string) []byte {
	return []byte(fmt.Sprintf(`{"apiVersion":%q,"kind":"Tool","metadata":{},"spec":%s}`, meta.APIVersionV1Alpha1, spec))
}

// decodeToolPatch reads JSON through the same strict boundary as API callers.
func decodeToolPatch(t *testing.T, spec string) *ToolPatch {
	t.Helper()
	var patch ToolPatch
	require.NoError(t, util.DecodeJSONStrict(toolPatchDocument(spec), &patch))
	return &patch
}

// TestToolPatchOmissionPreservesCurrentAndStatus verifies that an empty spec
// changes neither the current definition nor its server-owned revision.
func TestToolPatchOmissionPreservesCurrentAndStatus(t *testing.T) {
	current := storedToolForTest()
	patch := NewToolPatch()
	require.Equal(t, decodeToolPatch(t, `{}`).TypeMeta, patch.TypeMeta)
	candidate, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	require.Equal(t, current, candidate)
	require.NotSame(t, current, candidate)
	require.NotSame(t, current.Status, candidate.Status)
	require.NotSame(t, current.Spec.ProxyHTTP, candidate.Spec.ProxyHTTP)
}

// TestToolPatchReplacesWholeFields prevents patching from accidentally merging
// schema properties, executor fields, hints, or execution-limit subfields.
func TestToolPatchReplacesWholeFields(t *testing.T) {
	current := storedToolForTest()
	current.Spec.InputSchema = common.RawJSON(`{"type":"object","properties":{"old":{"type":"string"}}}`)
	current.Spec.OutputSchema = common.RawJSON(`{"type":"object","properties":{"old":{"type":"string"}}}`)
	readOnly, destructive := true, true
	current.Spec.Hints = &BehavioralHints{ReadOnly: &readOnly, Destructive: &destructive}
	maxRequests, timeout := 4, int64(5000)
	current.Spec.Limits = &ExecutionLimits{MaxRequests: &maxRequests, TimeoutMillis: &timeout}
	current.Spec.ProxyHTTP.Headers = map[string]string{"Old": "removed"}
	patch := decodeToolPatch(t, `{
		"description":"Replacement","verbs":["tool:replacement"],
		"inputSchema":{"type":"object","properties":{"new":{"type":"integer"}}},
		"outputSchema":false,"hints":{"readOnly":false},"limits":{"maxRequests":1},
		"proxyHttp":{"method":"POST","url":"https://example.test/replacement","bodyJson":null}
	}`)
	candidate, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	assert.Equal(t, "Replacement", candidate.Spec.Description)
	assert.Equal(t, []string{"tool:replacement"}, candidate.Spec.Verbs)
	assert.JSONEq(t, string(patch.Spec.InputSchema), string(candidate.Spec.InputSchema))
	assert.Equal(t, common.RawJSON(`false`), candidate.Spec.OutputSchema)
	require.NotNil(t, candidate.Spec.Hints.ReadOnly)
	assert.False(t, *candidate.Spec.Hints.ReadOnly)
	assert.Nil(t, candidate.Spec.Hints.Destructive)
	assert.Equal(t, 1, *candidate.Spec.Limits.MaxRequests)
	assert.Nil(t, candidate.Spec.Limits.TimeoutMillis)
	assert.Nil(t, candidate.Spec.ProxyHTTP.Headers)
	assert.Equal(t, common.RawJSON(`null`), candidate.Spec.ProxyHTTP.BodyJSON)
	assert.Equal(t, current.Status, candidate.Status)
	assert.Equal(t, "removed", current.Spec.ProxyHTTP.Headers["Old"])
}

// TestToolPatchMetadataMaps follows the shared metadata patch convention:
// omitted/null maps preserve state, empty maps clear it, and nonempty maps
// replace the whole value rather than merging individual entries.
func TestToolPatchMetadataMaps(t *testing.T) {
	for _, test := range []struct {
		name     string
		metadata string
		want     map[string]string
	}{
		{"omitted", `{}`, map[string]string{"old": "preserved"}},
		{"null", `{"labels":null,"annotations":null}`, map[string]string{"old": "preserved"}},
		{"empty", `{"labels":{},"annotations":{}}`, map[string]string{}},
		{"replacement", `{"labels":{"new":"replacement"},"annotations":{"new":"replacement"}}`, map[string]string{"new": "replacement"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
				current := storedToolForTest()
				current.Metadata.Labels = map[string]string{"old": "preserved"}
				current.Metadata.Annotations = map[string]string{"old": "preserved"}
				input := []byte(fmt.Sprintf(`{"apiVersion":%q,"kind":"Tool","metadata":%s,"spec":{}}`, meta.APIVersionV1Alpha1, test.metadata))
				var patch ToolPatch
				require.NoError(t, decode(input, &patch))
				candidate, err := patch.ApplyTo(current, nil)
				require.NoError(t, err)
				assert.Equal(t, test.want, candidate.Metadata.Labels)
				assert.Equal(t, test.want, candidate.Metadata.Annotations)
				assert.Equal(t, map[string]string{"old": "preserved"}, current.Metadata.Labels)
				assert.Equal(t, map[string]string{"old": "preserved"}, current.Metadata.Annotations)
			}
		})
	}
}

// TestToolPatchNullClearsOptionalFields covers explicit clearing, including
// YAML aliases and merges, without converting a schema's false into omission.
func TestToolPatchNullClearsOptionalFields(t *testing.T) {
	for _, format := range []string{"JSON", "YAML aliases and merges"} {
		t.Run(format, func(t *testing.T) {
			current := storedToolForTest()
			current.Spec.OutputSchema = common.RawJSON(`false`)
			readOnly, count := false, 2
			current.Spec.Hints = &BehavioralHints{ReadOnly: &readOnly}
			current.Spec.Limits = &ExecutionLimits{MaxRequests: &count}
			var patch ToolPatch
			if format == "JSON" {
				require.NoError(t, util.DecodeJSONStrict(toolPatchDocument(`{"outputSchema":null,"hints":null,"limits":null}`), &patch))
			} else {
				input := fmt.Sprintf("apiVersion: %s\nkind: Tool\nmetadata: {}\nspec:\n  <<: &clears\n    outputSchema: &clear null\n    hints: *clear\n  limits: *clear\n", meta.APIVersionV1Alpha1)
				require.NoError(t, util.DecodeYAMLStrict([]byte(input), &patch))
			}
			candidate, err := patch.ApplyTo(current, nil)
			require.NoError(t, err)
			assert.Nil(t, candidate.Spec.OutputSchema)
			assert.Nil(t, candidate.Spec.Hints)
			assert.Nil(t, candidate.Spec.Limits)
			assert.Equal(t, common.RawJSON(`false`), current.Spec.OutputSchema)
			encoded, err := json.Marshal(patch.Spec)
			require.NoError(t, err)
			require.JSONEq(t, `{"outputSchema":null,"hints":null,"limits":null}`, string(encoded))
			asYAML, err := yaml.Marshal(patch)
			require.NoError(t, err)
			var roundTrip ToolPatch
			require.NoError(t, util.DecodeYAMLStrict(asYAML, &roundTrip))
			again, err := roundTrip.ApplyTo(current, nil)
			require.NoError(t, err)
			assert.Equal(t, candidate, again)
		})
	}
}

// TestToolPatchRequiredFieldsCannotBeCleared rejects null after both JSON and
// YAML alias resolution rather than silently treating it as an omitted field.
func TestToolPatchRequiredFieldsCannotBeCleared(t *testing.T) {
	for _, field := range []string{"connectionRef", "description", "verbs", "inputSchema"} {
		t.Run(field, func(t *testing.T) {
			for _, format := range []string{"JSON", "YAML alias"} {
				var patch ToolPatch
				if format == "JSON" {
					require.NoError(t, util.DecodeJSONStrict(toolPatchDocument(fmt.Sprintf(`{%q:null}`, field)), &patch))
				} else {
					input := fmt.Sprintf("apiVersion: %s\nkind: Tool\nmetadata: {}\nspec:\n  outputSchema: &clear null\n  %s: *clear\n", meta.APIVersionV1Alpha1, field)
					require.NoError(t, util.DecodeYAMLStrict([]byte(input), &patch))
				}
				require.ErrorContains(t, patch.ValidateFor(meta.ValidationModeUpdate, nil), "spec."+field)
				candidate, err := patch.ApplyTo(storedToolForTest(), nil)
				require.ErrorContains(t, err, "must not be null")
				assert.Nil(t, candidate)
			}
		})
	}
}

// TestToolPatchExecutorSwitchIsAtomic validates the complete merged union,
// allowing a replacement only when the old executor is cleared in that patch.
func TestToolPatchExecutorSwitchIsAtomic(t *testing.T) {
	current := storedToolForTest()
	patch := decodeToolPatch(t, `{"proxyHttp":null,"javascript":"async function execute(params, context) { return params; }"}`)
	candidate, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	assert.Nil(t, candidate.Spec.ProxyHTTP)
	require.NotNil(t, candidate.Spec.Javascript)
	assert.NotNil(t, current.Spec.ProxyHTTP)
	back := decodeToolPatch(t, `{"javascript":null,"proxyHttp":{"method":"GET","url":"https://example.test/back"}}`)
	restored, err := back.ApplyTo(candidate, nil)
	require.NoError(t, err)
	assert.Nil(t, restored.Spec.Javascript)
	assert.Equal(t, "https://example.test/back", restored.Spec.ProxyHTTP.URL)
	for _, spec := range []string{
		`{"javascript":"replacement"}`, `{"proxyHttp":null}`, `{"proxyHttp":null,"javascript":null}`,
		`{"proxyHttp":{"url":"https://example.test/incomplete"}}`,
	} {
		invalid := decodeToolPatch(t, spec)
		result, err := invalid.ApplyTo(current, nil)
		require.Error(t, err)
		assert.Nil(t, result)
	}
}

// TestToolPatchConnectionAndIdentity allows rebinding within one namespace
// while rejecting identity edits, server status writes, and managed updates.
func TestToolPatchConnectionAndIdentity(t *testing.T) {
	current := storedToolForTest()
	ref := current.Spec.ConnectionRef
	ref.ID = apid.New(apid.PrefixConnection).String()
	patch := decodeToolPatch(t, `{}`)
	patch.Spec.ConnectionRef = &ref
	candidate, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	assert.Equal(t, ref, candidate.Spec.ConnectionRef)
	assert.NotEqual(t, current.Spec.ConnectionRef.ID, candidate.Spec.ConnectionRef.ID)
	ref.Namespace = "root.other"
	_, err = patch.ApplyTo(current, nil)
	require.ErrorContains(t, err, "must equal the tool namespace")
	ref.Namespace = current.Metadata.Namespace

	for _, edit := range []func(*ToolPatch){
		func(p *ToolPatch) { id := apid.New(apid.PrefixTool).String(); p.Metadata.ID = &id },
		func(p *ToolPatch) { ns := "root.other"; p.Metadata.Namespace = &ns },
		func(p *ToolPatch) { generation := uint64(0); p.Metadata.Generation = &generation },
		func(p *ToolPatch) { generation := uint64(1); p.Metadata.Generation = &generation },
		func(p *ToolPatch) { p.Status = &ToolStatus{} },
		func(p *ToolPatch) { p.Status = current.Status },
		func(p *ToolPatch) { p.Status = &ToolStatus{Revision: current.Status.Revision + 1} },
		func(p *ToolPatch) { p.Metadata.UpdatedAt = current.Metadata.UpdatedAt },
	} {
		invalid := decodeToolPatch(t, `{}`)
		edit(invalid)
		candidate, err := invalid.ApplyTo(current, nil)
		require.Error(t, err)
		assert.Nil(t, candidate)
	}
	ownerID := apid.New(apid.PrefixToolSet).String()
	current.Status.ManagedBy = &ToolManagedBy{ToolSetRef: meta.ObjectReference{ID: ownerID}, SourceKey: "operation"}
	_, err = decodeToolPatch(t, `{}`).ApplyTo(current, nil)
	require.ErrorContains(t, err, ownerID)
}

// TestToolPatchCandidateDoesNotAliasInputs exercises replaced and inherited
// mutable fields together; changing the candidate cannot rewrite either input.
func TestToolPatchCandidateDoesNotAliasInputs(t *testing.T) {
	current := storedToolForTest()
	current.Metadata.Annotations = map[string]string{"inherited": "unchanged"}
	current.Spec.OutputSchema = common.RawJSON(`{"type":"string"}`)
	readOnly, count := true, 2
	current.Spec.Hints = &BehavioralHints{ReadOnly: &readOnly}
	current.Spec.Limits = &ExecutionLimits{MaxRequests: &count}
	patch := decodeToolPatch(t, `{
		"verbs":["tool:new"],"inputSchema":{"type":"object"},
		"proxyHttp":{"method":"POST","url":"https://example.test","headers":{"X-Test":"original"},
		"query":{"key":[1]},"bodyJson":{"value":1},
		"response":{"errors":[{"statuses":[404],"code":"MISSING","message":"Missing"}]}}
	}`)
	labels := map[string]string{"patched": "unchanged"}
	patch.Metadata.Labels = &labels
	beforeCurrent, err := json.Marshal(current)
	require.NoError(t, err)
	beforePatch, err := json.Marshal(patch)
	require.NoError(t, err)
	candidate, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	candidate.Metadata.Labels["patched"] = "changed"
	candidate.Metadata.Annotations["inherited"] = "changed"
	candidate.Spec.Verbs[0] = "changed"
	candidate.Spec.InputSchema[0] = '?'
	candidate.Spec.OutputSchema[0] = '?'
	*candidate.Spec.Hints.ReadOnly = false
	*candidate.Spec.Limits.MaxRequests = 10
	candidate.Spec.ProxyHTTP.Headers["X-Test"] = "changed"
	candidate.Spec.ProxyHTTP.Query["key"][0] = '?'
	candidate.Spec.ProxyHTTP.BodyJSON[0] = '?'
	candidate.Spec.ProxyHTTP.Response.Errors[0].Statuses[0] = 500
	candidate.Status.Conditions[0].Message = "changed"
	candidate.Status.Revision++
	afterCurrent, err := json.Marshal(current)
	require.NoError(t, err)
	afterPatch, err := json.Marshal(patch)
	require.NoError(t, err)
	assert.Equal(t, beforeCurrent, afterCurrent)
	assert.Equal(t, beforePatch, afterPatch)
}

// TestToolPatchEnvelopeAndMergedValidation keeps malformed update shapes and
// incomplete final definitions from producing a candidate.
func TestToolPatchEnvelopeAndMergedValidation(t *testing.T) {
	for _, input := range []string{
		string(toolPatchDocument(`null`)),
		fmt.Sprintf(`{"apiVersion":%q,"kind":"Tool","spec":{}}`, meta.APIVersionV1Alpha1),
		`{"metadata":{},"spec":{}}`,
	} {
		var patch ToolPatch
		require.NoError(t, util.DecodeJSONStrict([]byte(input), &patch))
		require.Error(t, patch.ValidateFor(meta.ValidationModeUpdate, nil))
	}
	for _, spec := range []string{`{"description":""}`, `{"verbs":[]}`, `{"inputSchema":{}}`, `{"limits":{"maxRequests":0}}`} {
		candidate, err := decodeToolPatch(t, spec).ApplyTo(storedToolForTest(), nil)
		require.Error(t, err)
		assert.Nil(t, candidate)
	}
	for _, spec := range []string{`{"unknown":1}`, `{"Description":"case mismatch"}`, `{"hints":{"unknown":true}}`, `{"proxyHttp":{"method":"GET","url":"x","unknown":1}}`} {
		var patch ToolPatch
		require.Error(t, util.DecodeJSONStrict(toolPatchDocument(spec), &patch))
		require.Error(t, util.DecodeYAMLStrict(toolPatchDocument(spec), &patch))
	}
	var absent *ToolPatch
	_, err := absent.ApplyTo(storedToolForTest(), nil)
	require.Error(t, err)
	_, err = decodeToolPatch(t, `{}`).ApplyTo(nil, nil)
	require.Error(t, err)
	require.Error(t, decodeToolPatch(t, `{}`).ValidateFor(meta.ValidationModeCreate, nil))
	for _, spec := range []*ToolSpecPatch{
		{Verbs: new([]string)},
		{InputSchema: common.RawJSON(`null`)},
	} {
		patch := NewToolPatch()
		patch.Spec = spec
		require.ErrorContains(t, patch.ValidateFor(meta.ValidationModeUpdate, nil), "must not be null")
	}
}

// TestToolSpecPatchGoClearsAndDecodeReset preserves programmatic clears in both
// encodings and prevents stale presence flags when a decoder reuses a value.
func TestToolSpecPatchGoClearsAndDecodeReset(t *testing.T) {
	var patch ToolSpecPatch
	patch.SetOutputSchema(nil)
	patch.SetHints(nil)
	patch.SetLimits(nil)
	patch.SetProxyHTTP(nil)
	patch.SetJavascript(nil)
	raw, err := json.Marshal(patch)
	require.NoError(t, err)
	require.JSONEq(t, `{"outputSchema":null,"hints":null,"limits":null,"proxyHttp":null,"javascript":null}`, string(raw))
	encoded, err := yaml.Marshal(patch)
	require.NoError(t, err)
	var roundTrip ToolSpecPatch
	require.NoError(t, util.DecodeYAMLStrict(encoded, &roundTrip))
	encodedJSON, err := json.Marshal(roundTrip)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(encodedJSON))
	require.NoError(t, json.Unmarshal([]byte(`{}`), &roundTrip))
	encodedJSON, err = json.Marshal(roundTrip)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(encodedJSON))
}

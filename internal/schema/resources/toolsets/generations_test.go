package toolsets

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// generationForPolicyTest supplies a complete stored snapshot whose observed
// state and declarative release intent follow the resource lifecycle contract.
func generationForPolicyTest(state ToolSetReleaseState) *ToolSet {
	resource := storedToolSetForResourceTest()
	resource.Status.Release.State = state
	if state != ToolSetReleaseStateDraft {
		resource.Spec.Release.DesiredState = ToolSetReleaseStatePrimary
	}
	return resource
}

// TestToolSetGenerationState uses observed status exclusively and rejects nil or
// unfamiliar snapshots instead of guessing from their desired release intent.
func TestToolSetGenerationState(t *testing.T) {
	policy := GenerationPolicy()
	for _, test := range []struct {
		state ToolSetReleaseState
		want  meta.GenerationState
	}{
		{ToolSetReleaseStateDraft, meta.GenerationEditable},
		{ToolSetReleaseStatePrimary, meta.GenerationPublished},
		{ToolSetReleaseStateActive, meta.GenerationHistorical},
		{ToolSetReleaseStateArchived, meta.GenerationHistorical},
	} {
		got, err := policy.State(generationForPolicyTest(test.state))
		require.NoError(t, err)
		require.Equal(t, test.want, got)
	}
	for _, resource := range []*ToolSet{nil, NewToolSet(), generationForPolicyTest(""), generationForPolicyTest("unknown")} {
		_, err := policy.State(resource)
		require.Error(t, err)
	}
}

// TestToolSetGenerationChanges distinguishes membership and metadata writes
// from generation work, including empty release objects and invalid nulls.
func TestToolSetGenerationChanges(t *testing.T) {
	policy := GenerationPolicy()
	require.False(t, policy.ChangesGeneration(nil))
	require.False(t, policy.ChangesGeneration(&ToolSetPatch{}))
	for _, test := range []struct {
		spec string
		want bool
	}{
		{`{}`, false},
		{`{"connectionSelector":{"matchLabels":{}}}`, false},
		{`{"release":{}}`, false},
		{`{"release":null}`, false},
		{`{"release":{"desiredState":"draft"}}`, true},
		{`{"release":{"desiredState":null}}`, true},
		{`{"definition":null}`, true},
	} {
		var patch ToolSetPatch
		input := `{"apiVersion":"authproxy.net/v1alpha1","kind":"ToolSet","metadata":{},"spec":` + test.spec + `}`
		require.NoError(t, util.DecodeJSONStrict([]byte(input), &patch))
		require.Equal(t, test.want, policy.ChangesGeneration(&patch), test.spec)
	}
	patch := NewToolSetPatch()
	labels := map[string]string{"team": "integrations"}
	patch.Metadata.Labels = &labels
	require.False(t, policy.ChangesGeneration(patch))
	patch.Spec.Definition = explicitDefinitionForTest()
	require.True(t, policy.ChangesGeneration(patch))
}

// TestToolSetGenerationSelection keeps all logical-only work on the selected
// snapshot, then chooses a draft or newest source only for generation changes.
func TestToolSetGenerationSelection(t *testing.T) {
	policy := GenerationPolicy()
	for _, observed := range []ToolSetReleaseState{ToolSetReleaseStateDraft, ToolSetReleaseStatePrimary, ToolSetReleaseStateActive, ToolSetReleaseStateArchived} {
		for _, intent := range []ToolSetReleaseState{"", ToolSetReleaseStateDraft, ToolSetReleaseStatePrimary} {
			for _, hasEditable := range []bool{false, true} {
				for _, changed := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/editable=%t/changed=%t", observed, intent, hasEditable, changed), func(t *testing.T) {
						selected := generationForPolicyTest(observed)
						desired := selected.Clone()
						desired.Spec.Release.DesiredState = intent
						selection, err := policy.Select(desired, selected, changed, hasEditable)
						require.NoError(t, err)
						if !changed {
							require.Equal(t, meta.GenerationSelected, selection.Source)
							require.Nil(t, selection.Context)
							return
						}
						want := meta.GenerationNewest
						if hasEditable {
							want = meta.GenerationEditableSource
						}
						require.Equal(t, want, selection.Source)
						ctx := selection.Context.(toolSetGenerationContext)
						require.Equal(t, !hasEditable, ctx.requiresDraft)
						require.Equal(t, observed == ToolSetReleaseStatePrimary && intent == ToolSetReleaseStatePrimary, ctx.publishDefinition)
					})
				}
			}
		}
	}
}

// TestToolSetGenerationPreliminaryLogicalPatch reproduces the CLI ordering:
// Finalize runs before ChangesGeneration and Select. A selector-only edit must
// not become a publication or cause a diff against an unrelated existing draft.
func TestToolSetGenerationPreliminaryLogicalPatch(t *testing.T) {
	policy := GenerationPolicy()
	for _, observed := range []ToolSetReleaseState{ToolSetReleaseStateDraft, ToolSetReleaseStatePrimary, ToolSetReleaseStateActive, ToolSetReleaseStateArchived} {
		for _, intent := range []ToolSetReleaseState{"", ToolSetReleaseStateDraft, ToolSetReleaseStatePrimary} {
			for _, logicalField := range []string{"empty", "selector", "metadata", "empty release"} {
				t.Run(fmt.Sprintf("%s/%s/%s", observed, intent, logicalField), func(t *testing.T) {
					selected := generationForPolicyTest(observed)
					desired := selected.Clone()
					desired.Spec.Release.DesiredState = intent
					patch := NewToolSetPatch()
					switch logicalField {
					case "selector":
						patch.Spec.ConnectionSelector = &ConnectionSelector{MatchLabels: map[string]string{"provider": "other"}}
					case "metadata":
						labels := map[string]string{"team": "other"}
						patch.Metadata.Labels = &labels
					case "empty release":
						patch.Spec.Release = &ToolSetReleaseSpecPatch{}
					}
					finalized, err := policy.Finalize(desired, selected, patch, nil, false)
					require.NoError(t, err)
					require.Equal(t, patch, finalized)
					require.False(t, policy.ChangesGeneration(finalized))
					selection, err := policy.Select(desired, selected, policy.ChangesGeneration(finalized), true)
					require.NoError(t, err)
					require.Equal(t, meta.GenerationSelected, selection.Source, "an existing draft may have an unrelated definition")
					require.Nil(t, selection.Context)
				})
			}
		}
	}
}

// TestToolSetGenerationFinalization retains generation intent when a second
// comparison finds the chosen source already has the requested definition.
func TestToolSetGenerationFinalization(t *testing.T) {
	policy := GenerationPolicy()
	for _, test := range []struct {
		name                          string
		selected, current, intent     ToolSetReleaseState
		selectedContext, hasEditable  bool
		definitionPatch, emptyRelease bool
		wantDefinition                bool
		wantRelease                   ToolSetReleaseState
	}{
		{"edit draft", ToolSetReleaseStateDraft, ToolSetReleaseStateDraft, "", false, true, true, false, true, ""},
		{"publish replacement from primary", ToolSetReleaseStatePrimary, ToolSetReleaseStatePrimary, ToolSetReleaseStatePrimary, false, false, true, false, true, ToolSetReleaseStatePrimary},
		{"draft replacement from primary", ToolSetReleaseStatePrimary, ToolSetReleaseStatePrimary, ToolSetReleaseStateDraft, false, false, true, false, true, ToolSetReleaseStateDraft},
		{"publish replacement from historical", ToolSetReleaseStateArchived, ToolSetReleaseStateArchived, ToolSetReleaseStatePrimary, false, false, true, false, true, ToolSetReleaseStatePrimary},
		{"publish matching draft", ToolSetReleaseStatePrimary, ToolSetReleaseStateDraft, ToolSetReleaseStatePrimary, true, true, false, false, true, ToolSetReleaseStatePrimary},
		{"clone matching newest as draft", ToolSetReleaseStatePrimary, ToolSetReleaseStateArchived, "", true, false, false, false, false, ToolSetReleaseStateDraft},
		{"empty release does not block draft", ToolSetReleaseStatePrimary, ToolSetReleaseStateArchived, "", true, false, false, true, false, ToolSetReleaseStateDraft},
		{"publish matching newest", ToolSetReleaseStatePrimary, ToolSetReleaseStateArchived, ToolSetReleaseStatePrimary, true, false, false, false, true, ToolSetReleaseStatePrimary},
		{"matching existing draft is satisfied", ToolSetReleaseStatePrimary, ToolSetReleaseStateDraft, "", true, true, false, false, false, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			selected := generationForPolicyTest(test.selected)
			current := generationForPolicyTest(test.current)
			desired := current.Clone()
			desired.Spec.Release.DesiredState = test.intent
			var context any
			if test.selectedContext {
				selection, err := policy.Select(desired, selected, true, test.hasEditable)
				require.NoError(t, err)
				context = selection.Context
			}
			patch := NewToolSetPatch()
			if test.definitionPatch {
				patch.Spec.Definition = desired.Spec.Definition.Clone()
			}
			if test.emptyRelease {
				patch.Spec.Release = &ToolSetReleaseSpecPatch{}
			}
			finalized, err := policy.Finalize(desired, current, patch, context, false)
			require.NoError(t, err)
			require.Equal(t, test.wantDefinition, finalized.Spec.HasDefinition())
			if test.wantRelease == "" {
				require.False(t, finalized.Spec.Release.HasDesiredState())
			} else {
				require.True(t, finalized.Spec.Release.HasDesiredState())
				require.Equal(t, test.wantRelease, *finalized.Spec.Release.DesiredState)
			}
			require.NoError(t, finalized.ValidateFor(meta.ValidationModeUpdate, nil))
			require.Equal(t, test.current, current.Status.Release.State)
		})
	}
}

// TestToolSetGenerationExplicitTargets prohibits every selector write while
// preserving immutable historical no-ops, including bare release:{}.
func TestToolSetGenerationExplicitTargets(t *testing.T) {
	policy := GenerationPolicy()
	for _, observed := range []ToolSetReleaseState{ToolSetReleaseStateDraft, ToolSetReleaseStatePrimary, ToolSetReleaseStateActive, ToolSetReleaseStateArchived} {
		for _, field := range []string{"empty", "empty release", "selector", "null selector", "definition", "release", "labels", "generation"} {
			t.Run(string(observed)+"/"+field, func(t *testing.T) {
				current := generationForPolicyTest(observed)
				desired := current.Clone()
				patch := NewToolSetPatch()
				switch field {
				case "empty release":
					patch.Spec.Release = &ToolSetReleaseSpecPatch{}
				case "selector":
					patch.Spec.ConnectionSelector = current.Spec.ConnectionSelector.Clone()
				case "null selector":
					patch.Spec.connectionSelectorPresent = true
				case "definition":
					patch.Spec.Definition = current.Spec.Definition.Clone()
				case "release":
					intent := ToolSetReleaseStatePrimary
					patch.Spec.Release = &ToolSetReleaseSpecPatch{DesiredState: &intent}
				case "labels":
					labels := map[string]string{"team": "other"}
					patch.Metadata.Labels = &labels
				case "generation":
					generation := current.Metadata.Generation
					patch.Metadata.Generation = &generation
				}
				finalized, err := policy.Finalize(desired, current, patch, nil, true)
				if field == "selector" || field == "null selector" {
					require.ErrorContains(t, err, "logical tool set endpoint")
				} else if observed != ToolSetReleaseStateDraft && field != "empty" && field != "empty release" {
					require.ErrorContains(t, err, "not a draft")
				} else {
					require.NoError(t, err)
					require.NotNil(t, finalized)
				}
			})
		}
	}
	current := generationForPolicyTest(ToolSetReleaseStateDraft)
	desired := current.Clone()
	desired.Spec.Release.DesiredState = ToolSetReleaseStatePrimary
	patch := NewToolSetPatch()
	patch.Spec.Definition = desired.Spec.Definition.Clone()
	finalized, err := policy.Finalize(desired, current, patch, nil, true)
	require.NoError(t, err)
	require.Equal(t, ToolSetReleaseStatePrimary, *finalized.Spec.Release.DesiredState)
}

// TestToolSetGenerationFinalizationOwnership verifies both injected definitions
// and existing patch fields remain independent of every input after finalization.
func TestToolSetGenerationFinalizationOwnership(t *testing.T) {
	policy := GenerationPolicy()
	for _, suppliedDefinition := range []bool{false, true} {
		t.Run(fmt.Sprintf("supplied-definition=%t", suppliedDefinition), func(t *testing.T) {
			current := generationForPolicyTest(ToolSetReleaseStateDraft)
			selected := generationForPolicyTest(ToolSetReleaseStatePrimary)
			desired := current.Clone()
			desired.Spec.Release.DesiredState = ToolSetReleaseStatePrimary
			patch := NewToolSetPatch()
			labels := map[string]string{"team": "integrations"}
			patch.Metadata.Labels = &labels
			patch.Spec.ConnectionSelector = current.Spec.ConnectionSelector.Clone()
			if suppliedDefinition {
				patch.Spec.Definition = desired.Spec.Definition.Clone()
			}
			inputs := []any{current, selected, desired, patch}
			before, err := json.Marshal(inputs)
			require.NoError(t, err)
			selection, err := policy.Select(desired, selected, true, true)
			require.NoError(t, err)
			finalized, err := policy.Finalize(desired, current, patch, selection.Context, false)
			require.NoError(t, err)
			after, err := json.Marshal(inputs)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after))
			(*finalized.Metadata.Labels)["team"] = "changed"
			finalized.Spec.ConnectionSelector.MatchLabels["provider"] = "changed"
			*finalized.Spec.Release.DesiredState = ToolSetReleaseStateDraft
			template := &finalized.Spec.Definition.Source.Explicit.Tools[0]
			template.Metadata.Labels["capability"] = "changed"
			template.Spec.InputSchema[0] = '['
			after, err = json.Marshal(inputs)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after))
		})
	}
}

// TestToolSetGenerationInvalidInputs rejects missing snapshots and incompatible
// policy context without panicking or returning a partially usable patch.
func TestToolSetGenerationInvalidInputs(t *testing.T) {
	policy := GenerationPolicy()
	valid := generationForPolicyTest(ToolSetReleaseStateDraft)
	_, err := policy.Select(nil, valid, false, false)
	require.Error(t, err)
	for _, current := range []*ToolSet{nil, NewToolSet(), generationForPolicyTest("unknown")} {
		_, err := policy.Select(valid, current, false, false)
		require.Error(t, err)
		result, err := policy.Finalize(valid, current, NewToolSetPatch(), nil, false)
		require.Error(t, err)
		require.Nil(t, result)
	}
	_, err = policy.Finalize(nil, valid, NewToolSetPatch(), nil, false)
	require.Error(t, err)
	for _, patch := range []*ToolSetPatch{nil, {}, {Metadata: &meta.ObjectMetaPatch{}}, {Spec: &ToolSetSpecPatch{}}} {
		_, err := policy.Finalize(valid, valid, patch, nil, false)
		require.ErrorContains(t, err, "metadata and spec")
	}
	for _, context := range []any{"wrong policy", (*toolSetGenerationContext)(nil)} {
		_, err := policy.Finalize(valid, valid, NewToolSetPatch(), context, false)
		require.ErrorContains(t, err, "context")
	}
	selection, err := policy.Select(valid, valid, true, true)
	require.NoError(t, err)
	_, err = policy.Finalize(valid, valid, NewToolSetPatch(), selection.Context, true)
	require.ErrorContains(t, err, "context")
}

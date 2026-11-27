package routes

import (
	"context"
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/internal/cli/apply"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/stretchr/testify/require"
)

// applyToolSetManifest exercises the CLI loader, planner, and mutations through
// the real ToolSet routes. The fixture already creates the owning namespace.
func applyToolSetManifest(t *testing.T, client *apply.Client, manifest map[string]any) (*toolsetschema.ToolSet, apply.Operation) {
	t.Helper()
	ctx := context.Background()
	targets, err := client.ResolveBatch(ctx, []apply.Document{loadToolApplyManifest(t, manifest)})
	require.NoError(t, err)
	require.Len(t, targets, 1)
	plan, err := apply.Reconcile(targets[0], apply.ReconcileOptions{Overwrite: true})
	require.NoError(t, err)

	var live *apply.LiveResource
	switch plan.Operation {
	case apply.OperationCreate:
		live, err = client.Create(ctx, plan.Document)
	case apply.OperationUpdate:
		live, err = client.Update(ctx, plan.Target, plan.Patch)
	case apply.OperationUnchanged:
		live = plan.Target.Current
	default:
		t.Fatalf("unexpected apply operation: %s", plan.Operation)
	}
	require.NoError(t, err)
	toolSet, ok := live.Resource.(*toolsetschema.ToolSet)
	require.True(t, ok)
	return toolSet, plan.Operation
}

// TestToolSetApplyCreateWithDefaults checks that omitted release and selector
// scope are defaulted by the API without creating extra generations on reapply.
func TestToolSetApplyCreateWithDefaults(t *testing.T) {
	s, _ := newToolsTestSetup(t)
	client := newToolApplyClient(t, s)
	manifest := toolSetBody("root.acme", "applied-draft", "")

	// Omitted release starts as a draft. An empty label map selects the whole owner subtree.
	created, operation := applyToolSetManifest(t, client, manifest)
	require.Equal(t, apply.OperationCreate, operation)
	require.Equal(t, uint64(1), created.Metadata.Generation)
	require.Equal(t, toolsetschema.ToolSetReleaseStateDraft, created.Status.Release.State)
	require.Equal(t, "root.acme.**", created.Spec.ConnectionSelector.Namespace)
	require.Equal(t, map[string]string{}, created.Spec.ConnectionSelector.MatchLabels)

	// One reapply stores the server ID in history. The next does not write.
	applyToolSetManifest(t, client, manifest)
	reapplied, operation := applyToolSetManifest(t, client, manifest)
	require.Equal(t, apply.OperationUnchanged, operation)
	require.Equal(t, created.Metadata.ID, reapplied.Metadata.ID)
	w := s.do(t, http.MethodGet, "/toolsets/"+created.Metadata.ID+"/generations", nil, aschema.AllPermissions())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, []string{"1:draft"}, generationStates(decodeToolSetList(t, w)))
}

// TestToolSetApplySelectorAndPublication verifies that logical selector edits
// leave an existing draft alone, while definition changes use that draft and
// explicit publication retains the previous published snapshot.
func TestToolSetApplySelectorAndPublication(t *testing.T) {
	s, _ := newToolsTestSetup(t)
	client := newToolApplyClient(t, s)
	all := aschema.AllPermissions()
	manifest := toolSetBody("root.acme", "applied-calendar", "primary")
	spec := manifest["spec"].(map[string]any)
	spec["connectionSelector"] = map[string]any{
		"namespace": "root.acme", "matchLabels": map[string]string{"provider": "google"},
	}
	primary, operation := applyToolSetManifest(t, client, manifest)
	require.Equal(t, apply.OperationCreate, operation)
	require.Equal(t, toolsetschema.ToolSetReleaseStatePrimary, primary.Status.Release.State)
	path := "/toolsets/" + primary.Metadata.ID
	primaryURL := firstTemplateURL(*primary)

	// An independent draft has a different definition and must survive selector edits.
	const draftURL = "https://{{cfg.apiHost}}/draft/calendars"
	w := s.do(t, http.MethodPost, path+"/generations", toolSetPatchBody(nil, map[string]any{
		"definition": toolSetDefinition(toolSetTemplate("list-calendars", draftURL)),
	}), all)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Equal(t, uint64(2), decodeToolSet(t, w).Metadata.Generation)

	// Reset both selector filters while keeping the full primary definition in the manifest.
	spec["connectionSelector"] = map[string]any{"matchLabels": map[string]string{}}
	selected, operation := applyToolSetManifest(t, client, manifest)
	require.Equal(t, apply.OperationUpdate, operation)
	require.Equal(t, uint64(1), selected.Metadata.Generation)
	require.Equal(t, "root.acme.**", selected.Spec.ConnectionSelector.Namespace)
	require.Equal(t, map[string]string{}, selected.Spec.ConnectionSelector.MatchLabels)
	require.Equal(t, primaryURL, firstTemplateURL(*selected))
	w = s.do(t, http.MethodGet, path+"/generations", nil, all)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	generations := decodeToolSetList(t, w)
	require.Equal(t, []string{"1:primary", "2:draft"}, generationStates(generations))
	require.Equal(t, draftURL, firstTemplateURL(generations.Items[1]))
	require.Equal(t, selected.Spec.ConnectionSelector, generations.Items[1].Spec.ConnectionSelector)
	_, operation = applyToolSetManifest(t, client, manifest)
	require.Equal(t, apply.OperationUnchanged, operation)

	// Omitting earlier publication intent lets a changed definition edit the existing draft.
	const replacementURL = "https://{{cfg.apiHost}}/v2/calendars"
	spec["definition"] = toolSetDefinition(toolSetTemplate("list-calendars", replacementURL))
	delete(spec, "release")
	draft, operation := applyToolSetManifest(t, client, manifest)
	require.Equal(t, apply.OperationUpdate, operation)
	require.Equal(t, uint64(2), draft.Metadata.Generation)
	require.Equal(t, toolsetschema.ToolSetReleaseStateDraft, draft.Status.Release.State)
	require.Equal(t, replacementURL, firstTemplateURL(*draft))
	_, operation = applyToolSetManifest(t, client, manifest)
	require.Equal(t, apply.OperationUnchanged, operation)

	// The desired definition now matches the draft exactly. Explicit primary intent
	// still publishes it and preserves the previous generation's definition.
	spec["release"] = map[string]any{"desiredState": "primary"}
	published, operation := applyToolSetManifest(t, client, manifest)
	require.Equal(t, apply.OperationUpdate, operation)
	require.Equal(t, uint64(2), published.Metadata.Generation)
	require.Equal(t, toolsetschema.ToolSetReleaseStatePrimary, published.Status.Release.State)
	require.Equal(t, replacementURL, firstTemplateURL(*published))
	w = s.do(t, http.MethodGet, path+"/generations", nil, all)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	generations = decodeToolSetList(t, w)
	require.Equal(t, []string{"1:active", "2:primary"}, generationStates(generations))
	require.Equal(t, primaryURL, firstTemplateURL(generations.Items[0]))
	_, operation = applyToolSetManifest(t, client, manifest)
	require.Equal(t, apply.OperationUnchanged, operation)
}

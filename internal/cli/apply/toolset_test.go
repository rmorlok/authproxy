package apply

import (
	"fmt"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/stretchr/testify/require"
)

const toolSetApplyMetadata = "  name: example\n  namespace: root.product"

// TestLoadLogicalToolSetList accepts a typed list of logical manifests and
// rejects generation addressing and observed status from authored input.
func TestLoadLogicalToolSetList(t *testing.T) {
	// The empty label map must survive decoding as a match-all selector.
	docs, err := loadText(t, `apiVersion: authproxy.net/v1alpha1
kind: ToolSetList
items:
  - apiVersion: authproxy.net/v1alpha1
    kind: ToolSet
    metadata:
      name: example
    spec:
      connectionSelector: {matchLabels: {}}
      definition: {source: {explicit: {tools: []}}}
`, "root.product")
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Contains(t, docs[0].Source, "item 1")
	resource := docs[0].Resource.(*toolsets.ToolSet)
	require.Equal(t, "root.product", resource.Metadata.Namespace)
	require.Equal(t, map[string]string{}, resource.Spec.ConnectionSelector.MatchLabels)

	// Server response fields cannot turn an apply into a generation mutation.
	for _, tc := range []struct{ input, diagnostic string }{
		{manifestText("ToolSet", toolSetApplyMetadata+"\n  generation: 1", `{}`), "omit metadata.generation"},
		{manifestText("ToolSet", toolSetApplyMetadata, `{}`) + "status: {release: {state: primary}}\n", "status is server-owned"},
	} {
		docs, err := loadText(t, tc.input, "")
		require.ErrorContains(t, err, tc.diagnostic)
		require.Nil(t, docs)
	}
}

// liveToolSetForApply decodes a primary generation as returned through the
// logical endpoint, with a canonical selector scope supplied by the caller.
func liveToolSetForApply(t *testing.T, selector string) *LiveResource {
	t.Helper()
	data := fmt.Sprintf(`{
		"apiVersion":"authproxy.net/v1alpha1","kind":"ToolSet",
		"metadata":{"id":%q,"name":"example","namespace":"root.product","generation":3},
		"spec":{"connectionSelector":%s,"definition":{"source":{"explicit":{"tools":[]}}},"release":{"desiredState":"primary"}},
		"status":{"release":{"state":"primary"}}
	}`, apid.New(apid.PrefixToolSet).String(), selector)
	live, err := newClient().decodeLive(toolsets.ToolSetKind, []byte(data), false)
	require.NoError(t, err)
	return live
}

// TestToolSetApplyAdoptsWholeSelector ensures an authored empty selector clears
// both a preexisting namespace restriction and all unmanaged label requirements.
func TestToolSetApplyAdoptsWholeSelector(t *testing.T) {
	live := liveToolSetForApply(t, `{"namespace":"root.product.team","matchLabels":{"provider":"calendar"}}`)
	doc := clientDoc(t, "ToolSet", toolSetApplyMetadata, `{"connectionSelector":{"matchLabels":{}}}`)

	// Adopting the complete selector works even with overwriting drift disabled.
	plan, err := Reconcile(Target{Document: doc, Current: live}, ReconcileOptions{Overwrite: false})
	require.NoError(t, err)
	require.Equal(t, OperationUpdate, plan.Operation)
	require.Equal(t, map[string]any{
		"connectionSelector": map[string]any{"namespace": "root.product.**", "matchLabels": map[string]any{}},
	}, planObject(t, plan)["spec"])
	require.Equal(t, "root.product.team", live.Resource.(*toolsets.ToolSet).Spec.ConnectionSelector.Namespace)
	require.Empty(t, doc.Resource.(*toolsets.ToolSet).Spec.ConnectionSelector.Namespace)

	// A selector-only manifest also works after a full apply. The omitted
	// definition and release must stay out of the patch.
	full := clientDoc(t, "ToolSet", toolSetApplyMetadata, `{
		"connectionSelector":{"namespace":"root.product.team","matchLabels":{"provider":"calendar"}},
		"definition":{"source":{"explicit":{"tools":[]}}},
		"release":{"desiredState":"primary"}
	}`)
	managed := withHistory(t, live, full)
	partialPlan, err := Reconcile(Target{Document: doc, Current: managed}, ReconcileOptions{Overwrite: false})
	require.NoError(t, err)
	require.Equal(t, planObject(t, plan)["spec"], planObject(t, partialPlan)["spec"])
}

// TestToolSetApplySelectorScopeAndDrift compares the authored default against
// the projected scope and reports drift across the whole managed selector.
func TestToolSetApplySelectorScopeAndDrift(t *testing.T) {
	previous := clientDoc(t, "ToolSet", toolSetApplyMetadata, `{"connectionSelector":{"matchLabels":{"provider":"calendar"}}}`)
	live := withHistory(t, liveToolSetForApply(t, `{"namespace":"root.product.**","matchLabels":{"provider":"calendar"}}`), previous)

	// The server's explicit namespace default alone does not need a patch.
	plan, err := Reconcile(Target{Document: previous, Current: live}, ReconcileOptions{Overwrite: false})
	require.NoError(t, err)
	require.Equal(t, OperationUnchanged, plan.Operation)

	// An explicitly authored default only changes the recorded manifest.
	explicitDefault := clientDoc(t, "ToolSet", toolSetApplyMetadata, `{"connectionSelector":{"namespace":"root.product.**","matchLabels":{"provider":"calendar"}}}`)
	plan, err = Reconcile(Target{Document: explicitDefault, Current: live}, ReconcileOptions{Overwrite: false})
	require.NoError(t, err)
	require.Equal(t, OperationUpdate, plan.Operation)
	require.Empty(t, planObject(t, plan)["spec"])

	// An externally added label is drift of the selector, even if apply never
	// previously declared that particular label key.
	drifted := *live
	resource := live.Resource.(*toolsets.ToolSet).Clone()
	resource.Spec.ConnectionSelector.MatchLabels["team"] = "other"
	drifted.Resource = resource
	_, err = Reconcile(Target{Document: previous, Current: &drifted}, ReconcileOptions{Overwrite: false})
	require.ErrorContains(t, err, "apply conflict at /spec/connectionSelector")
}

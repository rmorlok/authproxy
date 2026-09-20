package connectors

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func TestGenerationPolicyObservedStates(t *testing.T) {
	policy := GenerationPolicy()

	// Callers use these classifications without knowing Connector state names:
	// only drafts are editable, primary is published, and active/archived are
	// historical generations that must not be modified in place.
	for _, tc := range []struct {
		state ConnectorReleaseState
		want  meta.GenerationState
	}{
		{"draft", meta.GenerationEditable},
		{"primary", meta.GenerationPublished},
		{"active", meta.GenerationHistorical},
		{"archived", meta.GenerationHistorical},
	} {
		t.Run(string(tc.state), func(t *testing.T) {
			c := NewConnector()
			c.Status = &ConnectorStatus{Release: ConnectorReleaseStatus{State: tc.state}}
			got, err := policy.State(c)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	// Missing observed state must fail instead of treating a partially populated
	// resource as editable or guessing from its desired release intent.
	_, err := policy.State(NewConnector())
	require.Error(t, err)

	// An unfamiliar state also fails closed: a new server state must not become
	// writable until the policy explicitly defines how to classify it.
	c := NewConnector()
	c.Status = &ConnectorStatus{
		Release: ConnectorReleaseStatus{
			State: "unknown",
		},
	}
	_, err = policy.State(c)
	require.Error(t, err)
}

func TestGenerationPolicyFinalizationIsPure(t *testing.T) {
	policy := GenerationPolicy()

	// Model the editable draft whose definition we intend to publish.
	current := NewConnector()
	current.Status = &ConnectorStatus{
		Release: ConnectorReleaseStatus{
			State: ConnectorReleaseStateDraft,
		},
	}
	current.Spec.Definition.DisplayName = "desired draft"

	// A logical GET may return a different, already-primary generation. Keep it
	// separate from the draft so the policy can account for that selected state.
	selected := current.Clone()
	selected.Status.Release.State = ConnectorReleaseStatePrimary

	// Desired state expresses publication intent; changing it here must not
	// alter the draft's observed state or the selected generation.
	desired := current.Clone()
	desired.Spec.Release.DesiredState = ConnectorReleaseStatePrimary

	// Supply the earlier reconciliation decision: a generation change is needed
	// and an editable draft exists. The returned context carries the publication
	// rule into finalization without the caller interpreting its contents.
	selection, err := policy.Select(
		desired,
		selected,
		true, // changesGeneration
		true, // hasEditable
	)
	require.NoError(t, err)

	// Start with an empty patch to prove finalization adds the required fields.
	// Snapshot every input so the later comparison detects mutations by policy.
	patch := NewConnectorPatch()
	before, err := json.Marshal([]any{current, selected, desired, patch})
	require.NoError(t, err)

	// This is a logical update, not an explicitly addressed generation. The
	// policy may prepare a publication patch but performs no cluster writes.
	finalized, err := policy.Finalize(
		desired,
		current,
		patch,
		selection.Context,
		false, // explicit generation
	)
	require.NoError(t, err)

	// Request primary and include the draft's definition. A release-only primary
	// patch could be treated as already satisfied by the existing primary; the
	// definition ensures the server handles publication of the chosen draft.
	// These assertions check the planned payload, not an observed state change.
	require.True(t, finalized.Spec.HasDefinition())
	require.True(t, finalized.Spec.HasRelease())
	require.Equal(t, ConnectorReleaseStatePrimary, *finalized.Spec.Release.DesiredState)
	require.Equal(t, "desired draft", finalized.Spec.Definition.DisplayName)

	// Planning must preserve both live snapshots, desired input, and the original
	// empty patch. All lifecycle adjustments belong to the returned patch.
	after, err := json.Marshal([]any{current, selected, desired, patch})
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))

	// Equality immediately after finalization alone would miss shared pointers.
	// Mutating the returned definition must not mutate the live draft through
	// an alias, either.
	finalized.Spec.Definition.DisplayName = "changed"
	require.Equal(t, "desired draft", current.Spec.Definition.DisplayName)

	// Selection context belongs to this resource policy. Reject incompatible
	// context rather than silently using defaults or asserting an unsafe type.
	_, err = policy.Finalize(
		desired,
		current,
		patch,
		"wrong policy context",
		false,
	)
	require.ErrorContains(t, err, "context")
}

func TestGenerationPolicyExplicitHistoricalGeneration(t *testing.T) {
	policy := GenerationPolicy()

	// An explicitly addressed archived generation is immutable. Its declaration
	// may still say primary because desired intent and observed history differ;
	// that is not permission to force it back into the primary state.
	current := NewConnector()
	current.Status = &ConnectorStatus{Release: ConnectorReleaseStatus{State: ConnectorReleaseStateArchived}}
	desired := current.Clone()
	desired.Spec.Release.DesiredState = ConnectorReleaseStatePrimary

	// No changes are permitted or needed for an empty patch, so finalization
	// succeeds without manufacturing a release transition for this explicit target.
	patch := NewConnectorPatch()
	_, err := policy.Finalize(desired, current, patch, nil, true)
	require.NoError(t, err)

	// Even a metadata-only update would send a PATCH to the historical generation
	// endpoint. It must fail before any write; immutability covers more than spec.
	labels := map[string]string{"team": "blue"}
	patch.Metadata.Labels = &labels
	_, err = policy.Finalize(desired, current, patch, nil, true)
	require.ErrorContains(t, err, "not a draft")

	// Identity fields also make a patch nonempty. They must not bypass the same
	// guard merely because neither labels nor the definition are being changed.
	patch = NewConnectorPatch()
	generation := uint64(1)
	patch.Metadata.Generation = &generation
	_, err = policy.Finalize(desired, current, patch, nil, true)
	require.ErrorContains(t, err, "not a draft")
}

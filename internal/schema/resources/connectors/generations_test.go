package connectors

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func TestGenerationPolicyObservedStates(t *testing.T) {
	policy := GenerationPolicy()
	for _, tc := range []struct {
		state ConnectorReleaseState
		want  meta.GenerationState
	}{
		{"draft", meta.GenerationEditable}, {"primary", meta.GenerationPublished}, {"active", meta.GenerationHistorical}, {"archived", meta.GenerationHistorical},
	} {
		t.Run(string(tc.state), func(t *testing.T) {
			c := NewConnector()
			c.Status = &ConnectorStatus{Release: ConnectorReleaseStatus{State: tc.state}}
			got, err := policy.State(c)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	_, err := policy.State(NewConnector())
	require.Error(t, err)
	c := NewConnector()
	c.Status = &ConnectorStatus{Release: ConnectorReleaseStatus{State: "unknown"}}
	_, err = policy.State(c)
	require.Error(t, err)
}

func TestGenerationPolicyFinalizationIsPure(t *testing.T) {
	policy := GenerationPolicy()
	current := NewConnector()
	current.Status = &ConnectorStatus{Release: ConnectorReleaseStatus{State: ConnectorReleaseStateDraft}}
	current.Spec.Definition.DisplayName = "desired draft"
	selected := current.Clone()
	selected.Status.Release.State = ConnectorReleaseStatePrimary
	desired := current.Clone()
	desired.Spec.Release.DesiredState = ConnectorReleaseStatePrimary
	selection, err := policy.Select(desired, selected, true, true)
	require.NoError(t, err)
	patch := NewConnectorPatch()
	before, err := json.Marshal([]any{current, selected, desired, patch})
	require.NoError(t, err)
	finalized, err := policy.Finalize(desired, current, patch, selection.Context, false)
	require.NoError(t, err)
	require.True(t, finalized.Spec.HasDefinition())
	require.True(t, finalized.Spec.HasRelease())
	require.Equal(t, ConnectorReleaseStatePrimary, *finalized.Spec.Release.DesiredState)
	require.Equal(t, "desired draft", finalized.Spec.Definition.DisplayName)
	after, err := json.Marshal([]any{current, selected, desired, patch})
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
	finalized.Spec.Definition.DisplayName = "changed"
	require.Equal(t, "desired draft", current.Spec.Definition.DisplayName)
	_, err = policy.Finalize(desired, current, patch, "wrong policy context", false)
	require.ErrorContains(t, err, "context")
}

func TestGenerationPolicyExplicitHistoricalGeneration(t *testing.T) {
	policy := GenerationPolicy()
	current := NewConnector()
	current.Status = &ConnectorStatus{Release: ConnectorReleaseStatus{State: ConnectorReleaseStateArchived}}
	desired := current.Clone()
	desired.Spec.Release.DesiredState = ConnectorReleaseStatePrimary
	patch := NewConnectorPatch()
	_, err := policy.Finalize(desired, current, patch, nil, true)
	require.NoError(t, err)
	labels := map[string]string{"team": "blue"}
	patch.Metadata.Labels = &labels
	_, err = policy.Finalize(desired, current, patch, nil, true)
	require.ErrorContains(t, err, "not a draft")
	patch = NewConnectorPatch()
	generation := uint64(1)
	patch.Metadata.Generation = &generation
	_, err = policy.Finalize(desired, current, patch, nil, true)
	require.ErrorContains(t, err, "not a draft")
}

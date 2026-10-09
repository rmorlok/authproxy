package toolsets

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/stretchr/testify/require"
)

// authoredToolSetForResourceTest supplies one complete explicit source without
// server-owned identity or status, keeping lifecycle cases independent.
func authoredToolSetForResourceTest() *ToolSet {
	resource := NewToolSet()
	resource.Metadata.Namespace = "root.product"
	resource.Spec.ConnectionSelector = &ConnectionSelector{
		MatchLabels: map[string]string{
			"provider": "calendar",
		},
	}
	resource.Spec.Definition = ToolSetDefinition{
		Source: ToolSetSource{
			Explicit: &ExplicitSource{
				Tools: []ToolTemplate{
					{
						Key: "list-calendars",
						Metadata: &ToolTemplateMetadata{
							Name:        "list-calendars",
							Labels:      map[string]string{"capability": "calendars"},
							Annotations: map[string]string{"description": "Calendar operations"},
						},
						Spec: tools.ToolDefinition{
							Description: "List calendars",
							Verbs:       []string{"tool:calendar.list"},
							InputSchema: common.RawJSON(`{"type":"object"}`),
							ProxyHTTP: &tools.ProxyHTTP{
								Method: "GET", URL: "https://{{cfg.host}}/calendars",
								Headers: map[string]string{"Accept": "application/json"},
								Query:   map[string]common.RawJSON{"limit": common.RawJSON(`10`)},
							},
						},
					},
				},
			},
		},
	}

	return resource
}

// storedToolSetForResourceTest represents a persisted draft generation with
// identity and release status supplied by the service.
func storedToolSetForResourceTest() *ToolSet {
	resource := authoredToolSetForResourceTest()
	resource.Metadata.ID = "tls_01example0000001"
	resource.Metadata.Name = "calendar-tools"
	resource.Metadata.Generation = 3
	resource.Spec.Release.DesiredState = ToolSetReleaseStateDraft
	resource.Status = &ToolSetStatus{Release: ToolSetReleaseStatus{State: ToolSetReleaseStateDraft}}
	return resource
}

// TestToolSetResourceLifecycle checks which fields must be authored or supplied
// by the server, including explicit namespace ownership in configuration files.
func TestToolSetResourceLifecycle(t *testing.T) {
	resource := authoredToolSetForResourceTest()
	for _, mode := range []meta.ValidationMode{
		meta.ValidationModeCreate,
		meta.ValidationModeConfig,
		meta.ValidationModeUpdate,
	} {
		require.NoError(t, resource.ValidateFor(mode, nil))
		withStatus := resource.Clone()
		withStatus.Status = &ToolSetStatus{Release: ToolSetReleaseStatus{State: ToolSetReleaseStateDraft}}
		require.ErrorContains(t, withStatus.ValidateFor(mode, nil), "status")
	}

	require.NoError(t, resource.Validate(nil))
	require.Empty(t, resource.Spec.Release.DesiredState, "config validation does not choose a release default")

	for _, mode := range []meta.ValidationMode{meta.ValidationModePersistence, meta.ValidationModeResponse} {
		err := resource.ValidateFor(mode, nil)
		for _, field := range []string{"metadata.id", "metadata.name", "metadata.generation", "spec.release.desiredState", "status"} {
			require.ErrorContains(t, err, field)
		}
		require.NoError(t, storedToolSetForResourceTest().ValidateFor(mode, nil))
	}

	require.ErrorContains(t, storedToolSetForResourceTest().ValidateFor(meta.ValidationModeCreate, nil), "server-owned on create")
	require.ErrorContains(t, resource.ValidateFor("unsupported", nil), "unknown metadata validation mode")
	require.ErrorContains(t, (*ToolSet)(nil).Validate(&common.ValidationContext{Path: "candidate"}), "candidate")

	resource.Metadata.Namespace = ""
	require.ErrorContains(t, resource.Validate(nil), "metadata.namespace")
}

// TestToolSetResourceValidation verifies shared metadata rules and that nested
// selector and template errors retain the caller's full validation path.
func TestToolSetResourceValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ToolSet)
		field  string
	}{
		{"kind", func(v *ToolSet) { v.Kind = "Tool" }, "kind"},
		{"version", func(v *ToolSet) { v.APIVersion = "authproxy.net/v2" }, "apiVersion"},
		{"ID prefix", func(v *ToolSet) { v.Metadata.ID = "tol_01example0000001" }, "metadata.id"},
		{"name", func(v *ToolSet) { v.Metadata.Name = "bad name" }, "metadata.name"},
		{"namespace", func(v *ToolSet) { v.Metadata.Namespace = "outside" }, "metadata.namespace"},
		{"labels", func(v *ToolSet) { v.Metadata.Labels = map[string]string{"invalid key": "value"} }, "metadata.labels"},
		{"annotations", func(v *ToolSet) { v.Metadata.Annotations = map[string]string{"invalid key": "value"} }, "metadata.annotations"},
		{"selector absent", func(v *ToolSet) { v.Spec.ConnectionSelector = nil }, "spec.connectionSelector"},
		{"selector labels absent", func(v *ToolSet) { v.Spec.ConnectionSelector.MatchLabels = nil }, "spec.connectionSelector.matchLabels"},
		{"selector escapes owner", func(v *ToolSet) { v.Spec.ConnectionSelector.Namespace = "root.**" }, "spec.connectionSelector.namespace"},
		{"definition source", func(v *ToolSet) { v.Spec.Definition.Source.Explicit = nil }, "spec.definition.source"},
		{"template definition", func(v *ToolSet) { v.Spec.Definition.Source.Explicit.Tools[0].Spec.Verbs = nil }, "spec.definition.source.explicit.tools[0].spec.verbs"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resource := storedToolSetForResourceTest()
			test.change(resource)
			err := resource.ValidateFor(meta.ValidationModePersistence, &common.ValidationContext{Path: "candidate"})
			require.ErrorContains(t, err, "candidate."+test.field)
		})
	}
}

// TestToolSetReleaseStates distinguishes authorable release intent from the
// observed state of a previously published generation.
func TestToolSetReleaseStates(t *testing.T) {
	states := []ToolSetReleaseState{
		ToolSetReleaseStateDraft,
		ToolSetReleaseStatePrimary,
		ToolSetReleaseStateActive,
		ToolSetReleaseStateArchived,
	}
	for _, desired := range states {
		t.Run(string(desired), func(t *testing.T) {
			authored := authoredToolSetForResourceTest()
			authored.Spec.Release.DesiredState = desired
			if desired == ToolSetReleaseStateDraft ||
				desired == ToolSetReleaseStatePrimary {
				require.NoError(t, authored.Validate(nil))
			} else {
				require.ErrorContains(t, authored.Validate(nil), "spec.release.desiredState")
			}
			for _, observed := range states {
				t.Run(string(observed), func(t *testing.T) {
					stored := storedToolSetForResourceTest()
					stored.Spec.Release.DesiredState = desired
					stored.Status.Release.State = observed
					err := stored.ValidateFor(meta.ValidationModeResponse, nil)
					valid := desired == ToolSetReleaseStateDraft && observed == ToolSetReleaseStateDraft ||
						desired == ToolSetReleaseStatePrimary && observed != ToolSetReleaseStateDraft
					if valid {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
				})
			}
		})
	}

	for _, state := range []ToolSetReleaseState{"", "unknown"} {
		stored := storedToolSetForResourceTest()
		stored.Status.Release.State = state
		require.ErrorContains(t, stored.ValidateFor(meta.ValidationModePersistence, nil), "status.release.state")
	}
	
	authored := authoredToolSetForResourceTest()
	authored.Spec.Release.DesiredState = "unknown"
	require.ErrorContains(t, authored.Validate(nil), "spec.release.desiredState")
}

// TestToolSetIdentityAndDefaults checks that API creation assigns only initial
// identity and desired state, preserving explicit choices and the input value.
func TestToolSetIdentityAndDefaults(t *testing.T) {
	id := apid.ID("tls_01example0000001")
	require.NoError(t, ValidateID(id.String()))
	ref := NewToolSetReference(id)
	require.Equal(t, meta.APIVersionV1Alpha1, ref.APIVersion)
	require.Equal(t, ToolSetKind, ref.Kind)
	require.Equal(t, id.String(), ref.ID)
	require.Zero(t, ref.Generation)
	require.Equal(t, apid.Nil, (*ToolSet)(nil).GetId())
	for _, invalid := range []string{"", "unknown", "tol_01example0000001"} {
		require.Error(t, ValidateID(invalid))
		resource := NewToolSet()
		resource.Metadata.ID = invalid
		require.Equal(t, apid.Nil, resource.GetId())
	}

	resource := authoredToolSetForResourceTest()
	created := resource.ApplyAPICreateDefaults(id)
	require.Equal(t, id, created.GetId())
	require.Equal(t, uint64(1), created.Metadata.Generation)
	require.Equal(t, common.ResourceName(id.String()), created.Metadata.Name)
	require.Equal(t, ToolSetReleaseStateDraft, created.Spec.Release.DesiredState)
	require.Equal(t, "root.product.**", created.Spec.ConnectionSelector.Namespace)
	require.Nil(t, created.Status)
	require.Equal(t, authoredToolSetForResourceTest(), resource)

	resource.Metadata.Name = "chosen-name"
	resource.Spec.Release.DesiredState = ToolSetReleaseStatePrimary
	resource.Spec.ConnectionSelector.Namespace = "root.product.child"
	created = resource.ApplyAPICreateDefaults(id)
	require.Equal(t, resource.Metadata.Name, created.Metadata.Name)
	require.Equal(t, ToolSetReleaseStatePrimary, created.Spec.Release.DesiredState)
	require.Equal(t, "root.product.child", created.Spec.ConnectionSelector.Namespace)
	resource.Spec.ConnectionSelector = nil
	require.Nil(t, resource.ApplyAPICreateDefaults(id).Spec.ConnectionSelector)
	require.Nil(t, (*ToolSet)(nil).ApplyAPICreateDefaults(id))
}

// TestToolSetCloneOwnership checks the resource's delegation to deep-copy
// helpers so logical selector updates cannot mutate a prior generation snapshot.
func TestToolSetCloneOwnership(t *testing.T) {
	require.Nil(t, (*ToolSet)(nil).Clone())
	empty := NewToolSet().Clone()
	require.Nil(t, empty.Status)
	require.Nil(t, empty.Spec.ConnectionSelector)

	original := storedToolSetForResourceTest()
	original.Metadata.Labels = map[string]string{"team": "integrations"}
	original.Metadata.Annotations = map[string]string{"description": "Shared calendars"}
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	original.Metadata.CreatedAt, original.Metadata.UpdatedAt = &now, &now
	clone := original.Clone()
	require.Equal(t, original, clone)
	clone.Metadata.Labels["team"] = "changed"
	clone.Metadata.Annotations["description"] = "changed"
	*clone.Metadata.CreatedAt = now.Add(time.Hour)
	*clone.Metadata.UpdatedAt = now.Add(2 * time.Hour)
	clone.Spec.ConnectionSelector.MatchLabels["provider"] = "changed"
	clone.Status.Release.State = ToolSetReleaseStatePrimary
	template := &clone.Spec.Definition.Source.Explicit.Tools[0]
	template.Key = "changed"
	template.Metadata.Labels["capability"] = "changed"
	template.Metadata.Annotations["description"] = "changed"
	template.Spec.Verbs[0] = "tool:changed"
	template.Spec.InputSchema[0] = '['
	template.Spec.ProxyHTTP.Headers["Accept"] = "changed"
	template.Spec.ProxyHTTP.Query["limit"][0] = '2'

	expected := storedToolSetForResourceTest()
	expected.Metadata.Labels = map[string]string{"team": "integrations"}
	expected.Metadata.Annotations = map[string]string{"description": "Shared calendars"}
	expected.Metadata.CreatedAt, expected.Metadata.UpdatedAt = &now, &now
	require.Equal(t, expected, original)
}

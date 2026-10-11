package api

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// toolSetMigrationForTest acknowledges a migration without claiming completion.
func toolSetMigrationForTest() *ConnectionToolSetGenerationMigrationAction {
	binding := toolSetBindingForTest()
	ref := binding.Spec.ToolSetRef
	ref.Generation = 3
	action := NewConnectionToolSetGenerationMigrationResponse(binding.Spec.ConnectionRef,
		ConnectionToolSetGenerationMigrationSpec{ToolSetRef: ref},
		ConnectionToolSetActionStatus{TaskID: "opaque-task-token+/="})
	return &action
}

// toolSetRefreshForTest identifies the logical binding without pinning its source.
func toolSetRefreshForTest() *ConnectionToolSetRefreshAction {
	binding := toolSetBindingForTest()
	action := NewConnectionToolSetRefreshResponse(binding.Spec.ConnectionRef,
		ConnectionToolSetRefreshSpec{ToolSetRef: binding.Spec.ToolSetRef},
		ConnectionToolSetActionStatus{TaskID: "opaque-task-token+/="})
	return &action
}

// TestConnectionToolSetActionReferences accepts canonical management references
// and leaves resource resolution, ancestry, and lifecycle eligibility to services.
func TestConnectionToolSetActionReferences(t *testing.T) {
	for _, named := range []bool{false, true} {
		migration, refresh := toolSetMigrationForTest(), toolSetRefreshForTest()
		for _, ref := range []*meta.ObjectReference{&migration.Metadata.Target, &refresh.Metadata.Target} {
			ref.Namespace, ref.Name = "root.product.user-1", "calendar"
			if named {
				ref.ID = ""
			}
		}
		for _, ref := range []*meta.ObjectReference{&migration.Spec.ToolSetRef, &refresh.Spec.ToolSetRef} {
			// Cross-namespace references have valid shape; authorization and
			// binding ownership cannot be established by these DTOs.
			ref.Namespace, ref.Name = "root.other", "calendar-tools"
			if named {
				ref.ID = ""
			}
		}
		require.NoError(t, migration.ValidateResponse(ConnectionToolSetGenerationMigrationActionKind))
		require.NoError(t, refresh.ValidateResponse(ConnectionToolSetRefreshActionKind))
		migration.Status, refresh.Status = nil, nil
		require.NoError(t, migration.ValidateRequest(ConnectionToolSetGenerationMigrationActionKind))
		require.NoError(t, refresh.ValidateRequest(ConnectionToolSetRefreshActionKind))
	}

	for _, target := range []bool{true, false} {
		for _, test := range []struct {
			name   string
			change func(*meta.ObjectReference)
			field  string
		}{
			{"version", func(r *meta.ObjectReference) { r.APIVersion = "authproxy.net/v2" }, ".apiVersion"},
			{"kind", func(r *meta.ObjectReference) { r.Kind = "Tool" }, ".kind"},
			{"wrong ID type", func(r *meta.ObjectReference) { r.ID = "tol_01example0000001" }, ".id"},
			{"missing identity", func(r *meta.ObjectReference) { r.ID = "" }, ""},
			{"name without namespace", func(r *meta.ObjectReference) { r.ID, r.Name = "", "calendar" }, ""},
			{"invalid namespace", func(r *meta.ObjectReference) { r.Namespace = "root..product" }, ".namespace"},
			{"invalid name", func(r *meta.ObjectReference) { r.Name = "invalid/name" }, ".name"},
		} {
			t.Run(test.name, func(t *testing.T) {
				migration, refresh := toolSetMigrationForTest(), toolSetRefreshForTest()
				migrationRef, refreshRef := &migration.Spec.ToolSetRef, &refresh.Spec.ToolSetRef
				path := "$.spec.toolSetRef"
				if target {
					migrationRef, refreshRef = &migration.Metadata.Target, &refresh.Metadata.Target
					path = "$.metadata.target"
				}
				test.change(migrationRef)
				test.change(refreshRef)
				require.ErrorContains(t, migration.ValidateResponse(ConnectionToolSetGenerationMigrationActionKind), path+test.field)
				require.ErrorContains(t, refresh.ValidateResponse(ConnectionToolSetRefreshActionKind), path+test.field)
				migration.Status, refresh.Status = nil, nil
				require.ErrorContains(t, migration.ValidateRequest(ConnectionToolSetGenerationMigrationActionKind), path+test.field)
				require.ErrorContains(t, refresh.ValidateRequest(ConnectionToolSetRefreshActionKind), path+test.field)
			})
		}
	}
}

// TestConnectionToolSetActionGenerationAndTimeout distinguishes an exact migration
// target from refresh while permitting rollback and positive operation timeouts.
func TestConnectionToolSetActionGenerationAndTimeout(t *testing.T) {
	migration, refresh := toolSetMigrationForTest(), toolSetRefreshForTest()
	for _, generation := range []uint64{1, 3, 100} {
		migration.Spec.ToolSetRef.Generation = generation
		require.NoError(t, migration.ValidateResponse(ConnectionToolSetGenerationMigrationActionKind))
	}
	migration.Spec.ToolSetRef.Generation = 0
	require.ErrorContains(t, migration.ValidateResponse(ConnectionToolSetGenerationMigrationActionKind), "spec.toolSetRef.generation")
	migration.Status = nil
	require.ErrorContains(t, migration.ValidateRequest(ConnectionToolSetGenerationMigrationActionKind), "spec.toolSetRef.generation")
	refresh.Spec.ToolSetRef.Generation = 1
	require.ErrorContains(t, refresh.ValidateResponse(ConnectionToolSetRefreshActionKind), "spec.toolSetRef.generation")
	refresh.Status = nil
	require.ErrorContains(t, refresh.ValidateRequest(ConnectionToolSetRefreshActionKind), "spec.toolSetRef.generation")

	for _, timeout := range []*int64{nil, util.ToPtr(int64(1)), util.ToPtr(int64(0)), util.ToPtr(int64(-1))} {
		migration, refresh = toolSetMigrationForTest(), toolSetRefreshForTest()
		migration.Spec.TimeoutSeconds, refresh.Spec.TimeoutSeconds = timeout, timeout
		for _, response := range []bool{true, false} {
			var migrationErr, refreshErr error
			if response {
				migrationErr = migration.ValidateResponse(ConnectionToolSetGenerationMigrationActionKind)
				refreshErr = refresh.ValidateResponse(ConnectionToolSetRefreshActionKind)
			} else {
				migration.Status, refresh.Status = nil, nil
				migrationErr = migration.ValidateRequest(ConnectionToolSetGenerationMigrationActionKind)
				refreshErr = refresh.ValidateRequest(ConnectionToolSetRefreshActionKind)
			}
			if timeout != nil && *timeout <= 0 {
				require.ErrorContains(t, migrationErr, "spec.timeoutSeconds")
				require.ErrorContains(t, refreshErr, "spec.timeoutSeconds")
			} else {
				require.NoError(t, migrationErr)
				require.NoError(t, refreshErr)
			}
		}
	}

	migration, refresh = toolSetMigrationForTest(), toolSetRefreshForTest()
	migration.Metadata.Target.Generation, refresh.Metadata.Target.Generation = 1, 1
	require.ErrorContains(t, migration.ValidateResponse(ConnectionToolSetGenerationMigrationActionKind), "metadata.target.generation")
	require.ErrorContains(t, refresh.ValidateResponse(ConnectionToolSetRefreshActionKind), "metadata.target.generation")
}

// TestConnectionToolSetActionEnvelope keeps task acknowledgments server-owned
// without treating their opaque token as an APID or asserting eventual success.
func TestConnectionToolSetActionEnvelope(t *testing.T) {
	require.Error(t, (*ConnectionToolSetGenerationMigrationAction)(nil).ValidateRequest(ConnectionToolSetGenerationMigrationActionKind))
	require.Error(t, (*ConnectionToolSetGenerationMigrationAction)(nil).ValidateResponse(ConnectionToolSetGenerationMigrationActionKind))
	require.Error(t, (*ConnectionToolSetRefreshAction)(nil).ValidateRequest(ConnectionToolSetRefreshActionKind))
	require.Error(t, (*ConnectionToolSetRefreshAction)(nil).ValidateResponse(ConnectionToolSetRefreshActionKind))
	for _, status := range []*ConnectionToolSetActionStatus{nil, {}, {TaskID: " \t"}, {TaskID: "opaque+/="}} {
		migration, refresh := toolSetMigrationForTest(), toolSetRefreshForTest()
		migration.Status, refresh.Status = status, status
		if status == nil {
			require.NoError(t, migration.ValidateRequest(ConnectionToolSetGenerationMigrationActionKind))
			require.NoError(t, refresh.ValidateRequest(ConnectionToolSetRefreshActionKind))
		} else {
			require.ErrorContains(t, migration.ValidateRequest(ConnectionToolSetGenerationMigrationActionKind), "status")
			require.ErrorContains(t, refresh.ValidateRequest(ConnectionToolSetRefreshActionKind), "status")
		}
		if status != nil && status.TaskID == "opaque+/=" {
			require.NoError(t, migration.ValidateResponse(ConnectionToolSetGenerationMigrationActionKind))
			require.NoError(t, refresh.ValidateResponse(ConnectionToolSetRefreshActionKind))
		} else {
			require.ErrorContains(t, migration.ValidateResponse(ConnectionToolSetGenerationMigrationActionKind), "status")
			require.ErrorContains(t, refresh.ValidateResponse(ConnectionToolSetRefreshActionKind), "status")
		}
	}

	for _, version := range []bool{false, true} {
		migration, refresh := toolSetMigrationForTest(), toolSetRefreshForTest()
		field := "kind"
		if version {
			migration.APIVersion, refresh.APIVersion = "authproxy.net/v2", "authproxy.net/v2"
			field = "apiVersion"
		} else {
			migration.Kind, refresh.Kind = ConnectionToolSetRefreshActionKind, ConnectionToolSetGenerationMigrationActionKind
		}
		require.ErrorContains(t, migration.ValidateResponse(ConnectionToolSetGenerationMigrationActionKind), field)
		require.ErrorContains(t, refresh.ValidateResponse(ConnectionToolSetRefreshActionKind), field)
		migration.Status, refresh.Status = nil, nil
		require.ErrorContains(t, migration.ValidateRequest(ConnectionToolSetGenerationMigrationActionKind), field)
		require.ErrorContains(t, refresh.ValidateRequest(ConnectionToolSetRefreshActionKind), field)
	}
}

// TestConnectionToolSetActionWireRoundTrip preserves safe request specifications
// and minimal task status, excluding internal binding and workflow state.
func TestConnectionToolSetActionWireRoundTrip(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"JSON", json.Marshal, util.DecodeJSONStrict},
		{"YAML", yaml.Marshal, util.DecodeYAMLStrict},
	} {
		t.Run(format.name, func(t *testing.T) {
			migration, refresh := toolSetMigrationForTest(), toolSetRefreshForTest()
			migration.Spec.TimeoutSeconds, refresh.Spec.TimeoutSeconds = util.ToPtr(int64(60)), util.ToPtr(int64(30))
			for _, response := range []bool{true, false} {
				if !response {
					migration.Status, refresh.Status = nil, nil
				}
				data, err := format.encode(migration)
				require.NoError(t, err)
				var decodedMigration ConnectionToolSetGenerationMigrationAction
				require.NoError(t, format.decode(data, &decodedMigration))
				require.Equal(t, migration, &decodedMigration)
				data, err = format.encode(refresh)
				require.NoError(t, err)
				var decodedRefresh ConnectionToolSetRefreshAction
				require.NoError(t, format.decode(data, &decodedRefresh))
				require.Equal(t, refresh, &decodedRefresh)
				if response {
					require.NoError(t, decodedMigration.ValidateResponse(ConnectionToolSetGenerationMigrationActionKind))
					require.NoError(t, decodedRefresh.ValidateResponse(ConnectionToolSetRefreshActionKind))
				} else {
					require.NoError(t, decodedMigration.ValidateRequest(ConnectionToolSetGenerationMigrationActionKind))
					require.NoError(t, decodedRefresh.ValidateRequest(ConnectionToolSetRefreshActionKind))
				}
			}
			for _, input := range []string{
				`{"spec":{"bindingId":"internal"}}`, `{"spec":{"toolSetRef":{"revision":1}}}`,
				`{"status":{"workflowId":"internal"}}`, `{"status":{"catalogRevision":1}}`,
				`{"metadata":{"target":{"configuration":{}}}}`, `{"spec":{"timeoutSeconds":{}}}`,
			} {
				var migration ConnectionToolSetGenerationMigrationAction
				var refresh ConnectionToolSetRefreshAction
				require.Error(t, format.decode([]byte(input), &migration), input)
				require.Error(t, format.decode([]byte(input), &refresh), input)
			}
		})
	}
}

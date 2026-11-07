package core

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/test_utils/connectorfixture"
	"github.com/stretchr/testify/require"
)

// enqueuedOfType returns the recorded tasks of one type, in enqueue order.
func (r *toolSetReconcileTest) enqueuedOfType(taskType string) []*asynq.Task {
	tasks := []*asynq.Task{}
	for _, task := range *r.enqueued {
		if task.Type() == taskType {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

// TestReconcileConnectionToolSets covers one connection's pass across the
// ToolSets that could select it.
func TestReconcileConnectionToolSets(t *testing.T) {
	calendar := map[string]string{"provider": "calendar"}

	t.Run("a connection created after its ToolSet's pass installs", func(t *testing.T) {
		r := newToolSetReconcileTest(t)
		id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
		require.NoError(t, r.service.reconcileToolSet(r.ctx, id))

		// The ToolSet pass ran before this connection existed.
		connection := r.connection(t, "root.team_a.sub", "late", database.ConnectionStateConfigured, calendar)
		require.NoError(t, r.service.reconcileConnectionToolSets(r.ctx, connection.Id))

		binding, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
		require.NoError(t, err)
		require.Equal(t, uint64(1), *binding.AppliedGeneration)
		require.Equal(t, "list", r.generatedTools(t, binding)[0].SourceKey)
	})

	t.Run("only ToolSets in the connection's namespace or its ancestors apply", func(t *testing.T) {
		r := newToolSetReconcileTest(t)

		// A ToolSet in root selects across everything, one in the connection's own
		// namespace selects it too, and one in a child namespace cannot reach it.
		ancestor := r.toolSetIn(t, "root", "root-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
		own := r.toolSetIn(t, "root.team_a", "own-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
		child := r.toolSetIn(t, "root.team_a.sub", "child-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
		connection := r.connection(t, "root.team_a", "owner", database.ConnectionStateConfigured, calendar)

		require.NoError(t, r.service.reconcileConnectionToolSets(r.ctx, connection.Id))

		for _, id := range []apid.ID{ancestor, own} {
			binding, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
			require.NoError(t, err)
			require.NotNil(t, binding.AppliedGeneration)
		}
		_, err := r.db.GetToolSetBindingForConnection(r.ctx, child, connection.Id)
		require.ErrorIs(t, err, database.ErrNotFound)
	})

	t.Run("setup waits, then completing setup installs", func(t *testing.T) {
		r := newToolSetReconcileTest(t)
		id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
		connection := r.connection(t, "root.team_a", "owner", database.ConnectionStateSetup, calendar)

		// During setup the connection gains a pending binding only.
		require.NoError(t, r.service.reconcileConnectionToolSets(r.ctx, connection.Id))
		pending, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
		require.NoError(t, err)
		require.Nil(t, pending.AppliedGeneration)

		// Once configured, the same binding installs the primary.
		require.NoError(t, r.db.SetConnectionState(r.ctx, connection.Id, database.ConnectionStateConfigured))
		require.NoError(t, r.service.reconcileConnectionToolSets(r.ctx, connection.Id))
		installed, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
		require.NoError(t, err)
		require.Equal(t, pending.Id, installed.Id)
		require.Equal(t, uint64(1), *installed.AppliedGeneration)
	})

	t.Run("a label change removes only the bindings that stop matching", func(t *testing.T) {
		r := newToolSetReconcileTest(t)
		calendarTools := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
		connection := r.connection(t, "root.team_a", "owner", database.ConnectionStateConfigured, calendar)

		// A second ToolSet selects every connection in the namespace, whatever its labels.
		everything, err := r.service.CreateToolSet(r.ctx, &toolsetschema.ToolSet{
			TypeMeta: meta.NewTypeMeta(toolsetschema.ToolSetKind),
			Metadata: meta.ObjectMeta{Namespace: "root.team_a", Name: "everything"},
			Spec: toolsetschema.ToolSetSpec{
				ConnectionSelector: &toolsetschema.ConnectionSelector{MatchLabels: map[string]string{}},
				Release:            toolsetschema.ToolSetReleaseSpec{DesiredState: toolsetschema.ToolSetReleaseStatePrimary},
				Definition: toolsetschema.ToolSetDefinition{Source: toolsetschema.ToolSetSource{
					Explicit: &toolsetschema.ExplicitSource{Tools: []toolsetschema.ToolTemplate{toolTemplate("search")}},
				}},
			},
		})
		require.NoError(t, err)
		require.NoError(t, r.service.reconcileConnectionToolSets(r.ctx, connection.Id))
		removed, err := r.db.GetToolSetBindingForConnection(r.ctx, calendarTools, connection.Id)
		require.NoError(t, err)
		require.Len(t, r.generatedTools(t, removed), 1)

		// Relabel the connection so only the label-agnostic ToolSet still selects it.
		_, err = r.db.UpdateConnectionLabels(r.ctx, connection.Id, map[string]string{"provider": "crm"})
		require.NoError(t, err)
		require.NoError(t, r.service.reconcileConnectionToolSets(r.ctx, connection.Id))

		// The calendar binding and its Tools are gone; the other binding remains.
		_, err = r.db.GetToolSetBindingForConnection(r.ctx, calendarTools, connection.Id)
		require.ErrorIs(t, err, database.ErrNotFound)
		require.Empty(t, r.generatedTools(t, removed))
		kept, err := r.db.GetToolSetBindingForConnection(r.ctx, everything.GetId(), connection.Id)
		require.NoError(t, err)
		require.Len(t, r.generatedTools(t, kept), 1)
	})

	t.Run("a selected connection that cannot bind keeps its binding", func(t *testing.T) {
		r := newToolSetReconcileTest(t)
		id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
		connection := r.connection(t, "root.team_a", "owner", database.ConnectionStateConfigured, calendar)
		require.NoError(t, r.service.reconcileConnectionToolSets(r.ctx, connection.Id))

		// Disconnecting still matches the selector, so nothing is removed here;
		// parent cleanup handles the binding once the connection is deleted.
		require.NoError(t, r.db.SetConnectionState(r.ctx, connection.Id, database.ConnectionStateDisconnecting))
		require.NoError(t, r.service.reconcileConnectionToolSets(r.ctx, connection.Id))
		binding, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
		require.NoError(t, err)
		require.Len(t, r.generatedTools(t, binding), 1)
	})

	t.Run("a deleted connection is a no-op", func(t *testing.T) {
		r := newToolSetReconcileTest(t)
		r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
		connection := r.connection(t, "root.team_a", "owner", database.ConnectionStateConfigured, calendar)
		require.NoError(t, r.db.DeleteConnection(r.ctx, connection.Id))

		require.NoError(t, r.service.reconcileConnectionToolSets(r.ctx, connection.Id))
	})
}

// connectionPatch builds a complete connection patch around a metadata change.
func connectionPatch(metadata meta.ObjectMetaPatch) *connectionschema.ConnectionPatch {
	return &connectionschema.ConnectionPatch{
		TypeMeta: meta.NewTypeMeta(connectionschema.ConnectionKind),
		Metadata: &metadata,
		Spec:     &connectionschema.ConnectionSpecPatch{},
	}
}

// TestConnectionChangesEnqueueToolSetReconcile checks which connection writes
// request a pass, and that the request outlives a canceled caller.
func TestConnectionChangesEnqueueToolSetReconcile(t *testing.T) {
	r := newToolSetReconcileTest(t)

	// Core connection writes load the connection's connector, so seed a real one.
	connectorId := apid.New(apid.PrefixConnector)
	connectorfixture.Seed(t, r.db, r.encrypt, cschema.Connector{
		TypeMeta: meta.NewTypeMeta(cschema.ConnectorKind),
		Metadata: meta.ObjectMeta{ID: connectorId.String(), Namespace: "root", Generation: 1},
		Spec: cschema.ConnectorSpec{Definition: cschema.ConnectorDefinition{
			DisplayName: "Calendar",
			Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		}},
	})
	connectionId := apid.New(apid.PrefixConnection)
	require.NoError(t, r.db.CreateConnection(r.ctx, &database.Connection{
		Id:                  connectionId,
		Namespace:           "root.team_a",
		Name:                "owner",
		State:               database.ConnectionStateSetup,
		ConnectorId:         connectorId,
		ConnectorGeneration: 1,
	}))
	connection, err := r.service.getConnection(r.ctx, connectionId)
	require.NoError(t, err)

	// Record whether each enqueue could still reach the queue.
	enqueueErrs := []error{}
	r.onEnqueue = func(ctx context.Context, _ *asynq.Task) { enqueueErrs = append(enqueueErrs, ctx.Err()) }
	requested := func() int { return len(r.enqueuedOfType(taskTypeReconcileConnectionToolSets)) }

	// Completing setup requests a pass naming the connection.
	require.NoError(t, connection.SetState(r.ctx, database.ConnectionStateConfigured))
	require.Equal(t, 1, requested())
	require.Contains(t, string(r.enqueuedOfType(taskTypeReconcileConnectionToolSets)[0].Payload()), connectionId.String())

	// Name and annotation changes cannot affect selection, so they request nothing.
	name := scommon.ResourceName("renamed")
	annotations := map[string]string{"note": "changed"}
	_, err = r.service.UpdateConnection(r.ctx, connectionId, connectionPatch(meta.ObjectMetaPatch{Name: &name, Annotations: &annotations}))
	require.NoError(t, err)
	require.Equal(t, 1, requested())

	// A label change requests another pass.
	labels := map[string]string{"provider": "calendar"}
	_, err = r.service.UpdateConnection(r.ctx, connectionId, connectionPatch(meta.ObjectMetaPatch{Labels: &labels}))
	require.NoError(t, err)
	require.Equal(t, 2, requested())

	// Every request was made on a context the queue could still use.
	for _, err := range enqueueErrs {
		require.NoError(t, err)
	}
}

// TestSweepToolSets covers the periodic repair fan-out.
func TestSweepToolSets(t *testing.T) {
	r := newToolSetReconcileTest(t)

	// Two live ToolSets in different namespaces, and one deleted ToolSet.
	first := r.toolSetIn(t, "root", "root-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
	second := r.toolSetIn(t, "root.team_a.sub", "sub-tools", toolsetschema.ToolSetReleaseStateDraft, toolTemplate("list"))
	deleted := r.toolSet(t, "deleted-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
	require.NoError(t, r.service.DeleteToolSet(r.ctx, deleted))
	*r.enqueued = nil

	// The sweep requests one pass per live ToolSet.
	require.NoError(t, r.service.sweepToolSets(r.ctx))
	swept := []string{}
	for _, task := range r.enqueuedOfType(taskTypeReconcileToolSet) {
		swept = append(swept, string(task.Payload()))
	}
	require.Len(t, swept, 2)
	require.Contains(t, swept[0]+swept[1], first.String())
	require.Contains(t, swept[0]+swept[1], second.String())
	require.NotContains(t, swept[0]+swept[1], deleted.String())

	// The sweep's own task needs no payload, and runs through its handler.
	*r.enqueued = nil
	require.NoError(t, r.service.sweepToolSetsTask(r.ctx, newSweepToolSetsTask()))
	require.Len(t, r.enqueuedOfType(taskTypeReconcileToolSet), 2)
}

// TestReconcileConnectionToolSetsTask covers payload handling in the asynq handler.
func TestReconcileConnectionToolSetsTask(t *testing.T) {
	r := newToolSetReconcileTest(t)

	// Undecodable or empty payloads can never succeed, so they skip retries.
	err := r.service.reconcileConnectionToolSetsTask(r.ctx, asynq.NewTask(taskTypeReconcileConnectionToolSets, []byte("{")))
	require.True(t, errors.Is(err, asynq.SkipRetry))
	err = r.service.reconcileConnectionToolSetsTask(r.ctx, asynq.NewTask(taskTypeReconcileConnectionToolSets, []byte(`{}`)))
	require.True(t, errors.Is(err, asynq.SkipRetry))

	// A valid payload runs the pass.
	id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
	connection := r.connection(t, "root.team_a", "owner", database.ConnectionStateConfigured, map[string]string{"provider": "calendar"})
	task, err := newReconcileConnectionToolSetsTask(connection.Id)
	require.NoError(t, err)
	require.NoError(t, r.service.reconcileConnectionToolSetsTask(r.ctx, task))
	_, err = r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
	require.NoError(t, err)
}

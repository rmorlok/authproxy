package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	asynqmock "github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	dbmock "github.com/rmorlok/authproxy/internal/database/mock"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/stretchr/testify/require"
)

// TestReconcileToolSetsAfterConnectionMigration retries pending assignments
// across parent namespaces without migrating any installed ToolSet inventory.
func TestReconcileToolSetsAfterConnectionMigration(t *testing.T) {
	r := newToolSetReconcileTest(t)
	connection := r.connection(t, "root.team_a.sub", "owner", database.ConnectionStateConfigured, map[string]string{"provider": "calendar"})

	// A child connection has two pending parent ToolSets and one installed one.
	pendingIDs := []apid.ID{
		r.toolSet(t, "pending-a", toolsetschema.ToolSetReleaseStateDraft, toolTemplate("list")),
		r.toolSet(t, "pending-b", toolsetschema.ToolSetReleaseStateDraft, toolTemplate("list")),
	}
	installedID := r.toolSet(t, "installed", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
	for _, id := range append(append([]apid.ID{}, pendingIDs...), installedID) {
		require.NoError(t, r.service.reconcileToolSet(r.ctx, id))
	}
	installedBefore, err := r.db.GetToolSetBindingForConnection(r.ctx, installedID, connection.Id)
	require.NoError(t, err)
	*r.enqueued = nil

	// An apply attempt that never reached its target does not trigger installs;
	// a connection removed since apply also needs no recovery.
	require.NoError(t, r.service.reconcileToolSetsAfterConnectionMigrationV1(r.ctx, connection.Id, connection.ConnectorGeneration+1))
	require.NoError(t, r.service.reconcileToolSetsAfterConnectionMigrationV1(r.ctx, apid.New(apid.PrefixConnection), 1))
	require.Empty(t, *r.enqueued)

	// A transient queue failure returns to the workflow. The retry may repeat
	// an earlier successful enqueue, which is safe for ToolSet reconciliation.
	queueErr := errors.New("queue unavailable")
	queuedIDs := []apid.ID{}
	attempts := 0
	ac := asynqmock.NewMockClient(gomock.NewController(t))
	ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
			attempts++
			if attempts == 2 {
				return nil, queueErr
			}
			require.Equal(t, taskTypeReconcileToolSet, task.Type())
			var payload reconcileToolSetTaskPayload
			require.NoError(t, json.Unmarshal(task.Payload(), &payload))
			queuedIDs = append(queuedIDs, payload.ToolSetId)
			return nil, nil
		},
	).Times(4)
	r.service.ac = ac
	require.ErrorIs(t, r.service.reconcileToolSetsAfterConnectionMigrationV1(r.ctx, connection.Id, connection.ConnectorGeneration), queueErr)
	require.NoError(t, r.service.reconcileToolSetsAfterConnectionMigrationV1(r.ctx, connection.Id, connection.ConnectorGeneration))

	// Both pending ToolSets are eventually queued; the installed binding and
	// inventory counters never change as a side effect of the migration trigger.
	require.Len(t, queuedIDs, 3)
	require.Equal(t, queuedIDs[0], queuedIDs[1])
	require.ElementsMatch(t, pendingIDs, queuedIDs[1:])
	installedAfter, err := r.db.GetToolSetBindingForConnection(r.ctx, installedID, connection.Id)
	require.NoError(t, err)
	require.Equal(t, installedBefore, installedAfter)
}

// TestReconcileToolSetsAfterConnectionMigrationLookupFailure ensures transient
// reads remain retryable rather than being mistaken for an uncommitted migration.
func TestReconcileToolSetsAfterConnectionMigrationLookupFailure(t *testing.T) {
	connectionID := apid.New(apid.PrefixConnection)
	dbErr := errors.New("database unavailable")
	db := dbmock.NewMockDB(gomock.NewController(t))
	db.EXPECT().GetConnection(gomock.Any(), connectionID).Return(nil, dbErr)
	s := &service{db: db}

	// No enqueue client is needed because a failed read cannot authorize work.
	require.ErrorIs(t, s.reconcileToolSetsAfterConnectionMigrationV1(context.Background(), connectionID, 2), dbErr)
}

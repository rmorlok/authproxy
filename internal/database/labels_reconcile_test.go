package database

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	clock "k8s.io/utils/clock/testing"
)

// staleLabels is drifted content for a labelled row: valid JSON that lacks the
// row's implicit labels, so a recompute always rewrites it.
const staleLabels = `{"apxy/stale":"1"}`

// reconcileTestCtx returns a context with a fixed clock for reconcile tests.
func reconcileTestCtx() context.Context {
	now := time.Date(2024, time.March, 1, 12, 0, 0, 0, time.UTC)
	return apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()
}

// seedReconcileKeys creates count keys in the root namespace and returns their ids.
func seedReconcileKeys(t *testing.T, ctx context.Context, db DB, count int) []apid.ID {
	t.Helper()
	ids := make([]apid.ID, 0, count)
	for i := 0; i < count; i++ {
		id := apid.New(apid.PrefixKey)
		require.NoError(t, db.CreateKey(ctx, &Key{Id: id, Namespace: "root", State: KeyStateActive}))
		ids = append(ids, id)
	}
	return ids
}

// corruptAllKeys drifts every key, bypassing the write paths that keep labels fresh.
func corruptAllKeys(t *testing.T, rawDb *sql.DB) {
	t.Helper()
	_, err := rawDb.Exec(fmt.Sprintf(`UPDATE keys SET labels = '%s'`, staleLabels))
	require.NoError(t, err)
}

// corruptKey drifts a single key.
func corruptKey(t *testing.T, rawDb *sql.DB, id string) {
	t.Helper()
	_, err := rawDb.Exec(fmt.Sprintf(`UPDATE keys SET labels = '%s' WHERE id = '%s'`, staleLabels, id))
	require.NoError(t, err)
}

// countRows returns the number of live rows in table.
func countRows(t *testing.T, rawDb *sql.DB, table string) int {
	t.Helper()
	var count int
	require.NoError(t, rawDb.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE deleted_at IS NULL`, table)).Scan(&count))
	return count
}

func TestReconcileCarryForwardLabelsCoverage(t *testing.T) {
	t.Run("repairs every row when every visit writes", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// Writes move rows on PostgreSQL, which made offset paging skip them.
		seedReconcileKeys(t, ctx, db, 250)
		total := countRows(t, rawDb, KeysTable)
		corruptAllKeys(t, rawDb)

		// A single pass finds and repairs every drifted row exactly once.
		fixed, err := db.ReconcileCarryForwardLabels(ctx, 50, nil)
		require.NoError(t, err)
		require.Equal(t, int64(total), fixed)

		// Nothing is left for a second pass to find.
		fixed, err = db.ReconcileCarryForwardLabels(ctx, 50, nil)
		require.NoError(t, err)
		require.Zero(t, fixed)
	})

	t.Run("repairs a namespace subtree in one pass", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// Parents and children are created with their carry-forward labels in place.
		paths := []string{"root.a", "root.a.b", "root.a.b.c", "root.a-sibling", "root.a1"}
		for _, path := range paths {
			require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: path, State: NamespaceStateActive}))
		}
		_, err := db.ReconcileCarryForwardLabels(ctx, 2, nil)
		require.NoError(t, err)

		// Changing the top row's user labels leaves its whole subtree stale.
		_, err = rawDb.Exec(`UPDATE namespaces SET labels = '{"team":"changed"}' WHERE path = 'root.a'`)
		require.NoError(t, err)

		// One pass reaches each parent before its child, so the label arrives at the leaf.
		_, err = db.ReconcileCarryForwardLabels(ctx, 2, nil)
		require.NoError(t, err)
		leaf, err := db.GetNamespace(ctx, "root.a.b.c")
		require.NoError(t, err)
		require.Equal(t, "changed", leaf.Labels["apxy/ns/team"])

		// A second pass confirms the first left nothing stale.
		fixed, err := db.ReconcileCarryForwardLabels(ctx, 2, nil)
		require.NoError(t, err)
		require.Zero(t, fixed)
	})
}

func TestReconcileCarryForwardLabelsCheckpoint(t *testing.T) {
	// interruptedAfterKeys runs a sweep with batch size 10 that stops once it has
	// processed the given number of keys, standing in for a run that ends early.
	interruptedAfterKeys := func(t *testing.T, ctx context.Context, db DB, rawDb *sql.DB, keys int) {
		t.Helper()
		earlierRows := countRows(t, rawDb, NamespacesTable) +
			countRows(t, rawDb, ConnectorsTable) +
			countRows(t, rawDb, ConnectionsTable) +
			countRows(t, rawDb, ActorTable)

		// With no refill, Wait fails at once when the burst is spent and it is
		// given a deadline it can never meet.
		runCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		limiter := rate.NewLimiter(0, earlierRows+keys)
		_, err := db.ReconcileCarryForwardLabels(runCtx, 10, limiter)
		require.ErrorContains(t, err, "reconcile keys")
	}

	t.Run("a later run resumes after finished work and then finishes the sweep", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()
		s := db.(*service)

		// Clear existing drift so that only the rows corrupted below count.
		seedReconcileKeys(t, ctx, db, 30)
		_, err := db.ReconcileCarryForwardLabels(ctx, 10, nil)
		require.NoError(t, err)
		total := countRows(t, rawDb, KeysTable)
		corruptAllKeys(t, rawDb)

		// The run stops midway through the third page of keys.
		interruptedAfterKeys(t, ctx, db, rawDb, 25)

		// Progress stops at the last full page, so at most one batch is repeated.
		checkpoint, err := s.getTaskCheckpoint(ctx, reconcileCarryForwardLabelsCheckpoint)
		require.NoError(t, err)
		require.NotNil(t, checkpoint)
		require.Equal(t, "keys", checkpoint.Step)
		require.NotEmpty(t, checkpoint.LastKey)

		// Drift behind the checkpoint must be left alone by the resumed run.
		corruptKey(t, rawDb, checkpoint.LastKey)

		// The resumed run repeats the interrupted page, whose first 5 rows are
		// already clean, and repairs only the rows the first run never reached.
		fixed, err := db.ReconcileCarryForwardLabels(ctx, 10, nil)
		require.NoError(t, err)
		require.Equal(t, int64(total-25), fixed)

		// Finishing the last step clears the checkpoint.
		checkpoint, err = s.getTaskCheckpoint(ctx, reconcileCarryForwardLabelsCheckpoint)
		require.NoError(t, err)
		require.Nil(t, checkpoint)

		// The following run starts over and finds the drift the resumed run skipped.
		fixed, err = db.ReconcileCarryForwardLabels(ctx, 10, nil)
		require.NoError(t, err)
		require.Equal(t, int64(1), fixed)
	})

	t.Run("resumes at the saved step without revisiting earlier steps", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()
		s := db.(*service)

		// Drift one row in an earlier step and one in the saved step.
		ids := seedReconcileKeys(t, ctx, db, 1)
		require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.early", State: NamespaceStateActive}))
		_, err := db.ReconcileCarryForwardLabels(ctx, 10, nil)
		require.NoError(t, err)
		_, err = rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET labels = '%s' WHERE path = 'root.early'`, staleLabels))
		require.NoError(t, err)
		corruptKey(t, rawDb, ids[0].String())

		// A checkpoint at the start of the keys step skips only namespaces.
		require.NoError(t, s.saveTaskCheckpoint(ctx, reconcileCarryForwardLabelsCheckpoint, "keys", ""))
		fixed, err := db.ReconcileCarryForwardLabels(ctx, 10, nil)
		require.NoError(t, err)
		require.Equal(t, int64(1), fixed)

		// With the sweep finished, the next run reaches the namespace.
		fixed, err = db.ReconcileCarryForwardLabels(ctx, 10, nil)
		require.NoError(t, err)
		require.Equal(t, int64(1), fixed)
	})

	t.Run("a checkpoint for an unknown step restarts the sweep", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()
		s := db.(*service)

		// Rows are drifted, and the checkpoint names a step no release has.
		seedReconcileKeys(t, ctx, db, 3)
		total := countRows(t, rawDb, KeysTable)
		corruptAllKeys(t, rawDb)
		require.NoError(t, s.saveTaskCheckpoint(ctx, reconcileCarryForwardLabelsCheckpoint, "retired", "zzz"))

		// The whole sweep runs, ignoring the checkpoint's key.
		fixed, err := db.ReconcileCarryForwardLabels(ctx, 10, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, fixed, int64(total))
	})

	t.Run("saving replaces earlier progress", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := reconcileTestCtx()
		s := db.(*service)

		// A missing checkpoint reads as nil rather than an error.
		checkpoint, err := s.getTaskCheckpoint(ctx, "sweep")
		require.NoError(t, err)
		require.Nil(t, checkpoint)

		// A second save overwrites the first.
		require.NoError(t, s.saveTaskCheckpoint(ctx, "sweep", "first", "a"))
		require.NoError(t, s.saveTaskCheckpoint(ctx, "sweep", "second", "b"))
		checkpoint, err = s.getTaskCheckpoint(ctx, "sweep")
		require.NoError(t, err)
		require.Equal(t, "second", checkpoint.Step)
		require.Equal(t, "b", checkpoint.LastKey)
		require.True(t, apctx.GetClock(ctx).Now().Equal(checkpoint.UpdatedAt))

		// Deleting is repeatable and leaves no progress behind.
		require.NoError(t, s.deleteTaskCheckpoint(ctx, "sweep"))
		require.NoError(t, s.deleteTaskCheckpoint(ctx, "sweep"))
		checkpoint, err = s.getTaskCheckpoint(ctx, "sweep")
		require.NoError(t, err)
		require.Nil(t, checkpoint)
	})
}

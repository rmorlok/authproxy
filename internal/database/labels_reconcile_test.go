package database

import (
	"context"
	"database/sql"
	"encoding/json"
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

// testReconcileCheckpointKey is the checkpoint key used by reconcile tests.
var testReconcileCheckpointKey = TaskCheckpointKey{TaskType: "test:reconcile"}

// testReconcileCheckpointer returns a checkpointer for one run of the sweep, as
// the reconcile task creates for each of its runs.
func testReconcileCheckpointer(db DB) *ReconcileCheckpointer {
	return NewTaskCheckpointer[ReconcilePosition](db, testReconcileCheckpointKey, time.Hour)
}

// rivalRunDB saves progress as an overlapping run immediately after the checkpoint
// is read, so the run under test is stale from its first save.
type rivalRunDB struct {
	DB
	rival func(ctx context.Context)
}

// GetTaskCheckpoint reads the checkpoint, then lets the rival run overtake the caller.
func (d *rivalRunDB) GetTaskCheckpoint(ctx context.Context, key TaskCheckpointKey) (*TaskCheckpoint, error) {
	checkpoint, err := d.DB.GetTaskCheckpoint(ctx, key)
	d.rival(ctx)
	return checkpoint, err
}

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
		fixed, err := db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 50, nil)
		require.NoError(t, err)
		require.Equal(t, int64(total), fixed)

		// Nothing is left for a second pass to find.
		fixed, err = db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 50, nil)
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
		_, err := db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 2, nil)
		require.NoError(t, err)

		// Changing the top row's user labels leaves its whole subtree stale.
		_, err = rawDb.Exec(`UPDATE namespaces SET labels = '{"team":"changed"}' WHERE path = 'root.a'`)
		require.NoError(t, err)

		// One pass reaches each parent before its child, so the label arrives at the leaf.
		_, err = db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 2, nil)
		require.NoError(t, err)
		leaf, err := db.GetNamespace(ctx, "root.a.b.c")
		require.NoError(t, err)
		require.Equal(t, "changed", leaf.Labels["apxy/ns/team"])

		// A second pass confirms the first left nothing stale.
		fixed, err := db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 2, nil)
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
		_, err := db.ReconcileCarryForwardLabels(runCtx, testReconcileCheckpointer(db), 10, limiter)
		require.ErrorContains(t, err, "reconcile keys")
	}

	t.Run("a later run resumes after finished work and then finishes the sweep", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// Clear existing drift so that only the rows corrupted below count.
		seedReconcileKeys(t, ctx, db, 30)
		_, err := db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 10, nil)
		require.NoError(t, err)
		total := countRows(t, rawDb, KeysTable)
		corruptAllKeys(t, rawDb)

		// The run stops midway through the third page of keys.
		interruptedAfterKeys(t, ctx, db, rawDb, 25)

		// Progress stops at the last full page, so at most one batch is repeated.
		position, err := testReconcileCheckpointer(db).Load(ctx)
		require.NoError(t, err)
		require.NotNil(t, position)
		require.Equal(t, "keys", position.Step)
		require.NotEmpty(t, position.AfterKey)

		// Drift behind the checkpoint must be left alone by the resumed run.
		corruptKey(t, rawDb, position.AfterKey)

		// The resumed run repeats the interrupted page, whose first 5 rows are
		// already clean, and repairs only the rows the first run never reached.
		fixed, err := db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 10, nil)
		require.NoError(t, err)
		require.Equal(t, int64(total-25), fixed)

		// Finishing the last step clears the checkpoint.
		position, err = testReconcileCheckpointer(db).Load(ctx)
		require.NoError(t, err)
		require.Nil(t, position)

		// The following run starts over and finds the drift the resumed run skipped.
		fixed, err = db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 10, nil)
		require.NoError(t, err)
		require.Equal(t, int64(1), fixed)
	})

	t.Run("resumes at the saved step without revisiting earlier steps", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// Drift one row in an earlier step and one in the saved step.
		ids := seedReconcileKeys(t, ctx, db, 1)
		require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.early", State: NamespaceStateActive}))
		_, err := db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 10, nil)
		require.NoError(t, err)
		_, err = rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET labels = '%s' WHERE path = 'root.early'`, staleLabels))
		require.NoError(t, err)
		corruptKey(t, rawDb, ids[0].String())

		// A checkpoint at the start of the keys step skips only namespaces.
		require.NoError(t, testReconcileCheckpointer(db).Save(ctx, ReconcilePosition{Step: "keys"}))
		fixed, err := db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 10, nil)
		require.NoError(t, err)
		require.Equal(t, int64(1), fixed)

		// With the sweep finished, the next run reaches the namespace.
		fixed, err = db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 10, nil)
		require.NoError(t, err)
		require.Equal(t, int64(1), fixed)
	})

	t.Run("a checkpoint for an unknown step restarts the sweep", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// Rows are drifted, and the checkpoint names a step no release has.
		seedReconcileKeys(t, ctx, db, 3)
		total := countRows(t, rawDb, KeysTable)
		corruptAllKeys(t, rawDb)
		require.NoError(t, testReconcileCheckpointer(db).Save(ctx, ReconcilePosition{Step: "retired", AfterKey: "zzz"}))

		// The whole sweep runs, ignoring the checkpoint's key.
		fixed, err := db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 10, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, fixed, int64(total))
	})

	t.Run("an unreadable checkpoint restarts the sweep and is replaced", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// Rows are drifted, and the stored payload has a shape no release can decode.
		seedReconcileKeys(t, ctx, db, 3)
		total := countRows(t, rawDb, KeysTable)
		corruptAllKeys(t, rawDb)
		_, err := db.SaveTaskCheckpoint(ctx, testReconcileCheckpointKey, "", json.RawMessage(`["old format"]`), time.Hour)
		require.NoError(t, err)

		// The whole sweep runs and its saves replace the checkpoint, which it then clears.
		fixed, err := db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 10, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, fixed, int64(total))
		_, err = db.GetTaskCheckpoint(ctx, testReconcileCheckpointKey)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("a run overtaken by an overlapping run stops and leaves its progress", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// An earlier run left progress at the keys step.
		require.NoError(t, testReconcileCheckpointer(db).Save(ctx, ReconcilePosition{Step: "keys"}))

		// A rival advances the sweep just after the run under test reads the checkpoint.
		overlapping := &rivalRunDB{DB: db, rival: func(ctx context.Context) {
			rival := testReconcileCheckpointer(db)
			_, err := rival.Load(ctx)
			require.NoError(t, err)
			require.NoError(t, rival.Save(ctx, ReconcilePosition{Step: "tools", AfterKey: "tol_rival"}))
		}}

		// The run under test finds it no longer owns the sweep at its first save.
		_, err := db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(overlapping), 10, nil)
		require.ErrorIs(t, err, ErrTaskCheckpointConflict)

		// The rival's position survives, so the sweep continues from there.
		position, err := testReconcileCheckpointer(db).Load(ctx)
		require.NoError(t, err)
		require.Equal(t, ReconcilePosition{Step: "tools", AfterKey: "tol_rival"}, *position)
	})
}

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

// reconcileFullSweep drives slices from the start until the sweep is done, as the
// sweep workflow does, and returns the total rows corrected.
func reconcileFullSweep(db DB, ctx context.Context, batchSize int32, limiter *rate.Limiter) (int64, error) {
	var total int64
	var position ReconcilePosition
	for {
		slice, err := db.ReconcileCarryForwardLabelsSlice(ctx, position, int(batchSize), batchSize, limiter)
		total += slice.Corrected
		if err != nil {
			return total, err
		}
		if slice.Done {
			return total, nil
		}
		position = slice.Next
	}
}

// reconcileTestCtx returns a context with a fixed clock for reconcile tests.
func reconcileTestCtx() context.Context {
	now := time.Date(2024, time.March, 1, 12, 0, 0, 0, time.UTC)
	return apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()
}

// seedReconcileKeys creates count keys in the root namespace.
func seedReconcileKeys(t *testing.T, ctx context.Context, db DB, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		id := apid.New(apid.PrefixKey)
		require.NoError(t, db.CreateKey(ctx, &Key{Id: id, Namespace: "root", State: KeyStateActive}))
	}
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

// countKeys returns the number of live keys.
func countKeys(t *testing.T, rawDb *sql.DB) int {
	t.Helper()
	var count int
	require.NoError(t, rawDb.QueryRow(`SELECT COUNT(*) FROM keys WHERE deleted_at IS NULL`).Scan(&count))
	return count
}

func TestReconcileCarryForwardLabelsCoverage(t *testing.T) {
	t.Run("repairs every row when every visit writes", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// Writes move rows on PostgreSQL, which made offset paging skip them.
		seedReconcileKeys(t, ctx, db, 250)
		total := countKeys(t, rawDb)
		corruptAllKeys(t, rawDb)

		// A single sweep finds and repairs every drifted row exactly once.
		fixed, err := reconcileFullSweep(db, ctx, 50, nil)
		require.NoError(t, err)
		require.Equal(t, int64(total), fixed)

		// Nothing is left for a second sweep to find.
		fixed, err = reconcileFullSweep(db, ctx, 50, nil)
		require.NoError(t, err)
		require.Zero(t, fixed)
	})

	t.Run("repairs a namespace subtree in one sweep", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// Parents and children are created with their carry-forward labels in place.
		paths := []string{"root.a", "root.a.b", "root.a.b.c", "root.a-sibling", "root.a1"}
		for _, path := range paths {
			require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: path, State: NamespaceStateActive}))
		}
		_, err := reconcileFullSweep(db, ctx, 2, nil)
		require.NoError(t, err)

		// Changing the top row's user labels leaves its whole subtree stale.
		_, err = rawDb.Exec(`UPDATE namespaces SET labels = '{"team":"changed"}' WHERE path = 'root.a'`)
		require.NoError(t, err)

		// One sweep reaches each parent before its child, so the label arrives at the leaf.
		_, err = reconcileFullSweep(db, ctx, 2, nil)
		require.NoError(t, err)
		leaf, err := db.GetNamespace(ctx, "root.a.b.c")
		require.NoError(t, err)
		require.Equal(t, "changed", leaf.Labels["apxy/ns/team"])

		// A second sweep confirms the first left nothing stale.
		fixed, err := reconcileFullSweep(db, ctx, 2, nil)
		require.NoError(t, err)
		require.Zero(t, fixed)
	})
}

func TestReconcileCarryForwardLabelsSlice(t *testing.T) {
	t.Run("stops at a page boundary and continues from its position", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// Clear existing drift so that only the rows corrupted below count.
		seedReconcileKeys(t, ctx, db, 30)
		_, err := reconcileFullSweep(db, ctx, 10, nil)
		require.NoError(t, err)
		total := countKeys(t, rawDb)
		corruptAllKeys(t, rawDb)

		// Enter the keys step directly, then ask for less than a page of rows.
		first, err := db.ReconcileCarryForwardLabelsSlice(ctx, ReconcilePosition{Step: "keys"}, 15, 10, nil)
		require.NoError(t, err)

		// The slice finishes the page that crosses maxRows and stops there.
		require.False(t, first.Done)
		require.Equal(t, int64(20), first.Corrected)
		require.Equal(t, "keys", first.Next.Step)
		require.NotEmpty(t, first.Next.AfterKey)

		// Drift behind the position must be left alone by the next slice.
		corruptKey(t, rawDb, first.Next.AfterKey)

		// Continuing repairs only the rows not reached yet.
		second, err := db.ReconcileCarryForwardLabelsSlice(ctx, first.Next, 1000, 10, nil)
		require.NoError(t, err)
		require.True(t, second.Done)
		require.Equal(t, int64(total-20), second.Corrected)

		// A new sweep starts from the top and finds the skipped drift.
		fixed, err := reconcileFullSweep(db, ctx, 10, nil)
		require.NoError(t, err)
		require.Equal(t, int64(1), fixed)
	})

	t.Run("ends between steps when the rows run out at a step boundary", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// The only namespace fits on a short page, which uses up the whole budget.
		slice, err := db.ReconcileCarryForwardLabelsSlice(ctx, ReconcilePosition{}, 1, 10, nil)
		require.NoError(t, err)

		// The next slice starts the following step rather than repeating this one.
		require.False(t, slice.Done)
		require.Equal(t, ReconcilePosition{Step: "connectors"}, slice.Next)
	})

	t.Run("a position in a later step skips earlier steps", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// Drift one namespace, whose step comes before the position.
		require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.early", State: NamespaceStateActive}))
		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET labels = '%s' WHERE path = 'root.early'`, staleLabels))
		require.NoError(t, err)

		// The slice from the keys step finishes without touching it.
		slice, err := db.ReconcileCarryForwardLabelsSlice(ctx, ReconcilePosition{Step: "keys"}, 1000, 10, nil)
		require.NoError(t, err)
		require.True(t, slice.Done)

		// A sweep from the start does reach it.
		fixed, err := reconcileFullSweep(db, ctx, 10, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, fixed, int64(1))
	})

	t.Run("a position for an unknown step restarts the sweep", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// Rows are drifted, and the position names a step no release has.
		seedReconcileKeys(t, ctx, db, 3)
		total := countKeys(t, rawDb)
		corruptAllKeys(t, rawDb)

		// The whole sweep runs, ignoring the position's key.
		slice, err := db.ReconcileCarryForwardLabelsSlice(ctx, ReconcilePosition{Step: "retired", AfterKey: "zzz"}, 1000, 10, nil)
		require.NoError(t, err)
		require.True(t, slice.Done)
		require.GreaterOrEqual(t, slice.Corrected, int64(total))
	})

	t.Run("an error leaves no position to continue from", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := reconcileTestCtx()

		// With no refill and a deadline, Wait fails at once when the burst is spent.
		seedReconcileKeys(t, ctx, db, 5)
		runCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		limiter := rate.NewLimiter(0, 1)

		// The caller must retry from its own position, not a partial one.
		slice, err := db.ReconcileCarryForwardLabelsSlice(runCtx, ReconcilePosition{Step: "keys"}, 1000, 10, limiter)
		require.Error(t, err)
		require.False(t, slice.Done)
		require.Equal(t, ReconcilePosition{}, slice.Next)
	})
}

package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/stretchr/testify/require"
)

// checkpointTestNow has sub-microsecond digits so that timestamps that are not
// normalized for storage fail on every platform.
var checkpointTestNow = time.Date(2026, 10, 30, 12, 0, 0, 123456789, time.UTC)

// checkpointTestProgress is a task-specific payload for checkpointer tests.
type checkpointTestProgress struct {
	Cursor string `json:"cursor"`
	Count  int    `json:"count"`
}

// checkpointTestCtx returns a context whose clock is offset from checkpointTestNow.
func checkpointTestCtx(offset time.Duration) context.Context {
	return apctx.WithFixedClock(context.Background(), checkpointTestNow.Add(offset))
}

func TestTaskCheckpointRevisions(t *testing.T) {
	key := TaskCheckpointKey{TaskType: "test:sweep"}
	first := json.RawMessage(`{"cursor":"a"}`)
	second := json.RawMessage(`{"cursor":"b"}`)

	t.Run("a missing checkpoint is not found", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)

		_, err := db.GetTaskCheckpoint(checkpointTestCtx(0), key)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("a created checkpoint reads back as stored", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := checkpointTestCtx(0)

		revision, err := db.SaveTaskCheckpoint(ctx, key, "", first, time.Hour)
		require.NoError(t, err)
		require.NotEmpty(t, revision)

		// Every field round-trips, with times normalized to stored precision.
		checkpoint, err := db.GetTaskCheckpoint(ctx, key)
		require.NoError(t, err)
		stored := checkpointTestNow.Truncate(time.Microsecond)
		require.Equal(t, key.TaskType, checkpoint.TaskType)
		require.Equal(t, key.Scope, checkpoint.Scope)
		require.Equal(t, revision, checkpoint.Revision)
		require.JSONEq(t, string(first), string(checkpoint.Payload))
		require.True(t, stored.Add(time.Hour).Equal(checkpoint.ExpiresAt))
		require.True(t, stored.Equal(checkpoint.CreatedAt))
		require.True(t, stored.Equal(checkpoint.UpdatedAt))
	})

	t.Run("the current revision replaces progress and extends the ttl", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)

		revision, err := db.SaveTaskCheckpoint(checkpointTestCtx(0), key, "", first, time.Hour)
		require.NoError(t, err)

		// A later save issues a new revision and moves expiry, keeping CreatedAt.
		later := checkpointTestCtx(30 * time.Minute)
		updated, err := db.SaveTaskCheckpoint(later, key, revision, second, time.Hour)
		require.NoError(t, err)
		require.NotEqual(t, revision, updated)

		checkpoint, err := db.GetTaskCheckpoint(later, key)
		require.NoError(t, err)
		require.Equal(t, updated, checkpoint.Revision)
		require.JSONEq(t, string(second), string(checkpoint.Payload))
		stored := checkpointTestNow.Truncate(time.Microsecond)
		require.True(t, stored.Add(90*time.Minute).Equal(checkpoint.ExpiresAt))
		require.True(t, stored.Equal(checkpoint.CreatedAt))
		require.True(t, stored.Add(30*time.Minute).Equal(checkpoint.UpdatedAt))
	})

	t.Run("a stale revision cannot overwrite newer progress", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := checkpointTestCtx(0)

		// Both runs read the first revision, and one of them saves.
		stale, err := db.SaveTaskCheckpoint(ctx, key, "", first, time.Hour)
		require.NoError(t, err)
		current, err := db.SaveTaskCheckpoint(ctx, key, stale, second, time.Hour)
		require.NoError(t, err)

		// The other run is refused and nothing changes.
		_, err = db.SaveTaskCheckpoint(ctx, key, stale, first, time.Hour)
		require.ErrorIs(t, err, ErrTaskCheckpointConflict)
		checkpoint, err := db.GetTaskCheckpoint(ctx, key)
		require.NoError(t, err)
		require.Equal(t, current, checkpoint.Revision)
		require.JSONEq(t, string(second), string(checkpoint.Payload))
	})

	t.Run("creating over live progress conflicts", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := checkpointTestCtx(0)

		_, err := db.SaveTaskCheckpoint(ctx, key, "", first, time.Hour)
		require.NoError(t, err)

		// A run that saw no checkpoint must not clobber one created since.
		_, err = db.SaveTaskCheckpoint(ctx, key, "", second, time.Hour)
		require.ErrorIs(t, err, ErrTaskCheckpointConflict)
	})

	t.Run("exactly one of several racing creators wins", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := checkpointTestCtx(0)

		// All writers start from the same state of having seen no checkpoint.
		const writers = 8
		results := make([]error, writers)
		var wg sync.WaitGroup
		for i := range writers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, results[i] = db.SaveTaskCheckpoint(ctx, key, "", first, time.Hour)
			}()
		}
		wg.Wait()

		// One write succeeds, and every other is told it lost.
		successes := 0
		for _, err := range results {
			if err == nil {
				successes++
				continue
			}
			require.ErrorIs(t, err, ErrTaskCheckpointConflict)
		}
		require.Equal(t, 1, successes)
	})

	t.Run("a revision from before a delete cannot touch the recreated checkpoint", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := checkpointTestCtx(0)

		// A finished run removes the checkpoint and the next run creates a new one.
		old, err := db.SaveTaskCheckpoint(ctx, key, "", first, time.Hour)
		require.NoError(t, err)
		require.NoError(t, db.DeleteTaskCheckpoint(ctx, key, old))
		_, err = db.SaveTaskCheckpoint(ctx, key, "", second, time.Hour)
		require.NoError(t, err)

		// A straggler from the earlier run matches neither by save nor by delete.
		_, err = db.SaveTaskCheckpoint(ctx, key, old, first, time.Hour)
		require.ErrorIs(t, err, ErrTaskCheckpointConflict)
		require.ErrorIs(t, db.DeleteTaskCheckpoint(ctx, key, old), ErrTaskCheckpointConflict)
	})

	t.Run("a stale delete leaves newer progress in place", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := checkpointTestCtx(0)

		stale, err := db.SaveTaskCheckpoint(ctx, key, "", first, time.Hour)
		require.NoError(t, err)
		current, err := db.SaveTaskCheckpoint(ctx, key, stale, second, time.Hour)
		require.NoError(t, err)

		// Only the holder of the current revision can remove the checkpoint.
		require.ErrorIs(t, db.DeleteTaskCheckpoint(ctx, key, stale), ErrTaskCheckpointConflict)
		_, err = db.GetTaskCheckpoint(ctx, key)
		require.NoError(t, err)
		require.NoError(t, db.DeleteTaskCheckpoint(ctx, key, current))
		_, err = db.GetTaskCheckpoint(ctx, key)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("task types and scopes keep separate checkpoints", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := checkpointTestCtx(0)

		// Each of these keys holds its own progress at the same time.
		keys := []TaskCheckpointKey{
			{TaskType: "test:one"},
			{TaskType: "test:one", Scope: "root.a"},
			{TaskType: "test:one", Scope: "root.b"},
			{TaskType: "test:two", Scope: "root.a"},
		}
		for i, k := range keys {
			_, err := db.SaveTaskCheckpoint(ctx, k, "", json.RawMessage(fmt.Sprintf(`{"count":%d}`, i)), time.Hour)
			require.NoError(t, err)
		}
		for i, k := range keys {
			checkpoint, err := db.GetTaskCheckpoint(ctx, k)
			require.NoError(t, err)
			require.JSONEq(t, fmt.Sprintf(`{"count":%d}`, i), string(checkpoint.Payload))
		}
	})

	t.Run("rejects writes that could not be used later", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := checkpointTestCtx(0)

		// Each bad write is refused before reaching the table.
		_, err := db.SaveTaskCheckpoint(ctx, TaskCheckpointKey{}, "", first, time.Hour)
		require.ErrorContains(t, err, "task type")
		_, err = db.SaveTaskCheckpoint(ctx, key, "", first, 0)
		require.ErrorContains(t, err, "ttl")
		_, err = db.SaveTaskCheckpoint(ctx, key, "", json.RawMessage(`{not json`), time.Hour)
		require.ErrorIs(t, err, ErrInvalidTaskCheckpointPayload)
		_, err = db.GetTaskCheckpoint(ctx, key)
		require.ErrorIs(t, err, ErrNotFound)
	})
}

func TestTaskCheckpointExpiry(t *testing.T) {
	key := TaskCheckpointKey{TaskType: "test:sweep"}
	payload := json.RawMessage(`{"cursor":"a"}`)

	t.Run("expires exactly at its ttl without waiting for cleanup", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)

		_, err := db.SaveTaskCheckpoint(checkpointTestCtx(0), key, "", payload, time.Hour)
		require.NoError(t, err)

		// It is usable just before the ttl passes and gone at that moment.
		_, err = db.GetTaskCheckpoint(checkpointTestCtx(time.Hour-time.Second), key)
		require.NoError(t, err)
		_, err = db.GetTaskCheckpoint(checkpointTestCtx(time.Hour), key)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("its holder cannot revive an expired checkpoint", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)

		revision, err := db.SaveTaskCheckpoint(checkpointTestCtx(0), key, "", payload, time.Hour)
		require.NoError(t, err)

		// The outcome must match what the holder would see after cleanup ran.
		_, err = db.SaveTaskCheckpoint(checkpointTestCtx(2*time.Hour), key, revision, payload, time.Hour)
		require.ErrorIs(t, err, ErrTaskCheckpointConflict)
	})

	t.Run("a new run replaces an expired checkpoint not yet cleaned up", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)

		old, err := db.SaveTaskCheckpoint(checkpointTestCtx(0), key, "", payload, time.Hour)
		require.NoError(t, err)

		// The new run saw nothing, so it creates over the expired row.
		later := checkpointTestCtx(2 * time.Hour)
		revision, err := db.SaveTaskCheckpoint(later, key, "", json.RawMessage(`{"cursor":"b"}`), time.Hour)
		require.NoError(t, err)
		require.NotEqual(t, old, revision)

		// The row is a fresh checkpoint, not a continuation of the expired one.
		checkpoint, err := db.GetTaskCheckpoint(later, key)
		require.NoError(t, err)
		require.JSONEq(t, `{"cursor":"b"}`, string(checkpoint.Payload))
		require.True(t, checkpointTestNow.Truncate(time.Microsecond).Add(2*time.Hour).Equal(checkpoint.CreatedAt))
	})

	t.Run("expiry compares instants across clock time zones", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		east := time.FixedZone("east", 10*60*60)

		// A writer ahead of UTC on the wall clock must not appear to live longer.
		ctx := apctx.WithFixedClock(context.Background(), checkpointTestNow.In(east))
		_, err := db.SaveTaskCheckpoint(ctx, key, "", payload, time.Hour)
		require.NoError(t, err)
		_, err = db.GetTaskCheckpoint(checkpointTestCtx(2*time.Hour), key)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("cleanup removes only expired checkpoints", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		defer rawDb.Close()
		ctx := checkpointTestCtx(0)

		// One checkpoint will have expired by the cleanup and one will not.
		expiring := TaskCheckpointKey{TaskType: "test:short"}
		lasting := TaskCheckpointKey{TaskType: "test:long"}
		_, err := db.SaveTaskCheckpoint(ctx, expiring, "", payload, time.Hour)
		require.NoError(t, err)
		_, err = db.SaveTaskCheckpoint(ctx, lasting, "", payload, 3*time.Hour)
		require.NoError(t, err)

		// Before anything expires there is nothing to do.
		deleted, err := db.DeleteExpiredTaskCheckpoints(ctx)
		require.NoError(t, err)
		require.Zero(t, deleted)

		// Later, only the expired row leaves the table.
		deleted, err = db.DeleteExpiredTaskCheckpoints(checkpointTestCtx(2 * time.Hour))
		require.NoError(t, err)
		require.Equal(t, int64(1), deleted)
		var remaining string
		require.NoError(t, rawDb.QueryRow(`SELECT task_type FROM task_checkpoints`).Scan(&remaining))
		require.Equal(t, lasting.TaskType, remaining)
	})
}

func TestTaskCheckpointer(t *testing.T) {
	key := TaskCheckpointKey{TaskType: "test:sweep", Scope: "root.a"}
	newCheckpointer := func(db DB) *TaskCheckpointer[checkpointTestProgress] {
		return NewTaskCheckpointer[checkpointTestProgress](db, key, time.Hour)
	}

	t.Run("carries typed progress from one run to the next until cleared", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := checkpointTestCtx(0)

		// The first run finds nothing and saves twice, tracking its own revision.
		first := newCheckpointer(db)
		progress, err := first.Load(ctx)
		require.NoError(t, err)
		require.Nil(t, progress)
		require.NoError(t, first.Save(ctx, checkpointTestProgress{Cursor: "a", Count: 1}))
		require.NoError(t, first.Save(ctx, checkpointTestProgress{Cursor: "b", Count: 2}))

		// The next run resumes from the latest save and finishes the work.
		second := newCheckpointer(db)
		progress, err = second.Load(ctx)
		require.NoError(t, err)
		require.Equal(t, checkpointTestProgress{Cursor: "b", Count: 2}, *progress)
		require.NoError(t, second.Clear(ctx))

		// A run after that starts from the beginning.
		progress, err = newCheckpointer(db).Load(ctx)
		require.NoError(t, err)
		require.Nil(t, progress)
	})

	t.Run("overlapping runs resolve to one owner", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := checkpointTestCtx(0)
		require.NoError(t, newCheckpointer(db).Save(ctx, checkpointTestProgress{Cursor: "a"}))

		// Both runs load the same state.
		winner, loser := newCheckpointer(db), newCheckpointer(db)
		_, err := winner.Load(ctx)
		require.NoError(t, err)
		_, err = loser.Load(ctx)
		require.NoError(t, err)

		// The first to save owns the work, so the other cannot save or clear.
		require.NoError(t, winner.Save(ctx, checkpointTestProgress{Cursor: "b"}))
		err = loser.Save(ctx, checkpointTestProgress{Cursor: "stale"})
		require.ErrorIs(t, err, ErrTaskCheckpointConflict)
		require.ErrorIs(t, loser.Clear(ctx), ErrTaskCheckpointConflict)

		// The owner carries on unaffected.
		require.NoError(t, winner.Save(ctx, checkpointTestProgress{Cursor: "c"}))
	})

	t.Run("clearing without seeing a checkpoint leaves other progress alone", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := checkpointTestCtx(0)

		// This run finished without saving, while another run saved progress.
		idle := newCheckpointer(db)
		_, err := idle.Load(ctx)
		require.NoError(t, err)
		require.NoError(t, newCheckpointer(db).Save(ctx, checkpointTestProgress{Cursor: "a"}))

		// Its clear changes nothing.
		require.NoError(t, idle.Clear(ctx))
		progress, err := newCheckpointer(db).Load(ctx)
		require.NoError(t, err)
		require.NotNil(t, progress)
	})

	t.Run("a run can start over from an unreadable payload and replace it", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := checkpointTestCtx(0)

		// Progress stored by an older release no longer decodes into the payload type.
		_, err := db.SaveTaskCheckpoint(ctx, key, "", json.RawMessage(`["old format"]`), time.Hour)
		require.NoError(t, err)

		// Loading reports the problem but keeps the revision, so saving succeeds.
		checkpointer := newCheckpointer(db)
		_, err = checkpointer.Load(ctx)
		require.ErrorIs(t, err, ErrInvalidTaskCheckpointPayload)
		require.NoError(t, checkpointer.Save(ctx, checkpointTestProgress{Cursor: "a"}))

		progress, err := newCheckpointer(db).Load(ctx)
		require.NoError(t, err)
		require.Equal(t, "a", progress.Cursor)
	})

	t.Run("passes through read failures", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)

		// A cancelled read must not look like "no checkpoint", which would restart the work.
		ctx, cancel := context.WithCancel(checkpointTestCtx(0))
		cancel()
		_, err := newCheckpointer(db).Load(ctx)
		require.Error(t, err)
		require.False(t, errors.Is(err, ErrNotFound))
	})
}

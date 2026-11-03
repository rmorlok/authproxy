package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apctx"
)

// TaskCheckpointsTable stores the progress of unfinished background tasks.
const TaskCheckpointsTable = "task_checkpoints"

// ErrTaskCheckpointConflict is returned when a checkpoint write does not hold the
// latest revision, which means another run has taken over the same work, or the
// checkpoint expired or was removed. The caller must stop rather than retry the
// write: its view of the work is out of date.
var ErrTaskCheckpointConflict = errors.New("task checkpoint conflict")

// ErrInvalidTaskCheckpointPayload is returned when a stored checkpoint cannot be
// decoded, typically because a release changed the task's payload type.
var ErrInvalidTaskCheckpointPayload = errors.New("invalid task checkpoint payload")

// TaskCheckpointKey identifies the work a checkpoint belongs to. Runs that should
// continue each other's progress must use the same key, so it is built from the
// task's type rather than asynq's task ID, which changes on every scheduled enqueue.
type TaskCheckpointKey struct {
	// TaskType is the asynq task type. It is required.
	TaskType string
	// Scope separates runs of a parameterized task, such as the namespace a
	// propagation task is working on. It is empty for a task with one logical run.
	Scope string
}

// TaskCheckpoint is the saved progress of a long-running background task, letting
// a retry, a restarted worker, or the next scheduled run continue rather than
// repeat finished work. It is operational state rather than a resource: rows are
// hard-deleted when the work finishes or expires, and losing one only costs
// repeated work, so payloads must never be the only copy of anything.
//
// Tasks should normally use TaskCheckpointer rather than this record.
type TaskCheckpoint struct {
	// TaskType and Scope together identify the checkpoint. See TaskCheckpointKey.
	TaskType string
	Scope    string
	// Revision is an opaque token replaced by every write. A random token is used
	// rather than a counter so a value is never reused, even after the row is
	// deleted and recreated: a stale writer can then never match a newer row.
	Revision string
	// Payload is task-defined JSON describing the progress.
	Payload json.RawMessage
	// ExpiresAt is when the checkpoint stops being usable. Each write moves it.
	ExpiresAt time.Time
	// CreatedAt is when this run of the work saved its first progress.
	CreatedAt time.Time
	// UpdatedAt is when progress was last saved, for operators checking on a task.
	UpdatedAt time.Time
}

func (c *TaskCheckpoint) cols() []string {
	return []string{"task_type", "scope", "revision", "payload", "expires_at", "created_at", "updated_at"}
}

func (c *TaskCheckpoint) fields() []any {
	return []any{&c.TaskType, &c.Scope, &c.Revision, (*[]byte)(&c.Payload), &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt}
}

func (c *TaskCheckpoint) values() []any {
	// The payload is bound as text because that is what both a SQLite text column
	// and a PostgreSQL jsonb column expect.
	return []any{c.TaskType, c.Scope, c.Revision, string(c.Payload), c.ExpiresAt, c.CreatedAt, c.UpdatedAt}
}

// taskCheckpointTime returns the current time in the form stored for checkpoints:
// UTC, because SQLite compares datetimes as text, and to the microsecond, because
// that is PostgreSQL's precision.
func taskCheckpointTime(ctx context.Context) time.Time {
	return apctx.GetClock(ctx).Now().UTC().Truncate(time.Microsecond)
}

// GetTaskCheckpoint returns the checkpoint for key, or ErrNotFound when there is
// none. An expired checkpoint counts as missing whether or not cleanup has removed
// it yet, so behavior does not depend on the cleanup task's timing.
func (s *service) GetTaskCheckpoint(ctx context.Context, key TaskCheckpointKey) (*TaskCheckpoint, error) {
	var checkpoint TaskCheckpoint
	err := s.sq.
		Select(checkpoint.cols()...).
		From(TaskCheckpointsTable).
		Where(sq.Eq{"task_type": key.TaskType, "scope": key.Scope}).
		Where(sq.Gt{"expires_at": taskCheckpointTime(ctx)}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(checkpoint.fields()...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &checkpoint, nil
}

// SaveTaskCheckpoint writes progress for key and returns the new revision, which
// the caller must present on its next write. expectedRevision is the revision the
// caller last read or wrote, or empty when it found no checkpoint. The write is a
// compare-and-swap: it returns ErrTaskCheckpointConflict, changing nothing, unless
// expectedRevision still matches a live checkpoint or, when empty, no live
// checkpoint exists. Of any set of runs that started from the same state, exactly
// one therefore succeeds, and a stale run cannot move progress backward or recreate
// a checkpoint that a finished run removed.
//
// ttl measures from now and must be positive, and it must comfortably exceed the
// time between saves: once a checkpoint expires, its holder gets a conflict.
func (s *service) SaveTaskCheckpoint(
	ctx context.Context,
	key TaskCheckpointKey,
	expectedRevision string,
	payload json.RawMessage,
	ttl time.Duration,
) (string, error) {
	if key.TaskType == "" {
		return "", errors.New("task checkpoint requires a task type")
	}
	if ttl <= 0 {
		return "", fmt.Errorf("task checkpoint ttl must be positive, got %s", ttl)
	}
	if !json.Valid(payload) {
		return "", fmt.Errorf("%w: payload is not valid JSON", ErrInvalidTaskCheckpointPayload)
	}

	now := taskCheckpointTime(ctx)
	checkpoint := TaskCheckpoint{
		TaskType:  key.TaskType,
		Scope:     key.Scope,
		Revision:  uuid.NewString(),
		Payload:   payload,
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
		UpdatedAt: now,
	}

	var (
		result sql.Result
		err    error
	)
	if expectedRevision == "" {
		// Only an expired row may be replaced, because nobody can still own it.
		result, err = s.sq.
			Insert(TaskCheckpointsTable).
			Columns(checkpoint.cols()...).
			Values(checkpoint.values()...).
			Suffix(`ON CONFLICT (task_type, scope) DO UPDATE SET
				revision = excluded.revision,
				payload = excluded.payload,
				expires_at = excluded.expires_at,
				created_at = excluded.created_at,
				updated_at = excluded.updated_at
				WHERE ` + TaskCheckpointsTable + `.expires_at <= excluded.updated_at`).
			RunWith(s.db).
			ExecContext(ctx)
	} else {
		result, err = s.sq.
			Update(TaskCheckpointsTable).
			Set("revision", checkpoint.Revision).
			Set("payload", string(checkpoint.Payload)).
			Set("expires_at", checkpoint.ExpiresAt).
			Set("updated_at", checkpoint.UpdatedAt).
			Where(sq.Eq{"task_type": key.TaskType, "scope": key.Scope, "revision": expectedRevision}).
			Where(sq.Gt{"expires_at": now}).
			RunWith(s.db).
			ExecContext(ctx)
	}
	if err != nil {
		return "", err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if affected == 0 {
		return "", ErrTaskCheckpointConflict
	}
	return checkpoint.Revision, nil
}

// DeleteTaskCheckpoint removes the checkpoint for key so the next run starts from
// the beginning. Like SaveTaskCheckpoint it is a compare-and-swap, so a stale run
// finishing cannot erase a newer run's progress: it returns
// ErrTaskCheckpointConflict, changing nothing, unless revision is still current.
func (s *service) DeleteTaskCheckpoint(ctx context.Context, key TaskCheckpointKey, revision string) error {
	result, err := s.sq.
		Delete(TaskCheckpointsTable).
		Where(sq.Eq{"task_type": key.TaskType, "scope": key.Scope, "revision": revision}).
		RunWith(s.db).
		ExecContext(ctx)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrTaskCheckpointConflict
	}
	return nil
}

// DeleteExpiredTaskCheckpoints hard-deletes every expired checkpoint and returns
// how many it removed. Expired rows are already ignored, so this only reclaims
// storage for work that never finished.
func (s *service) DeleteExpiredTaskCheckpoints(ctx context.Context) (int64, error) {
	result, err := s.sq.
		Delete(TaskCheckpointsTable).
		Where(sq.LtOrEq{"expires_at": taskCheckpointTime(ctx)}).
		RunWith(s.db).
		ExecContext(ctx)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// TaskCheckpointer is the standard way for a task to save and resume progress. It
// holds one run's view of a checkpoint: it converts the task's payload type T to
// and from JSON, applies the task's TTL, and carries the revision between calls so
// that writes are safe against overlapping runs.
//
// A run creates its own checkpointer, calls Load once, calls Save as it makes
// progress, and calls Clear when the work is finished. Save and Clear return
// ErrTaskCheckpointConflict when another run has taken over; the run must then stop
// without treating that as a failure, because retrying would only compete with the
// run that now owns the work. Writes are not deduplicated: a save whose commit was
// lost surfaces as an error and the task resumes from older progress, so tasks must
// tolerate repeating the work since their last save.
//
// A TaskCheckpointer belongs to one run and is not safe for concurrent use.
type TaskCheckpointer[T any] struct {
	db  DB
	key TaskCheckpointKey
	ttl time.Duration
	// revision is the last revision this run read or wrote, or empty when no live
	// checkpoint was seen.
	revision string
}

// NewTaskCheckpointer returns a checkpointer for one run of the work named by key.
// ttl is applied on every save, so it limits how long abandoned progress stays
// usable rather than how long the work may take. It should exceed the longest
// expected gap between saves, including the gap between scheduled runs.
func NewTaskCheckpointer[T any](db DB, key TaskCheckpointKey, ttl time.Duration) *TaskCheckpointer[T] {
	return &TaskCheckpointer[T]{db: db, key: key, ttl: ttl}
}

// Load returns the saved progress, or nil when the work should start from the
// beginning. When the stored payload cannot be decoded it returns an error wrapping
// ErrInvalidTaskCheckpointPayload, and the run may start over: it still holds the
// revision, so its first Save replaces the unreadable checkpoint.
func (c *TaskCheckpointer[T]) Load(ctx context.Context) (*T, error) {
	c.revision = ""
	checkpoint, err := c.db.GetTaskCheckpoint(ctx, c.key)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	c.revision = checkpoint.Revision
	var payload T
	if err := json.Unmarshal(checkpoint.Payload, &payload); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTaskCheckpointPayload, err)
	}
	return &payload, nil
}

// Save replaces the saved progress. It returns ErrTaskCheckpointConflict when
// another run has taken over or the checkpoint has expired.
func (c *TaskCheckpointer[T]) Save(ctx context.Context, payload T) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode task checkpoint: %w", err)
	}
	revision, err := c.db.SaveTaskCheckpoint(ctx, c.key, c.revision, encoded, c.ttl)
	if err != nil {
		return err
	}
	c.revision = revision
	return nil
}

// Clear removes the saved progress once the work is finished. A run that never
// saw a checkpoint has nothing to remove and leaves any other run's progress alone.
// It returns ErrTaskCheckpointConflict when another run has taken over.
func (c *TaskCheckpointer[T]) Clear(ctx context.Context) error {
	if c.revision == "" {
		return nil
	}
	if err := c.db.DeleteTaskCheckpoint(ctx, c.key, c.revision); err != nil {
		return err
	}
	c.revision = ""
	return nil
}

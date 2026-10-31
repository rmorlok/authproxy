package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
)

// TaskCheckpointsTable stores the progress of unfinished background sweeps.
const TaskCheckpointsTable = "task_checkpoints"

// taskCheckpoint records how far a long-running sweep has progressed, so that a
// retry, a restarted worker, or the next scheduled run continues from there
// rather than repeating finished work. It is operational state rather than a
// resource: it is hard-deleted when the sweep finishes, so a row exists only
// while a sweep is unfinished, and losing one only costs repeated work.
type taskCheckpoint struct {
	// Name identifies the sweep.
	Name string
	// Step is the sweep-defined phase to continue.
	Step string
	// LastKey is the last key finished within Step, or empty when no key in the
	// step has been finished yet.
	LastKey string
	// UpdatedAt is when progress was last saved, for operators checking on a sweep.
	UpdatedAt time.Time
}

func (c *taskCheckpoint) cols() []string {
	return []string{"name", "step", "last_key", "updated_at"}
}

func (c *taskCheckpoint) fields() []any {
	return []any{&c.Name, &c.Step, &c.LastKey, &c.UpdatedAt}
}

func (c *taskCheckpoint) values() []any {
	return []any{c.Name, c.Step, c.LastKey, c.UpdatedAt}
}

// getTaskCheckpoint returns the saved progress of the named sweep, or nil when
// the sweep has no unfinished run.
func (s *service) getTaskCheckpoint(ctx context.Context, name string) (*taskCheckpoint, error) {
	var checkpoint taskCheckpoint
	err := s.sq.
		Select(checkpoint.cols()...).
		From(TaskCheckpointsTable).
		Where(sq.Eq{"name": name}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(checkpoint.fields()...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &checkpoint, nil
}

// saveTaskCheckpoint creates or replaces the saved progress of the named sweep.
func (s *service) saveTaskCheckpoint(ctx context.Context, name, step, lastKey string) error {
	checkpoint := taskCheckpoint{
		Name:      name,
		Step:      step,
		LastKey:   lastKey,
		UpdatedAt: apctx.GetClock(ctx).Now(),
	}
	_, err := s.sq.
		Insert(TaskCheckpointsTable).
		Columns(checkpoint.cols()...).
		Values(checkpoint.values()...).
		Suffix(`ON CONFLICT (name) DO UPDATE SET
			step = excluded.step,
			last_key = excluded.last_key,
			updated_at = excluded.updated_at`).
		RunWith(s.db).
		ExecContext(ctx)
	return err
}

// deleteTaskCheckpoint removes the saved progress of the named sweep so its next
// run starts from the beginning. Deleting a missing checkpoint is a no-op.
func (s *service) deleteTaskCheckpoint(ctx context.Context, name string) error {
	_, err := s.sq.
		Delete(TaskCheckpointsTable).
		Where(sq.Eq{"name": name}).
		RunWith(s.db).
		ExecContext(ctx)
	return err
}

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
)

const ConnectionProbeOutcomesTable = "connection_probe_outcomes"

// Outcome enum stored in connection_probe_outcomes.outcome.
const (
	ProbeOutcomeStatusSuccess = "success"
	ProbeOutcomeStatusFailure = "failure"
)

// ConnectionProbeOutcome is one append-only event in the probe-outcome log.
// Each probe invocation produces a row; the runtime walks the most recent rows
// for a (connection_id, probe_id) pair to compute consecutive-success or
// -failure counts that drive the connection's health_state transitions.
//
// Storage is event-shaped rather than counter-shaped so that:
//   - writes never race on a shared counter row;
//   - the log naturally carries probe history for operators (when did the
//     last failure happen? what error did it report?);
//   - threshold semantics can change without a schema migration.
//
// A daily cleanup task (DeleteOldProbeOutcomes) caps growth — see
// internal/core/task_probe_outcome_cleanup.go.
type ConnectionProbeOutcome struct {
	Id           apid.ID
	ConnectionId apid.ID
	ProbeId      string
	Outcome      string
	ErrorMessage *string
	OccurredAt   time.Time
	CreatedAt    time.Time
}

// InsertProbeOutcome appends an outcome event for the (connection, probe).
// errorMessage is recorded only for failures; pass an empty string for success
// and it will be stored as NULL.
func (s *service) InsertProbeOutcome(
	ctx context.Context,
	connectionId apid.ID,
	probeId string,
	outcome string,
	errorMessage string,
) (*ConnectionProbeOutcome, error) {
	if connectionId == apid.Nil {
		return nil, errors.New("connection id is required")
	}
	if probeId == "" {
		return nil, errors.New("probe id is required")
	}
	if outcome != ProbeOutcomeStatusSuccess && outcome != ProbeOutcomeStatusFailure {
		return nil, fmt.Errorf("invalid probe outcome %q", outcome)
	}

	now := apctx.GetClock(ctx).Now()
	row := &ConnectionProbeOutcome{
		Id:           apctx.GetIdGenerator(ctx).New(apid.PrefixProbeOutcome),
		ConnectionId: connectionId,
		ProbeId:      probeId,
		Outcome:      outcome,
		OccurredAt:   now,
		CreatedAt:    now,
	}
	if outcome == ProbeOutcomeStatusFailure && errorMessage != "" {
		row.ErrorMessage = &errorMessage
	}

	_, err := s.sq.
		Insert(ConnectionProbeOutcomesTable).
		Columns("id", "connection_id", "probe_id", "outcome", "error_message", "occurred_at", "created_at").
		Values(row.Id, row.ConnectionId, row.ProbeId, row.Outcome, row.ErrorMessage, row.OccurredAt, row.CreatedAt).
		RunWith(s.db).
		Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to insert probe outcome: %w", err)
	}
	return row, nil
}

// GetRecentProbeOutcomes returns up to limit rows for the (connection, probe)
// pair, most recent first. The runtime uses this to compute consecutive
// matching outcomes by walking from the head until the outcome flips.
//
// Returns an empty slice when no outcome has ever been recorded.
func (s *service) GetRecentProbeOutcomes(
	ctx context.Context,
	connectionId apid.ID,
	probeId string,
	limit int,
) ([]*ConnectionProbeOutcome, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows, err := s.sq.
		Select("id", "connection_id", "probe_id", "outcome", "error_message", "occurred_at", "created_at").
		From(ConnectionProbeOutcomesTable).
		Where(sq.Eq{"connection_id": connectionId, "probe_id": probeId}).
		OrderBy("occurred_at DESC").
		Limit(uint64(limit)).
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*ConnectionProbeOutcome
	for rows.Next() {
		var r ConnectionProbeOutcome
		if err := rows.Scan(&r.Id, &r.ConnectionId, &r.ProbeId, &r.Outcome, &r.ErrorMessage, &r.OccurredAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// DeleteOldProbeOutcomes removes rows for the (connection, probe) older than
// olderThan, BUT always keeps at least keepMinimum most-recent rows so the
// transition runtime always has enough history to compute its thresholds.
//
// Returns the number of rows deleted (>= 0).
func (s *service) DeleteOldProbeOutcomes(
	ctx context.Context,
	connectionId apid.ID,
	probeId string,
	keepMinimum int,
	olderThan time.Time,
) (int64, error) {
	if keepMinimum < 0 {
		keepMinimum = 0
	}

	// Find the ids of the most recent keepMinimum rows; exclude them from the
	// delete. Works in both SQLite and Postgres.
	keepIds, err := s.sq.
		Select("id").
		From(ConnectionProbeOutcomesTable).
		Where(sq.Eq{"connection_id": connectionId, "probe_id": probeId}).
		OrderBy("occurred_at DESC").
		Limit(uint64(keepMinimum)).
		RunWith(s.db).
		Query()
	if err != nil {
		return 0, err
	}

	var protected []apid.ID
	for keepIds.Next() {
		var id apid.ID
		if err := keepIds.Scan(&id); err != nil {
			_ = keepIds.Close()
			return 0, err
		}
		protected = append(protected, id)
	}
	_ = keepIds.Close()
	if err := keepIds.Err(); err != nil {
		return 0, err
	}

	q := s.sq.
		Delete(ConnectionProbeOutcomesTable).
		Where(sq.Eq{"connection_id": connectionId, "probe_id": probeId}).
		Where(sq.Lt{"occurred_at": olderThan})

	if len(protected) > 0 {
		q = q.Where(sq.NotEq{"id": protected})
	}

	res, err := q.RunWith(s.db).Exec()
	if err != nil {
		return 0, fmt.Errorf("failed to delete old probe outcomes: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// DistinctProbeIdsForConnection returns the probe ids that have ever recorded
// an outcome for the connection. Used by the cleanup task to enumerate which
// (connection, probe) pairs need pruning.
func (s *service) DistinctProbeIdsForConnection(ctx context.Context, connectionId apid.ID) ([]string, error) {
	rows, err := s.sq.
		Select("DISTINCT probe_id").
		From(ConnectionProbeOutcomesTable).
		Where(sq.Eq{"connection_id": connectionId}).
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CountProbeOutcomes returns the total row count for the (connection, probe)
// pair. Useful for tests that need to assert deletion semantics directly.
func (s *service) CountProbeOutcomes(ctx context.Context, connectionId apid.ID, probeId string) (int, error) {
	var n int
	err := s.sq.
		Select("COUNT(*)").
		From(ConnectionProbeOutcomesTable).
		Where(sq.Eq{"connection_id": connectionId, "probe_id": probeId}).
		RunWith(s.db).
		QueryRow().
		Scan(&n)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

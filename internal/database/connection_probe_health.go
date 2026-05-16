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


const ConnectionProbeHealthTable = "connection_probe_health"

// Probe-outcome enum stored in connection_probe_health.last_outcome.
const (
	ProbeOutcomeStatusSuccess = "success"
	ProbeOutcomeStatusFailure = "failure"
)

// ConnectionProbeHealth is the per-(connection, probe) row that drives the
// probe-driven health-check signal. Each probe outcome (success or failure)
// adjusts the consecutive counters atomically; the runtime then compares to
// the probe's configured thresholds to decide whether to flip the connection's
// health_state.
//
// One row per (connection_id, probe_id). Rows are created lazily on first
// outcome via UPSERT — the connector definition doesn't need to pre-populate
// them.
type ConnectionProbeHealth struct {
	ConnectionId         apid.ID
	ProbeId              string
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LastOutcome          *string
	LastOutcomeAt        *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// RecordProbeSuccess increments the consecutive-successes counter and resets
// the consecutive-failures counter for the (connection, probe) pair. Returns
// the post-update row so callers can immediately compare to thresholds without
// a second read.
//
// First-time call inserts a row with successes=1, failures=0.
func (s *service) RecordProbeSuccess(ctx context.Context, connectionId apid.ID, probeId string) (*ConnectionProbeHealth, error) {
	return s.recordProbeOutcome(ctx, connectionId, probeId, ProbeOutcomeStatusSuccess)
}

// RecordProbeFailure increments the consecutive-failures counter and resets
// the consecutive-successes counter for the (connection, probe) pair. Returns
// the post-update row so callers can immediately compare to thresholds without
// a second read.
//
// First-time call inserts a row with failures=1, successes=0.
func (s *service) RecordProbeFailure(ctx context.Context, connectionId apid.ID, probeId string) (*ConnectionProbeHealth, error) {
	return s.recordProbeOutcome(ctx, connectionId, probeId, ProbeOutcomeStatusFailure)
}

func (s *service) recordProbeOutcome(ctx context.Context, connectionId apid.ID, probeId string, outcome string) (*ConnectionProbeHealth, error) {
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

	// Initial counter values for the insert path (first time this probe runs
	// on this connection). On conflict, the UPDATE clause increments one
	// counter and resets the other.
	var initialFailures, initialSuccesses int
	var updateFailuresExpr, updateSuccessesExpr string
	if outcome == ProbeOutcomeStatusSuccess {
		initialSuccesses = 1
		updateFailuresExpr = "0" // reset failure streak
		updateSuccessesExpr = ConnectionProbeHealthTable + ".consecutive_successes + 1"
	} else {
		initialFailures = 1
		updateFailuresExpr = ConnectionProbeHealthTable + ".consecutive_failures + 1"
		updateSuccessesExpr = "0" // reset success streak
	}

	// INSERT ... ON CONFLICT (connection_id, probe_id) DO UPDATE — supported
	// by both SQLite (≥ 3.24) and Postgres. Single statement keeps Postgres
	// out of the "current transaction aborted" state that a separate
	// INSERT + UPDATE recovery would trigger on PK collision.
	onConflict := fmt.Sprintf(
		"ON CONFLICT (connection_id, probe_id) DO UPDATE SET "+
			"consecutive_failures = %s, "+
			"consecutive_successes = %s, "+
			"last_outcome = excluded.last_outcome, "+
			"last_outcome_at = excluded.last_outcome_at, "+
			"updated_at = excluded.updated_at",
		updateFailuresExpr, updateSuccessesExpr,
	)

	_, err := s.sq.
		Insert(ConnectionProbeHealthTable).
		Columns("connection_id", "probe_id", "consecutive_failures", "consecutive_successes", "last_outcome", "last_outcome_at", "created_at", "updated_at").
		Values(connectionId, probeId, initialFailures, initialSuccesses, outcome, now, now, now).
		Suffix(onConflict).
		RunWith(s.db).
		Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to record probe outcome: %w", err)
	}

	return s.GetConnectionProbeHealth(ctx, connectionId, probeId)
}

// GetConnectionProbeHealth returns the counter row for the (connection, probe)
// pair, or ErrNotFound when no outcome has ever been recorded.
func (s *service) GetConnectionProbeHealth(ctx context.Context, connectionId apid.ID, probeId string) (*ConnectionProbeHealth, error) {
	var row ConnectionProbeHealth
	err := s.sq.
		Select(
			"connection_id", "probe_id",
			"consecutive_failures", "consecutive_successes",
			"last_outcome", "last_outcome_at",
			"created_at", "updated_at",
		).
		From(ConnectionProbeHealthTable).
		Where(sq.Eq{"connection_id": connectionId, "probe_id": probeId}).
		RunWith(s.db).
		QueryRow().
		Scan(
			&row.ConnectionId, &row.ProbeId,
			&row.ConsecutiveFailures, &row.ConsecutiveSuccesses,
			&row.LastOutcome, &row.LastOutcomeAt,
			&row.CreatedAt, &row.UpdatedAt,
		)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("probe health not found for (%s, %s): %w", connectionId, probeId, ErrNotFound)
		}
		return nil, err
	}
	return &row, nil
}

// ListConnectionProbeHealth returns all probe-counter rows for a connection,
// keyed by probe id. The map is empty when no probe has run yet.
func (s *service) ListConnectionProbeHealth(ctx context.Context, connectionId apid.ID) (map[string]*ConnectionProbeHealth, error) {
	rows, err := s.sq.
		Select(
			"connection_id", "probe_id",
			"consecutive_failures", "consecutive_successes",
			"last_outcome", "last_outcome_at",
			"created_at", "updated_at",
		).
		From(ConnectionProbeHealthTable).
		Where(sq.Eq{"connection_id": connectionId}).
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*ConnectionProbeHealth)
	for rows.Next() {
		var row ConnectionProbeHealth
		if err := rows.Scan(
			&row.ConnectionId, &row.ProbeId,
			&row.ConsecutiveFailures, &row.ConsecutiveSuccesses,
			&row.LastOutcome, &row.LastOutcomeAt,
			&row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result[row.ProbeId] = &row
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ResetConnectionProbeHealth zeroes every counter row for the connection.
// Called when the connection transitions back to healthy so a subsequent
// failure starts fresh from zero.
func (s *service) ResetConnectionProbeHealth(ctx context.Context, connectionId apid.ID) error {
	now := apctx.GetClock(ctx).Now()
	_, err := s.sq.
		Update(ConnectionProbeHealthTable).
		SetMap(map[string]interface{}{
			"consecutive_failures":  0,
			"consecutive_successes": 0,
			"updated_at":            now,
		}).
		Where(sq.Eq{"connection_id": connectionId}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to reset probe health counters: %w", err)
	}
	return nil
}

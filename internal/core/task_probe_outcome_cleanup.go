package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const taskTypeProbeOutcomeCleanup = "core:probe_outcome_cleanup"

// DefaultProbeOutcomeRetention is the default age beyond which probe-outcome
// rows are pruned by the cleanup task, subject to always keeping at least
// max(failure_threshold, recovery_threshold) most-recent rows per
// (connection, probe). Matches the daily cleanup cadence — the table will hold
// roughly retention + cleanup-interval worth of rows in steady state.
const DefaultProbeOutcomeRetention = 24 * time.Hour

// probeOutcomeCleanupTaskPayload carries the run-time configuration for one
// cleanup invocation. Kept tiny so individual runs can be re-tuned without
// changing schedule wiring (e.g. an operator could enqueue an ad-hoc cleanup
// with a stricter retention).
type probeOutcomeCleanupTaskPayload struct {
	RetentionSeconds int64 `json:"retention_seconds,omitempty"`
}

func newProbeOutcomeCleanupTask(retention time.Duration) (*asynq.Task, error) {
	payload, err := json.Marshal(probeOutcomeCleanupTaskPayload{RetentionSeconds: int64(retention.Seconds())})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskTypeProbeOutcomeCleanup, payload), nil
}

// runProbeOutcomeCleanup walks every non-deleted connection and prunes its
// probe outcome events. For each (connection, probe):
//
//   - Reads the probe's effective failure_threshold and recovery_threshold
//     from the connector definition; keeps at least max(those) most-recent
//     rows so the runtime always has enough history to compute transitions.
//   - Deletes rows older than the retention window beyond that minimum.
//
// Probe ids that have outcomes but no longer appear in the connector
// definition (e.g. probe was removed from YAML) still get pruned using
// default thresholds — keeps stale outcomes from accumulating forever.
//
// Errors on individual connections are logged and skipped — one bad connection
// shouldn't poison the whole sweep.
func (s *service) runProbeOutcomeCleanup(ctx context.Context, t *asynq.Task) error {
	logger := aplog.NewBuilder(s.logger).
		WithTask(t).
		WithCtx(ctx).
		Build()

	retention := DefaultProbeOutcomeRetention
	if len(t.Payload()) > 0 {
		var p probeOutcomeCleanupTaskPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("%s json.Unmarshal failed: %v: %w", taskTypeProbeOutcomeCleanup, err, asynq.SkipRetry)
		}
		if p.RetentionSeconds > 0 {
			retention = time.Duration(p.RetentionSeconds) * time.Second
		}
	}

	cutoff := apctx.GetClock(ctx).Now().Add(-retention)
	logger.Info("probe outcome cleanup starting", "retention", retention, "cutoff", cutoff)

	totalDeleted := int64(0)
	totalConns := 0
	err := s.db.ListConnectionsBuilder().
		WithDeletedHandling(database.DeletedHandlingExclude).
		Enumerate(ctx, func(pr pagination.PageResult[database.Connection]) (pagination.KeepGoing, error) {
			for _, dbConn := range pr.Results {
				totalConns++
				deleted, err := s.cleanupProbeOutcomesForConnection(ctx, dbConn.Id, cutoff)
				if err != nil {
					logger.Error("failed to cleanup probe outcomes for connection",
						"connection_id", dbConn.Id, "error", err)
					continue
				}
				totalDeleted += deleted
			}
			return pagination.Continue, nil
		})
	if err != nil {
		return fmt.Errorf("enumerate connections: %w", err)
	}

	logger.Info("probe outcome cleanup complete",
		"connections_examined", totalConns,
		"rows_deleted", totalDeleted,
	)
	return nil
}

// cleanupProbeOutcomesForConnection prunes outcome rows for one connection.
// Returns the count deleted.
func (s *service) cleanupProbeOutcomesForConnection(ctx context.Context, connectionId apid.ID, cutoff time.Time) (int64, error) {
	// Look up the connector definition once so we can read each probe's
	// thresholds. A connection without a resolvable definition still gets
	// outcomes pruned using default thresholds — stale outcomes for a
	// missing definition would otherwise grow unbounded.
	probesByID, err := s.probeConfigsForConnection(ctx, connectionId)
	if err != nil {
		// Don't bail — fall through to default thresholds.
		probesByID = nil
	}

	probeIds, err := s.db.DistinctProbeIdsForConnection(ctx, connectionId)
	if err != nil {
		return 0, fmt.Errorf("distinct probe ids: %w", err)
	}

	var totalDeleted int64
	for _, probeId := range probeIds {
		keepMin := defaultKeepMinimum
		if cfg, ok := probesByID[probeId]; ok {
			keepMin = probeKeepMinimum(cfg)
		}
		n, err := s.db.DeleteOldProbeOutcomes(ctx, connectionId, probeId, keepMin, cutoff)
		if err != nil {
			return totalDeleted, fmt.Errorf("delete old outcomes for probe %q: %w", probeId, err)
		}
		totalDeleted += n
	}
	return totalDeleted, nil
}

// defaultKeepMinimum is used for outcomes belonging to a probe that's no
// longer in the connector definition (or for which the definition couldn't be
// loaded). max(default failure, default recovery) so the runtime would still
// have enough history if the probe came back.
var defaultKeepMinimum = func() int {
	a, b := cschema.DefaultProbeFailureThreshold, cschema.DefaultProbeRecoveryThreshold
	if a > b {
		return a
	}
	return b
}()

// probeKeepMinimum returns the number of recent outcomes that must be kept for
// transition correctness: enough history to satisfy whichever threshold is
// larger.
func probeKeepMinimum(p *cschema.Probe) int {
	a := p.EffectiveFailureThreshold()
	b := p.EffectiveRecoveryThreshold()
	if a > b {
		return a
	}
	return b
}

// probeConfigsForConnection returns the connection's connector-defined probes
// keyed by id, for threshold lookup during cleanup. Returns nil + error if the
// connection or definition can't be resolved; caller falls back to defaults.
func (s *service) probeConfigsForConnection(ctx context.Context, connectionId apid.ID) (map[string]*cschema.Probe, error) {
	conn, err := s.getConnection(ctx, connectionId)
	if err != nil {
		return nil, err
	}
	def := conn.cv.GetDefinition()
	if def == nil {
		return nil, fmt.Errorf("nil connector definition")
	}
	out := make(map[string]*cschema.Probe, len(def.Probes))
	for i := range def.Probes {
		p := &def.Probes[i]
		out[p.Id] = p
	}
	return out, nil
}

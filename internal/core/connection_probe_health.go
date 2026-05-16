package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

// healthReasonPrefix prefixes the reason field on the structured health
// transition event so dashboards can filter probe-driven transitions distinct
// from refresh-driven ones (which use refresh_<category>).
const healthReasonPrefix = "probe:"

// recordPeriodicProbeOutcome is the probe-driven half of the health-state
// signal. It records the per-(connection, probe) counter, then decides whether
// the outcome crossed a threshold:
//
//   - On failure: if the just-incremented failure count reaches the probe's
//     failure threshold, flip the connection unhealthy. MarkHealthState is
//     idempotent so subsequent failures are no-ops on the state machine.
//
//   - On success: if the connection is currently unhealthy, check whether
//     recovery is satisfied. Recovery requires the current probe's success
//     streak to reach its recovery threshold AND no other probe to be at or
//     over its failure threshold. When satisfied, flip healthy and reset all
//     counters for the connection so a future failure starts a fresh streak.
//
// Also stamps last_validated_at on the active api-key credential when the
// outcome is a success against an api-key connector — gives operators a
// per-connection "credential last checked OK" timestamp without scraping logs.
//
// Errors are logged at call sites but do not cause the probe task to retry —
// the probe outcome is already authoritative; a counter-write failure should
// not invalidate the invocation.
func (c *connection) recordPeriodicProbeOutcome(ctx context.Context, probe iface.Probe, success bool) error {
	var row *database.ConnectionProbeHealth
	var err error
	if success {
		row, err = c.s.db.RecordProbeSuccess(ctx, c.Id, probe.GetId())
	} else {
		row, err = c.s.db.RecordProbeFailure(ctx, c.Id, probe.GetId())
	}
	if err != nil {
		return fmt.Errorf("record probe outcome: %w", err)
	}

	if success {
		if err := c.maybeUpdateApiKeyLastValidated(ctx); err != nil {
			// Non-fatal: log and continue with the health-state decision.
			c.logger.LogAttrs(ctx, slog.LevelWarn, "failed to update api-key last_validated_at",
				slog.String("probe_id", probe.GetId()),
				slog.String("error", err.Error()),
			)
		}
		return c.maybeRecoverHealth(ctx, probe, row)
	}

	if row.ConsecutiveFailures >= probe.EffectiveFailureThreshold() {
		reason := healthReasonPrefix + probe.GetId()
		return c.MarkHealthState(ctx, database.ConnectionHealthStateUnhealthy, reason)
	}
	return nil
}

// maybeRecoverHealth handles the success-side of the probe-driven transition.
// Called only on success; assumes the connection's current probe counters have
// already been updated (this probe's success streak incremented, failure streak
// reset to zero).
func (c *connection) maybeRecoverHealth(ctx context.Context, probe iface.Probe, row *database.ConnectionProbeHealth) error {
	if c.GetHealthState() == database.ConnectionHealthStateHealthy {
		return nil
	}

	if row.ConsecutiveSuccesses < probe.EffectiveRecoveryThreshold() {
		return nil
	}

	// Any other probe still at or over its failure threshold blocks recovery —
	// recovery means ALL probes are within bounds, not just the one that just
	// passed.
	all, err := c.s.db.ListConnectionProbeHealth(ctx, c.Id)
	if err != nil {
		return fmt.Errorf("list probe health: %w", err)
	}
	def := c.cv.GetDefinition()
	if def != nil {
		for _, p := range def.Probes {
			if p.Id == probe.GetId() {
				continue
			}
			cnt, ok := all[p.Id]
			if !ok {
				continue
			}
			if cnt.ConsecutiveFailures >= p.EffectiveFailureThreshold() {
				return nil // another probe is still failing
			}
		}
	}

	reason := healthReasonPrefix + probe.GetId()
	if err := c.MarkHealthState(ctx, database.ConnectionHealthStateHealthy, reason); err != nil {
		return err
	}
	// Reset every probe's counters so future failures start a fresh streak.
	// Without this, a probe that was at (threshold - 1) failures stays primed
	// and would flip unhealthy on its next failure rather than requiring the
	// full threshold count.
	if err := c.s.db.ResetConnectionProbeHealth(ctx, c.Id); err != nil {
		return fmt.Errorf("reset probe counters: %w", err)
	}
	return nil
}

// maybeUpdateApiKeyLastValidated stamps the active api-key credential's
// last_validated_at on probe success. No-op for non-api-key auth types and
// when no active credential exists.
func (c *connection) maybeUpdateApiKeyLastValidated(ctx context.Context) error {
	def := c.cv.GetDefinition()
	if def == nil || def.Auth == nil {
		return nil
	}
	if _, ok := def.Auth.Inner().(*config.AuthApiKey); !ok {
		return nil
	}

	cred, err := c.s.db.GetActiveApiKeyCredential(ctx, c.Id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			// No active credential — probe succeeded without one (raw http
			// probe against a public endpoint, say). Nothing to stamp.
			return nil
		}
		return err
	}
	return c.s.db.UpdateApiKeyCredentialLastValidated(ctx, cred.Id, apctx.GetClock(ctx).Now())
}

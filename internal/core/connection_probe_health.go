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
// signal. It appends an outcome event for the (connection, probe), then walks
// the recent event log to decide whether thresholds were crossed:
//
//   - On failure: if the most-recent rows show a consecutive-failure streak
//     >= the probe's failure_threshold, flip the connection unhealthy.
//     MarkHealthState is idempotent so further failures are no-ops on the
//     state machine.
//
//   - On success: if currently unhealthy, check whether recovery is satisfied.
//     Recovery requires (1) this probe's consecutive-success streak >= its
//     recovery_threshold AND (2) no other probe currently has a consecutive-
//     failure streak >= its own failure_threshold. When satisfied, flip
//     healthy.
//
// Also stamps last_validated_at on the active api-key credential when the
// outcome is a success against an api-key connector.
//
// Errors are returned for the caller (task_probe.go) to log. The probe
// invocation outcome itself is already authoritative; a bookkeeping failure
// should not trigger an Asynq retry.
func (c *connection) recordPeriodicProbeOutcome(ctx context.Context, probe iface.Probe, success bool, invokeErr error) error {
	outcome := database.ProbeOutcomeStatusSuccess
	errorMessage := ""
	if !success {
		outcome = database.ProbeOutcomeStatusFailure
		if invokeErr != nil {
			errorMessage = invokeErr.Error()
		}
	}

	if _, err := c.s.db.InsertProbeOutcome(ctx, c.Id, probe.GetId(), outcome, errorMessage); err != nil {
		return fmt.Errorf("insert probe outcome: %w", err)
	}

	if success {
		if err := c.maybeUpdateApiKeyLastValidated(ctx); err != nil {
			c.logger.LogAttrs(ctx, slog.LevelWarn, "failed to update api-key last_validated_at",
				slog.String("probe_id", probe.GetId()),
				slog.String("error", err.Error()),
			)
		}
		return c.maybeRecoverHealth(ctx, probe)
	}

	streak, err := c.consecutiveOutcomeStreak(ctx, probe.GetId(), database.ProbeOutcomeStatusFailure, probe.EffectiveFailureThreshold())
	if err != nil {
		return err
	}
	if streak >= probe.EffectiveFailureThreshold() {
		return c.MarkHealthState(ctx, database.ConnectionHealthStateUnhealthy, healthReasonPrefix+probe.GetId())
	}
	return nil
}

// maybeRecoverHealth handles the success-side of the probe-driven transition.
// Called only on success; assumes the success outcome has already been
// appended to the event log.
func (c *connection) maybeRecoverHealth(ctx context.Context, probe iface.Probe) error {
	if c.GetHealthState() == database.ConnectionHealthStateHealthy {
		return nil
	}

	successStreak, err := c.consecutiveOutcomeStreak(ctx, probe.GetId(), database.ProbeOutcomeStatusSuccess, probe.EffectiveRecoveryThreshold())
	if err != nil {
		return err
	}
	if successStreak < probe.EffectiveRecoveryThreshold() {
		return nil
	}

	// Recovery requires every OTHER enabled probe to be within its failure
	// threshold. A recovery on probe A doesn't restore health while enabled
	// probe B is still over-failing, but disabled peers are ignored.
	enabledProbes, err := c.GetEnabledProbes(ctx)
	if err != nil {
		return err
	}
	for _, otherProbe := range enabledProbes {
		if otherProbe.GetId() == probe.GetId() {
			continue
		}
		otherFailureThreshold := otherProbe.EffectiveFailureThreshold()
		otherStreak, err := c.consecutiveOutcomeStreak(ctx, otherProbe.GetId(), database.ProbeOutcomeStatusFailure, otherFailureThreshold)
		if err != nil {
			return err
		}
		if otherStreak >= otherFailureThreshold {
			return nil // another probe is still failing
		}
	}

	return c.MarkHealthState(ctx, database.ConnectionHealthStateHealthy, healthReasonPrefix+probe.GetId())
}

// consecutiveOutcomeStreak returns the number of most-recent outcomes for the
// (connection, probe) that match the given outcome, capped at limit.
//
// The "cap at limit" matters: we only need to know whether the streak crosses
// a threshold, so reading more than `limit` rows is wasted work. The caller
// passes the relevant threshold as limit; if the returned streak equals
// limit, "≥ threshold" is satisfied.
func (c *connection) consecutiveOutcomeStreak(ctx context.Context, probeId, matchOutcome string, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	rows, err := c.s.db.GetRecentProbeOutcomes(ctx, c.Id, probeId, limit)
	if err != nil {
		return 0, fmt.Errorf("get recent probe outcomes: %w", err)
	}
	n := 0
	for _, r := range rows {
		if r.Outcome != matchOutcome {
			break
		}
		n++
	}
	return n, nil
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
			return nil
		}
		return err
	}
	return c.s.db.UpdateApiKeyCredentialLastValidated(ctx, cred.Id, apctx.GetClock(ctx).Now())
}

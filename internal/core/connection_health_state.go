package core

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/database"
)

// connectionHealthStateChangedMessage is the single message string for the
// structured event emitted when a connection's health flips. Dashboards and
// alerts filter on this — keep it stable.
const connectionHealthStateChangedMessage = "connection health state changed"

// MarkHealthState updates the connection's operational health signal. It is
// the single write site for the field — both refresh-driven (this PR) and
// probe-driven (project #255) callers funnel through here so the structured
// transition event is emitted consistently.
//
// Idempotent: a call that does not change the state is a no-op and emits no
// event. This is the right shape for callers that don't track the prior
// state themselves (e.g. a refresh succeeding when the connection was
// already healthy should not produce a transition event).
func (c *connection) MarkHealthState(ctx context.Context, state database.ConnectionHealthState, reason string) error {
	if !database.IsValidConnectionHealthState(state) {
		return fmt.Errorf("invalid health state: %q", state)
	}

	prev := c.GetHealthState()
	if prev == state {
		return nil
	}

	if err := c.s.db.SetConnectionHealthState(ctx, c.Id, state); err != nil {
		return fmt.Errorf("failed to update connection health state: %w", err)
	}
	c.HealthState = state

	c.logger.LogAttrs(ctx, slog.LevelInfo, connectionHealthStateChangedMessage,
		slog.String("previous_health_state", string(prev)),
		slog.String("health_state", string(state)),
		slog.String("reason", reason),
	)
	return nil
}

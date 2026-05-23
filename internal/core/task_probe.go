package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

const taskTypeProbe = "core:probe"

func newProbeTask(connectionId apid.ID, probeId string) (*asynq.Task, error) {
	payload, err := json.Marshal(probeTaskPayload{ConnectionId: connectionId, ProbeId: probeId})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskTypeProbe, payload), nil
}

type probeTaskPayload struct {
	ConnectionId apid.ID `json:"connection_id"`
	ProbeId      string  `json:"probe_id"`
}

func skipTaskErrorIfProbeIsPeriodic(p iface.Probe, err error) error {
	if err == nil {
		return nil
	}

	if p.IsPeriodic() {
		return asynq.SkipRetry
	}

	return err
}

func (s *service) runProbeForConnection(ctx context.Context, t *asynq.Task) error {
	logger := aplog.NewBuilder(s.logger).
		WithTask(t).
		WithCtx(ctx).
		Build()
	logger.Debug("probe task started")
	defer logger.Debug("probe task completed")

	var p probeTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("%s json.Unmarshal failed: %v: %w", taskTypeDisconnectConnection, err, asynq.SkipRetry)
	}

	if p.ConnectionId == apid.Nil {
		return fmt.Errorf("%s connection id not specified: %w", taskTypeDisconnectConnection, asynq.SkipRetry)
	}

	logger = aplog.NewBuilder(logger).
		WithConnectionId(p.ConnectionId).
		With("probe_id", p.ProbeId).
		Build()

	probe, invokeErr := s.runProbeInternal(ctx, logger, p.ConnectionId, p.ProbeId)
	if probe != nil && invokeErr != nil {
		return skipTaskErrorIfProbeIsPeriodic(probe, invokeErr)
	}
	return invokeErr
}

// runProbeInternal is the shared body executed by both the periodic asynq task
// handler and the inline RunProbe entry point. It loads the connection, looks
// up the probe by id, invokes it, and records the outcome against the
// connection's health-state counters. Returns the probe (when found) and any
// error from looking it up or invoking it; health-recording errors are logged
// but not returned because they don't invalidate the probe outcome itself.
func (s *service) runProbeInternal(ctx context.Context, logger *slog.Logger, connectionId apid.ID, probeId string) (iface.Probe, error) {
	logger.Debug("getting connection")
	conn, err := s.getConnection(ctx, connectionId)
	if err != nil {
		// Arg order matters: errors.Is(err, target). s.getConnection
		// wraps database.ErrNotFound inside iface.ErrConnectionNotFound,
		// so the unwrap-aware order is required to detect the
		// not-found case.
		if errors.Is(err, database.ErrNotFound) {
			logger.Error("connection not found", "error", err)
			return nil, asynq.SkipRetry
		}

		return nil, err
	}

	probe, err := conn.GetProbe(probeId)
	if err != nil {
		if errors.Is(err, ErrProbeNotFound) {
			logger.Error("probe not found", "error", err)
			return nil, fmt.Errorf("%s probe not found: %w", taskTypeProbe, asynq.SkipRetry)
		}

		return probe, err
	}

	_, invokeErr := probe.Invoke(ctx)

	// Record the per-probe counter and (when a threshold is crossed)
	// transition the connection's health_state. Health bookkeeping is
	// best-effort: a failure here is logged but does not invalidate the
	// underlying probe outcome.
	if healthErr := conn.recordPeriodicProbeOutcome(ctx, probe, invokeErr == nil, invokeErr); healthErr != nil {
		logger.Error("failed to record probe outcome for health state", "error", healthErr)
	}

	return probe, invokeErr
}

// RunProbe invokes a single probe synchronously and records the outcome
// against health-state counters — see iface.C.RunProbe for the public
// contract. The task handler shares the same underlying logic.
func (s *service) RunProbe(ctx context.Context, connectionId apid.ID, probeId string) error {
	logger := aplog.NewBuilder(s.logger).
		WithCtx(ctx).
		WithConnectionId(connectionId).
		With("probe_id", probeId).
		Build()

	_, invokeErr := s.runProbeInternal(ctx, logger, connectionId, probeId)
	return invokeErr
}

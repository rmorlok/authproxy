package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

const taskTypeVerifyConnection = "core:verify_connection"

func newVerifyConnectionTask(connectionId apid.ID) (*asynq.Task, error) {
	payload, err := json.Marshal(verifyConnectionTaskPayload{ConnectionId: connectionId})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskTypeVerifyConnection, payload), nil
}

type verifyConnectionTaskPayload struct {
	ConnectionId apid.ID `json:"connection_id"`
}

// verifyConnection runs all probes for a connection and advances the setup flow based on the
// outcome. On success, the connection moves forward to the next step (configure or ready). On
// failure, credentials are revoked, setup_error is populated, and setup_step becomes
// "verify_failed" — a terminal pseudo-step the UI surfaces to the user with retry/cancel options.
func (s *service) verifyConnection(ctx context.Context, t *asynq.Task) error {
	logger := aplog.NewBuilder(s.logger).
		WithTask(t).
		WithCtx(ctx).
		Build()
	logger.Info("verify connection task started")
	defer logger.Info("verify connection task completed")

	var p verifyConnectionTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("%s json.Unmarshal failed: %v: %w", taskTypeVerifyConnection, err, asynq.SkipRetry)
	}

	if p.ConnectionId == apid.Nil {
		return fmt.Errorf("%s connection id not specified: %w", taskTypeVerifyConnection, asynq.SkipRetry)
	}

	logger = aplog.NewBuilder(logger).
		WithConnectionId(p.ConnectionId).
		Build()

	conn, err := s.getConnection(ctx, p.ConnectionId)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			logger.Error("connection not found", "error", err)
			return asynq.SkipRetry
		}
		return fmt.Errorf("failed to get connection for verify: %w", err)
	}

	// Guard against stale tasks: only run if the connection is still in verify phase.
	setupStep := conn.GetSetupStep()
	if setupStep == nil || *setupStep != cschema.SetupStepVerify {
		logger.Info("connection is no longer in verify phase; skipping", "setup_step", setupStep)
		return nil
	}

	probes := conn.GetProbes()
	for _, probe := range probes {
		outcome, invokeErr := probe.Invoke(ctx)
		if invokeErr == nil {
			continue
		}
		logger.Error("probe failed during verify", "probe_id", probe.GetId(), "outcome", outcome, "error", invokeErr)
		if failErr := conn.onVerifyFailed(ctx, probe.GetId(), invokeErr); failErr != nil {
			// Return the failErr so asynq retries — probe outcome is preserved for a later attempt.
			return fmt.Errorf("failed to record verify failure: %w", failErr)
		}
		return asynq.SkipRetry
	}

	// All probes passed. Advance to the next step in the flow.
	return conn.onVerifyPassed(ctx)
}

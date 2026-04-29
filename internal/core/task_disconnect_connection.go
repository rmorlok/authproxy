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
)

const taskTypeDisconnectConnection = "core:disconnect_connection"

// maxRevokeAttempts caps the number of times a revoke operation is retried
// inside a single disconnect task invocation. After exhausting attempts, the
// disconnect proceeds so a connection cannot get stuck in `disconnecting`
// because a 3rd-party revoke endpoint is misbehaving.
const maxRevokeAttempts = 3

func newDisconnectConnectionTask(connectionId apid.ID) (*asynq.Task, error) {
	payload, err := json.Marshal(disconnectConnectionTaskPayload{connectionId})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskTypeDisconnectConnection, payload), nil
}

type disconnectConnectionTaskPayload struct {
	ConnectionId apid.ID `json:"connection_id"`
}

func (s *service) disconnectConnection(ctx context.Context, t *asynq.Task) error {
	logger := aplog.NewBuilder(s.logger).
		WithTask(t).
		WithCtx(ctx).
		Build()
	logger.Info("disconnect connection task started")
	defer logger.Info("disconnect connection task completed")

	var p disconnectConnectionTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("%s json.Unmarshal failed: %v: %w", taskTypeDisconnectConnection, err, asynq.SkipRetry)
	}

	if p.ConnectionId == apid.Nil {
		return fmt.Errorf("%s connection id not specified: %w", taskTypeDisconnectConnection, asynq.SkipRetry)
	}

	conn, err := s.getConnection(ctx, p.ConnectionId)
	if err != nil {
		return fmt.Errorf("failed to get connection to disconnect connection: %w", err)
	}

	revokeOps := conn.getRevokeCredentialsOperations()
	if len(revokeOps) > 0 {
		logger.Info("revoking credentials")
		clk := apctx.GetClock(ctx)
		for _, op := range revokeOps {
			var lastErr error
			for attempt := 1; attempt <= maxRevokeAttempts; attempt++ {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				lastErr = op(ctx)
				if lastErr == nil {
					break
				}
				logger.Warn(
					"revoke attempt failed",
					"error", lastErr,
					"attempt", attempt,
					"max_attempts", maxRevokeAttempts,
				)
				if attempt < maxRevokeAttempts {
					clk.Sleep(time.Duration(attempt) * time.Second)
				}
			}
			if lastErr != nil {
				// Proceed with the rest of the disconnect so the connection
				// does not stay stuck in `disconnecting` forever.
				logger.Error(
					"revocation failed after max attempts; proceeding with disconnect",
					"error", lastErr,
					"attempts", maxRevokeAttempts,
				)
			}
		}
	}

	logger.Debug("marking connection as disconnected")
	err = conn.SetState(ctx, database.ConnectionStateDisconnected)
	if err != nil {
		logger.Error("failed to mark connection as disconnected", "error", err)
		return err
	}

	logger.Debug("deleting connection")
	err = s.db.DeleteConnection(ctx, p.ConnectionId)
	if err != nil {
		logger.Error("failed to delete connection", "error", err)
		return err
	}

	return nil
}

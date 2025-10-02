package connectors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/database"
)

const taskTypeDisconnectConnection = "connectors:disconnect_connection"

func newDisconnectConnectionTask(connectionId uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(disconnectConnectionTaskPayload{connectionId})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskTypeDisconnectConnection, payload), nil
}

type disconnectConnectionTaskPayload struct {
	ConnectionId uuid.UUID `json:"connection_id"`
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

	if p.ConnectionId == uuid.Nil {
		return fmt.Errorf("%s connection id not specified: %w", taskTypeDisconnectConnection, asynq.SkipRetry)
	}

	logger = aplog.NewBuilder(logger).
		WithConnectionId(p.ConnectionId).
		Build()

	logger.Info("disconnecting connection")

	logger.Debug("getting connection")
	connection, err := s.db.GetConnection(ctx, p.ConnectionId)
	if err != nil {
		if errors.Is(database.ErrNotFound, err) {
			logger.Error("connection not found", "error", err)
			return nil
		}

		return err
	}

	logger.Debug("getting connector for connection")
	cv, err := s.getConnectorVersion(ctx, connection.ConnectorId, connection.ConnectorVersion)
	if err != nil {
		logger.Error("failed to get connector for connection", "error", err)
		return err
	}

	revokeOps := cv.getRevokeCredentialsOperations(s, *connection)
	if len(revokeOps) > 0 {
		logger.Info("revoking credentials")
		for _, op := range revokeOps {
			err = op(ctx)
			if err != nil {
				logger.Error("failed to revoke credentials", "error", err)
				return errors.Wrap(err, "failed to revoke credentials")
			}
		}
	}

	logger.Debug("marking connection as disconnected")
	err = s.db.SetConnectionState(ctx, p.ConnectionId, database.ConnectionStateDisconnected)
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

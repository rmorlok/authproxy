package connectors

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/aplog"
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
	logger.Info("Disconnect connection task started")
	defer logger.Info("Disconnect connection task completed")

	return nil
}

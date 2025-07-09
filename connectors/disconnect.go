package connectors

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/database"
)

func (s *service) DisconnectConnection(
	ctx context.Context,
	id uuid.UUID,
) (taskId string, err error) {
	s.logger.Info("disconnecting connection", "id", id)
	err = s.db.SetConnectionState(ctx, id, database.ConnectionStateDisconnecting)
	if err != nil {
		if errors.Is(database.ErrNotFound, err) {
			// Default the error type to a 404 error
			return "", api_common.
				HttpStatusErrorBuilderFromError(err).
				WithStatusNotFound().
				Build()
		}

		return "", err
	}

	s.logger.Info("queueing disconnect connection task", "id", id)
	t, err := newDisconnectConnectionTask(id)
	if err != nil {
		return "", err
	}

	ti, err := s.ac.EnqueueContext(ctx, t)
	if err != nil {
		return "", err
	}

	s.logger.Info("disconnect connection task queued", "id", id, "task_id", ti.ID)
	return ti.ID, nil
}

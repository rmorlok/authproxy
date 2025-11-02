package connectors

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/tasks"
)

func (s *service) DisconnectConnection(
	ctx context.Context,
	id uuid.UUID,
) (taskInfo *tasks.TaskInfo, err error) {
	s.logger.Info("disconnecting connection", "id", id)
	err = s.db.SetConnectionState(ctx, id, database.ConnectionStateDisconnecting)
	if err != nil {
		if errors.Is(database.ErrNotFound, err) {
			// Default the error type to a 404 error
			return nil, api_common.
				HttpStatusErrorBuilderFromError(err).
				WithStatusNotFound().
				Build()
		}

		return nil, err
	}

	s.logger.Info("queueing disconnect connection task", "id", id)
	t, err := newDisconnectConnectionTask(id)
	if err != nil {
		return nil, err
	}

	ti, err := s.ac.EnqueueContext(ctx, t, asynq.Retention(10*time.Minute))
	if err != nil {
		return nil, err
	}

	s.logger.Info("disconnect connection task queued", "id", id, "task_id", ti.ID)
	return tasks.FromAsynqTask(ti), nil
}

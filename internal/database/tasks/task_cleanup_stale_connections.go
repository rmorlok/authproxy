package tasks

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const taskTypeCleanupStaleConnections = "database:cleanup_stale_connections"

func (th *taskHandler) cleanupStaleConnections(ctx context.Context, t *asynq.Task) error {
	th.logger.Info("running cleanup stale connections task")

	ttl := th.cfg.GetRoot().Connections.GetSetupTtlOrDefault()
	cutoff := apctx.GetClock(ctx).Now().Add(-ttl)

	var cleaned int
	err := th.db.ListConnectionsBuilder().
		WithDeletedHandling(database.DeletedHandlingExclude).
		ForStates([]database.ConnectionState{database.ConnectionStateCreated}).
		WithSetupStepNotNull().
		UpdatedBefore(cutoff).
		Enumerate(ctx, func(pr pagination.PageResult[database.Connection]) (pagination.KeepGoing, error) {
			for _, conn := range pr.Results {
				th.logger.Info("cleaning up stale connection", "id", conn.Id, "updated_at", conn.UpdatedAt)

				if err := th.db.SetConnectionState(ctx, conn.Id, database.ConnectionStateDisconnected); err != nil {
					th.logger.Error("failed to set state for stale connection", "error", err, "id", conn.Id)
					continue
				}

				if err := th.db.DeleteConnection(ctx, conn.Id); err != nil {
					th.logger.Error("failed to delete stale connection", "error", err, "id", conn.Id)
					continue
				}

				cleaned++
			}
			return pagination.Continue, nil
		})

	if err != nil {
		th.logger.Error("failed to enumerate stale connections", "error", err)
		return err
	}

	th.logger.Info("cleanup stale connections complete", "cleaned", cleaned, "ttl", ttl.String())
	return nil
}

func newCleanupStaleConnectionsTask() *asynq.Task {
	return asynq.NewTask(taskTypeCleanupStaleConnections, nil)
}

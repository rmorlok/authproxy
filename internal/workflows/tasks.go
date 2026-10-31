package workflows

import (
	"context"
	"fmt"
	"log/slog"

	wfbackend "github.com/cschleiden/go-workflows/backend"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apctx"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

const taskTypeRemoveFinishedInstances = "workflows:remove_finished_instances"

// instanceRemover is the part of the workflow runtime the cleanup task needs.
type instanceRemover interface {
	RemoveWorkflowInstances(ctx context.Context, options ...wfbackend.RemovalOption) error
}

// TaskHandler owns the Asynq maintenance tasks for the workflow runtime. It is registered with the worker's
// mux and scheduler like the other task handlers.
type TaskHandler struct {
	remover instanceRemover
	cfg     *sconfig.Tasks
	logger  *slog.Logger
}

// NewTaskHandler creates the handler for workflow maintenance tasks. cfg may be nil, in which case the default
// retention applies.
func NewTaskHandler(runtime *Runtime, cfg *sconfig.Tasks, logger *slog.Logger) *TaskHandler {
	return &TaskHandler{
		remover: runtime,
		cfg:     cfg,
		logger:  logger,
	}
}

// RegisterTasks registers the workflow maintenance task handlers with the Asynq mux.
func (h *TaskHandler) RegisterTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskTypeRemoveFinishedInstances, h.removeFinishedInstances)
}

// GetCronTasks returns the daily cleanup of finished workflow instances, or nothing when retention is disabled.
func (h *TaskHandler) GetCronTasks() []*asynq.PeriodicTaskConfig {
	if h.cfg.GetWorkflowRetention() <= 0 {
		return nil
	}

	return []*asynq.PeriodicTaskConfig{{
		Cronspec: "@daily",
		Task:     newRemoveFinishedInstancesTask(),
	}}
}

// removeFinishedInstances deletes workflow instances, and their history, that finished before the retention
// window. The backend only considers instances with a completion time, so in-progress workflows are untouched.
// The removal is idempotent, so a failed run can safely be retried by Asynq. Retention is re-checked here
// because a task enqueued before a config change to disable cleanup may still be delivered.
func (h *TaskHandler) removeFinishedInstances(ctx context.Context, _ *asynq.Task) error {
	retention := h.cfg.GetWorkflowRetention()
	if retention <= 0 {
		h.logger.Info("skipping finished workflow instance removal because retention is disabled")
		return nil
	}

	finishedBefore := apctx.GetClock(ctx).Now().Add(-retention)

	h.logger.Info("removing finished workflow instances", "retention", retention.String(), "finished_before", finishedBefore)
	if err := h.remover.RemoveWorkflowInstances(ctx, wfbackend.RemoveFinishedBefore(finishedBefore)); err != nil {
		h.logger.Error("failed to remove finished workflow instances", "error", err)
		return fmt.Errorf("remove finished workflow instances: %w", err)
	}

	h.logger.Info("removed finished workflow instances", "retention", retention.String())
	return nil
}

func newRemoveFinishedInstancesTask() *asynq.Task {
	return asynq.NewTask(taskTypeRemoveFinishedInstances, nil)
}

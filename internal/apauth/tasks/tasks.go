package tasks

import (
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// TaskRegistrar interface for registering tasks and providing cron configs.
type TaskRegistrar interface {
	RegisterTasks(mux *asynq.ServeMux)
	GetCronTasks() []*asynq.PeriodicTaskConfig
}

type taskHandler struct {
	cfg     config.C
	db      database.DB
	encrypt encrypt.E
	logger  *slog.Logger
}

// NewTaskHandler creates a new task handler for admin sync operations.
func NewTaskHandler(
	cfg config.C,
	db database.DB,
	encrypt encrypt.E,
	logger *slog.Logger,
) TaskRegistrar {
	return &taskHandler{
		cfg:     cfg,
		db:      db,
		encrypt: encrypt,
		logger:  logger,
	}
}

// RegisterTasks registers the admin sync task handlers with the asynq mux.
func (th *taskHandler) RegisterTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskTypeClearExpiredNonces, th.clearExpiredNonces)
	mux.HandleFunc(taskTypeSyncActorsExternalSource, th.syncAdminUsersExternalSource)
}

// GetCronTasks returns the cron task configurations for admin sync.
// Only returns tasks if AdminUsersExternalSource is configured.
func (th *taskHandler) GetCronTasks() []*asynq.PeriodicTaskConfig {
	adminUsers := th.cfg.GetRoot().SystemAuth.AdminUsers
	if adminUsers == nil {
		return nil
	}

	// Only create cron task for external source configuration
	aues, ok := adminUsers.InnerVal.(*sconfig.AdminUsersExternalSource)
	if !ok {
		return nil
	}

	return []*asynq.PeriodicTaskConfig{
		{
			Cronspec: aues.GetSyncCronScheduleOrDefault(),
			Task:     NewSyncActorsExternalSourceTask(),
		},
		{
			Task:     newClearExpiredNoncesTask(),
			Cronspec: "@hourly",
		},
	}
}

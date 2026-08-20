package tasks

import (
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apredis"
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
	redis   apredis.Client
	encrypt encrypt.E
	logger  *slog.Logger
}

// NewTaskHandler creates a new task handler for actor sync operations.
func NewTaskHandler(
	cfg config.C,
	db database.DB,
	redis apredis.Client,
	encrypt encrypt.E,
	logger *slog.Logger,
) TaskRegistrar {
	return &taskHandler{
		cfg:     cfg,
		db:      db,
		redis:   redis,
		encrypt: encrypt,
		logger:  logger,
	}
}

// RegisterTasks registers the actor sync task handlers with the asynq mux.
func (th *taskHandler) RegisterTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskTypeClearExpiredNonces, th.clearExpiredNonces)
	mux.HandleFunc(taskTypeSyncActorsExternalSource, th.syncConfiguredActorsExternalSource)
}

// GetCronTasks returns the cron task configurations for actor sync.
// Only returns tasks if an external actor source is configured.
func (th *taskHandler) GetCronTasks() []*asynq.PeriodicTaskConfig {
	actors := th.cfg.GetRoot().SystemAuth.Actors
	if actors == nil {
		return nil
	}

	var cronspec string
	switch sources := actors.InnerVal.(type) {
	case *sconfig.ConfiguredActorsExternalSource:
		cronspec = sources.GetSyncCronScheduleOrDefault()
	case *sconfig.ConfiguredActorsExternalSources:
		cronspec = sources.GetSyncCronScheduleOrDefault()
	default:
		return nil
	}

	return []*asynq.PeriodicTaskConfig{
		{
			Cronspec: cronspec,
			Task:     NewSyncActorsExternalSourceTask(),
		},
		{
			Task:     newClearExpiredNoncesTask(),
			Cronspec: "@hourly",
		},
	}
}

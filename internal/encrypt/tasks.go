package encrypt

import (
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
)

type EncryptServiceTaskHandler struct {
	cfg    config.C
	db     database.DB
	enc    E
	redis  apredis.Client
	logger *slog.Logger
}

func (h *EncryptServiceTaskHandler) RegisterTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(TaskTypeSyncKeysToDatabase, h.handleSyncKeysToDatabase)
	mux.HandleFunc(TaskTypeReencryptAll, h.handleReencryptAll)
}

func (h *EncryptServiceTaskHandler) GetCronTasks() []*asynq.PeriodicTaskConfig {
	return []*asynq.PeriodicTaskConfig{
		{
			Cronspec: "*/15 * * * *", // every 15 minutes
			Task:     NewSyncKeysToDatabaseTask(),
		},
	}
}

func NewEncryptServiceTaskHandler(
	cfg config.C,
	db database.DB,
	enc E,
	redis apredis.Client,
	logger *slog.Logger,
) *EncryptServiceTaskHandler {
	return &EncryptServiceTaskHandler{
		cfg:    cfg,
		db:     db,
		enc:    enc,
		redis:  redis,
		logger: logger,
	}
}

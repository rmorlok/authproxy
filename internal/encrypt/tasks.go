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
	mux.HandleFunc(TaskTypeSyncKeys, h.handleSyncKeys)
	mux.HandleFunc(TaskTypeReencryptAll, h.handleReencryptAll)
}

func (h *EncryptServiceTaskHandler) GetCronTasks() []*asynq.PeriodicTaskConfig {
	return []*asynq.PeriodicTaskConfig{
		{
			Cronspec: "*/15 * * * *", // every 15 minutes
			Task:     NewSyncKeysTask(),
		},
	}
}

func NewEncryptServiceTaskHandler(
	db database.DB,
	enc E,
	redis apredis.Client,
	logger *slog.Logger,
) *EncryptServiceTaskHandler {
	return &EncryptServiceTaskHandler{
		db:     db,
		enc:    enc,
		redis:  redis,
		logger: logger,
	}
}

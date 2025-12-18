package oauth2

import (
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
)

type taskHandler struct {
	cfg     config.C
	db      database.DB
	redis   apredis.Client
	core    coreIface.C
	asynq   apasynq.Client
	httpf   httpf.F
	encrypt encrypt.E
	factory Factory
	logger  *slog.Logger
}

type TaskRegistrar interface {
	RegisterTasks(mux *asynq.ServeMux)
	GetCronTasks() []*asynq.PeriodicTaskConfig
}

func NewTaskHandler(
	cfg config.C,
	db database.DB,
	redis apredis.Client,
	c coreIface.C,
	ac apasynq.Client,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) TaskRegistrar {
	return &taskHandler{
		cfg:     cfg,
		db:      db,
		redis:   redis,
		core:    c,
		asynq:   ac,
		httpf:   httpf,
		encrypt: encrypt,
		logger:  logger,
		factory: NewFactory(cfg, db, redis, c, httpf, encrypt, logger),
	}
}

func (th *taskHandler) RegisterTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskTypeRefreshExpiringOAuthTokens, th.refreshExpiringOauth2Tokens)
	mux.HandleFunc(taskTypeRefreshOAuthToken, th.refreshOauth2Token)
}

func (th *taskHandler) GetCronTasks() []*asynq.PeriodicTaskConfig {
	return []*asynq.PeriodicTaskConfig{
		{
			Cronspec: th.cfg.GetRoot().Oauth.GetRefreshTokensCronScheduleOrDefault(),
			Task: asynq.NewTask(
				taskTypeRefreshExpiringOAuthTokens,
				nil,
				asynq.MaxRetry(1),
			),
		},
	}
}

package oauth2

import (
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
)

type taskHandler struct {
	cfg     config.C
	db      database.DB
	redis   redis.R
	httpf   httpf.F
	encrypt encrypt.E
	factory Factory
}

type TaskRegistrar interface {
	RegisterTasks(mux *asynq.ServeMux)
}

func NewTaskHandler(
	cfg config.C,
	db database.DB,
	redis redis.R,
	httpf httpf.F,
	encrypt encrypt.E,
) TaskRegistrar {
	return &taskHandler{
		cfg:     cfg,
		db:      db,
		redis:   redis,
		httpf:   httpf,
		encrypt: encrypt,
		factory: NewFactory(cfg, db, redis, httpf, encrypt),
	}
}

func (th *taskHandler) RegisterTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskTypeRefreshOAuthToken, th.refreshOauth2Token)
}

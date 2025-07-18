package auth

import (
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/redis"
	"log/slog"
)

// service is the implementation of the core auth service.
type service struct {
	config  config.C
	service config.HttpService
	db      database.DB
	redis   redis.R
	encrypt encrypt.E
	logger  *slog.Logger
}

// NewService makes an auth service
func NewService(cfg config.C, svc config.HttpService, db database.DB, redis redis.R, e encrypt.E, logger *slog.Logger) A {
	if cfg == nil {
		panic("config is required")
	}

	if svc == nil {
		panic("service is required")
	}

	return &service{
		config:  cfg,
		service: svc,
		db:      db,
		redis:   redis,
		encrypt: e,
		logger:  logger,
	}
}

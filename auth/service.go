package auth

import (
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/redis"
	"log/slog"
)

// service is the implementation of the core auth service.
type service struct {
	// Configuration for the overall application. Provides many options that control the system.
	config config.C

	// The service using this authentication
	service config.HttpService

	// logger interface, default is no logging at all
	logger *slog.Logger

	db    database.DB
	redis redis.R
}

// NewService makes an auth service
func NewService(cfg config.C, svc config.HttpService, db database.DB, redis redis.R, logger *slog.Logger) A {
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
		logger:  logger,
	}
}

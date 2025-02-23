package auth

import (
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/logger"
	"github.com/rmorlok/authproxy/redis"
)

// service is the implementation of the core auth service.
type service struct {
	// Configuration for the overall application. Provides many options that control the system.
	config config.C

	// The service using this authentication
	service config.Service

	// logger interface, default is no logging at all
	logger logger.L

	db    database.DB
	redis redis.R
}

// NewService makes an auth service
func NewService(cfg config.C, svc config.Service, db database.DB, redis redis.R) A {
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
	}
}

func (s *service) logf(format string, args ...interface{}) {
	if s.logger == nil {
		return
	}

	s.logger.Logf(format, args...)
}

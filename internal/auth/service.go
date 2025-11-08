package auth

import (
	"log/slog"

	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
)

// service is the implementation of the core auth service.
type service struct {
	config                 config.C
	service                config.HttpService
	db                     database.DB
	r                      apredis.Client
	encrypt                encrypt.E
	logger                 *slog.Logger
	defaultActorValidators []ActorValidator
}

// NewService makes an auth service
func NewService(cfg config.C, svc config.HttpService, db database.DB, r apredis.Client, e encrypt.E, logger *slog.Logger) A {
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
		r:       r,
		encrypt: e,
		logger:  logger,
	}
}

func (s *service) WithDefaultActorValidators(validators ...ActorValidator) A {
	s2 := *s
	s2.defaultActorValidators = validators
	return &s2
}

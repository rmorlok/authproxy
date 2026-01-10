package service

import (
	"log/slog"

	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// service is the implementation of the core auth service.
type service struct {
	config                config.C
	service               sconfig.HttpService
	db                    database.DB
	r                     apredis.Client
	encrypt               encrypt.E
	logger                *slog.Logger
	defaultAuthValidators []AuthValidator
}

// NewService makes an auth service
func NewService(cfg config.C, svc sconfig.HttpService, db database.DB, r apredis.Client, e encrypt.E, logger *slog.Logger) A {
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

func (s *service) WithDefaultAuthValidators(validators ...AuthValidator) A {
	s2 := *s
	s2.defaultAuthValidators = validators
	return &s2
}

func (s *service) NewRequiredBuilder() *PermissionValidatorBuilder {
	return &PermissionValidatorBuilder{s: s}
}

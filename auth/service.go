package auth

import (
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/logger"
	"github.com/rmorlok/authproxy/redis"
)

// service service that wraps operations for validating JWTs from both headers and cookies.
type service struct {
	Opts
}

// NewService makes an auth service
func NewService(opts Opts) A {
	if opts.Config == nil {
		panic("Ops.Config is required")
	}

	if opts.ServiceId == "" {
		panic("Opts.ServiceId is required")
	}

	res := service{Opts: opts}

	return &res
}

func StandardAuthService(
	cfg config.C,
	serviceId config.ServiceId,
	db database.DB,
	redis *redis.Wrapper,
) A {
	return NewService(Opts{
		Config:    cfg,
		ServiceId: serviceId,
		Logger:    logger.Std,
		Db:        db,
		Redis:     redis,
	})
}

func (s *service) logf(format string, args ...interface{}) {
	if s.Opts.Logger == nil {
		return
	}

	s.Opts.Logger.Logf(format, args...)
}

func (s *service) apiHost() *config.ApiHost {
	return s.Config.MustApiHostForService(s.ServiceId)
}

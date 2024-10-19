package auth

import (
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/logger"
)

// Service service that wraps operations for validating JWTs from both headers and cookies.
type Service struct {
	Opts
}

// NewService makes an auth service
func NewService(opts Opts) *Service {
	if opts.Config == nil {
		panic("Ops.Config is required")
	}

	if opts.ServiceId == "" {
		panic("Opts.ServiceId is required")
	}

	res := Service{Opts: opts}

	return &res
}

func StandardAuthService(cfg config.C, serviceId config.ServiceId) *Service {
	return NewService(Opts{
		Config:    cfg,
		ServiceId: serviceId,
		Logger:    logger.Std,
	})
}

func (s *Service) logf(format string, args ...interface{}) {
	if s.Opts.Logger == nil {
		return
	}

	s.Opts.Logger.Logf(format, args...)
}

func (s *Service) apiHost() *config.ApiHost {
	return s.Config.MustApiHostForService(s.ServiceId)
}

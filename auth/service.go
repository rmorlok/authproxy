package auth

import (
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/logger"
)

// Auth service that wraps operations for validating JWTs from both headers and cookies.
type Service struct {
	Opts
}

// NewService makes an auth service
func NewService(opts Opts) *Service {
	if opts.Config == nil {
		panic("Ops.Config is required")
	}

	if opts.ApiHost == nil {
		panic("Opts.ApiHost is required")
	}

	res := Service{Opts: opts}

	return &res
}

func StandardAuthService(cfg *config.Root, apiHost *config.ApiHost) *Service {
	return NewService(Opts{
		Config:  cfg,
		ApiHost: apiHost,
		SecretReader: SecretFunc(func(id string) (string, error) { // secret key for JWT
			return "some-secret", nil
		}),
		Logger: logger.Std,
	})
}

func (s *Service) logf(format string, args ...interface{}) {
	if s.Opts.Logger == nil {
		return
	}

	s.Opts.Logger.Logf(format, args...)
}

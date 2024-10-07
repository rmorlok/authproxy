package auth

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

func (s *Service) logf(format string, args ...interface{}) {
	if s.Opts.Logger == nil {
		return
	}

	s.Opts.Logger.Logf(format, args...)
}

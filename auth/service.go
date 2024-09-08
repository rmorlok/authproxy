package auth

// Auth service that wraps operations for validating JWTs from both headers and cookies.
type Service struct {
	Opts
}

// NewService makes an auth service
func NewService(opts Opts) *Service {
	res := Service{Opts: opts}

	setDefault := func(fld *string, def string) {
		if *fld == "" {
			*fld = def
		}
	}

	setDefault(&res.JWTCookieName, defaultJWTCookieName)
	setDefault(&res.JWTHeaderKey, defaultJWTHeaderKey)
	setDefault(&res.XSRFCookieName, defaultXSRFCookieName)
	setDefault(&res.XSRFHeaderKey, defaultXSRFHeaderKey)
	setDefault(&res.JWTQuery, defaultTokenQuery)
	setDefault(&res.Issuer, defaultIssuer)
	setDefault(&res.JWTCookieDomain, defaultJWTCookieDomain)

	if opts.TokenDuration == 0 {
		res.TokenDuration = defaultTokenDuration
	}

	if opts.CookieDuration == 0 {
		res.CookieDuration = defaultCookieDuration
	}

	return &res
}

func (s *Service) logf(format string, args ...interface{}) {
	if s.Opts.Logger == nil {
		return
	}

	s.Opts.Logger.Logf(format, args...)
}

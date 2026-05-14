package core

import "github.com/rmorlok/authproxy/internal/auth_methods/oauth2"

func (s *service) getOAuth2Factory() oauth2.Factory {
	s.o2FactoryOnce.Do(func() {
		var opts []oauth2.FactoryOption
		if s.telProviders != nil {
			opts = append(opts, oauth2.WithTelemetry(s.telProviders, s.telCfg))
		}
		s.o2Factory = oauth2.NewFactory(s.cfg, s.db, s.r, s, s.httpf, s.encrypt, s.logger, opts...)
	})

	return s.o2Factory
}

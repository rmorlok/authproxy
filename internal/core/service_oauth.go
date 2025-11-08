package core

import "github.com/rmorlok/authproxy/internal/auth_methods/oauth2"

func (s *service) getOAuth2Factory() oauth2.Factory {
	s.o2FactoryOnce.Do(func() {
		s.o2Factory = oauth2.NewFactory(s.cfg, s.db, s.r, s, s.httpf, s.encrypt, s.logger)
	})

	return s.o2Factory
}

package core

import "github.com/rmorlok/authproxy/internal/auth_methods/api_key"

// getApiKeyFactory returns the lazily-constructed api-key proxy factory.
// Mirrors getOAuth2Factory — one factory per service, shared across all
// api-key connections.
func (s *service) getApiKeyFactory() api_key.Factory {
	s.apiKeyFactoryOnce.Do(func() {
		s.apiKeyFactory = api_key.NewFactory(s.db, s.encrypt, s.httpf, s.logger)
	})
	return s.apiKeyFactory
}

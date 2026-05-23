package core

import (
	"github.com/rmorlok/authproxy/internal/auth_methods/api_key"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// getApiKeyFactory returns the api-key factory from the registry, typed as
// api_key.Factory. Today api_key.Factory has no extras beyond the generic
// auth_methods.Factory surface, but later subtickets (#364 credential
// persistence move) will add api-key-specific methods that this typed
// accessor exposes. Generic call sites should use getAuthMethodFactory
// instead.
func (s *service) getApiKeyFactory() api_key.Factory {
	return s.authMethodFactories[cschema.AuthTypeAPIKey].(api_key.Factory)
}

package core

import (
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// getOAuth2Factory returns the OAuth2 factory from the registry, typed as
// oauth2.Factory so OAuth2-specific extras (NewOAuth2, GetOAuth2State) are
// available. Call sites that only need NewAuthenticator should resolve
// through getAuthMethodFactory instead — this accessor is reserved for the
// OAuth2-specific paths (token state lookup, manual OAuth2Connection
// construction).
func (s *service) getOAuth2Factory() oauth2.Factory {
	return s.authMethodFactories[cschema.AuthTypeOAuth2].(oauth2.Factory)
}

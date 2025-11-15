package connectors

import (
	"github.com/rmorlok/authproxy/internal/config/common"
)

type AuthOAuth2 struct {
	Type          AuthType                `json:"type" yaml:"type"`
	ClientId      *common.StringValue     `json:"client_id" yaml:"client_id"`
	ClientSecret  *common.StringValue     `json:"client_secret" yaml:"client_secret"`
	Scopes        []Scope                 `json:"scopes" yaml:"scopes"`
	Authorization AuthOauth2Authorization `json:"authorization" yaml:"authorization"`
	Token         AuthOauth2Token         `json:"token" yaml:"token"`
	Revocation    *AuthOauth2Revocation   `json:"revocation,omitempty" yaml:"revocation,omitempty"`
}

func (a *AuthOAuth2) GetType() AuthType {
	return AuthTypeOAuth2
}

func (a *AuthOAuth2) Clone() AuthImpl {
	if a == nil {
		return nil
	}

	clone := *a

	if a.ClientId != nil {
		clone.ClientId = a.ClientId.CloneValue()
	}

	if a.ClientSecret != nil {
		clone.ClientSecret = a.ClientSecret.CloneValue()
	}

	scopes := make([]Scope, 0, len(a.Scopes))
	for _, scope := range a.Scopes {
		scopes = append(scopes, scope)
	}
	clone.Scopes = scopes

	return &clone
}

var _ AuthImpl = (*AuthOAuth2)(nil)

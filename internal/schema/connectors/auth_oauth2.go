package connectors

import (
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
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

// ValidateMustacheReferences cross-checks every templated field on the OAuth2 auth
// definition against the field-availability data in mctx. Authorization and Token
// templates render during the auth phase and may only reference preconnect fields;
// Revocation templates render after setup completes and may reference any cfg field.
func (a *AuthOAuth2) ValidateMustacheReferences(vc *common.ValidationContext, mctx *MustacheValidationContext) error {
	if a == nil || mctx == nil {
		return nil
	}

	result := &multierror.Error{}
	preconnectFields := mctx.PreconnectFields
	allConfigFields := mctx.AllConfigFields

	checkMustacheTemplate(vc.PushField("authorization").PushField("endpoint"), a.Authorization.Endpoint, preconnectFields, "preconnect", result)
	for k, v := range a.Authorization.QueryOverrides {
		checkMustacheTemplate(vc.PushField("authorization").PushField("query_overrides").PushField(k), v, preconnectFields, "preconnect", result)
	}

	checkMustacheTemplate(vc.PushField("token").PushField("endpoint"), a.Token.Endpoint, preconnectFields, "preconnect", result)
	for k, v := range a.Token.QueryOverrides {
		checkMustacheTemplate(vc.PushField("token").PushField("query_overrides").PushField(k), v, preconnectFields, "preconnect", result)
	}
	for k, v := range a.Token.FormOverrides {
		checkMustacheTemplate(vc.PushField("token").PushField("form_overrides").PushField(k), v, preconnectFields, "preconnect", result)
	}

	if a.Revocation != nil {
		checkMustacheTemplate(vc.PushField("revocation").PushField("endpoint"), a.Revocation.Endpoint, allConfigFields, "setup flow", result)
		for k, v := range a.Revocation.QueryOverrides {
			checkMustacheTemplate(vc.PushField("revocation").PushField("query_overrides").PushField(k), v, allConfigFields, "setup flow", result)
		}
		for k, v := range a.Revocation.FormOverrides {
			checkMustacheTemplate(vc.PushField("revocation").PushField("form_overrides").PushField(k), v, allConfigFields, "setup flow", result)
		}
	}

	return result.ErrorOrNil()
}

var _ MustacheValidator = (*AuthOAuth2)(nil)

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

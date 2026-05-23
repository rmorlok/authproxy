package connectors

import (
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// TokenEndpointAuthMethod selects how the connector authenticates itself to
// the OAuth2 token endpoint per RFC 6749 §2.3.1 / RFC 7591 §2. Values follow
// the RFC 7591 token_endpoint_auth_method registry.
type TokenEndpointAuthMethod string

const (
	// TokenEndpointAuthClientSecretPost sends client_id and client_secret in
	// the form-encoded request body. This is the current AuthProxy default
	// for backwards compatibility — every connector authored before this
	// option existed assumed post-body credentials.
	TokenEndpointAuthClientSecretPost = TokenEndpointAuthMethod("client_secret_post")
	// TokenEndpointAuthClientSecretBasic sends "client_id:client_secret"
	// URL-form-encoded then base64-encoded in an HTTP Basic Authorization
	// header. RFC 6749 §2.3.1 designates this as the canonical method
	// ("The authorization server MUST support" it) and many providers
	// (Google, Salesforce, Okta variants) reject the post-body form.
	TokenEndpointAuthClientSecretBasic = TokenEndpointAuthMethod("client_secret_basic")
	// TokenEndpointAuthNone marks the connector as a public client
	// (RFC 7591 §2). The token-endpoint POST sends client_id only — no
	// secret, no Authorization header. Public clients have no
	// proof-of-possession from a secret, so we additionally require PKCE
	// (RFC 7636) for the authorize/exchange to be meaningful.
	TokenEndpointAuthNone = TokenEndpointAuthMethod("none")
)

type AuthOAuth2 struct {
	Type AuthType `json:"type" yaml:"type"`
	// TokenEndpointAuthMethod selects how client credentials are presented
	// to the token endpoint. When empty, defaults to client_secret_post —
	// matches the proxy's behavior before this field existed.
	TokenEndpointAuthMethod TokenEndpointAuthMethod `json:"token_endpoint_auth_method,omitempty" yaml:"token_endpoint_auth_method,omitempty"`
	ClientId                *common.StringValue     `json:"client_id,omitempty" yaml:"client_id,omitempty"`
	ClientSecret            *common.StringValue     `json:"client_secret,omitempty" yaml:"client_secret,omitempty"`
	Scopes                  []Scope                 `json:"scopes" yaml:"scopes"`
	Authorization           AuthOauth2Authorization `json:"authorization" yaml:"authorization"`
	Token                   AuthOauth2Token         `json:"token" yaml:"token"`
	Revocation              *AuthOauth2Revocation   `json:"revocation,omitempty" yaml:"revocation,omitempty"`
}

// GetTokenEndpointAuthMethodOrDefault returns the configured method, defaulting
// to client_secret_post when the field was omitted. Keeps the default in one
// place so callsites don't have to repeat the empty-string check.
func (a *AuthOAuth2) GetTokenEndpointAuthMethodOrDefault() TokenEndpointAuthMethod {
	if a == nil || a.TokenEndpointAuthMethod == "" {
		return TokenEndpointAuthClientSecretPost
	}
	return a.TokenEndpointAuthMethod
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

	clone.Authorization.PKCE = a.Authorization.PKCE.Clone()

	return &clone
}

// Validate enforces OAuth2-specific schema invariants: the optional PKCE
// block, and the optional token_endpoint_auth_method selector and its
// cross-field requirements on client_secret / PKCE.
func (a *AuthOAuth2) Validate(vc *common.ValidationContext) error {
	if a == nil {
		return nil
	}

	result := &multierror.Error{}

	if a.Authorization.PKCE != nil {
		pkceVC := vc.PushField("authorization").PushField("pkce")
		switch a.Authorization.PKCE.Method {
		case "", PKCEMethodS256, PKCEMethodPlain:
			// "" defaults to S256 at runtime — accept here.
		default:
			result = multierror.Append(result, pkceVC.NewErrorfForField("method",
				"%q is not a valid PKCE method; must be %q or %q",
				a.Authorization.PKCE.Method, PKCEMethodS256, PKCEMethodPlain,
			))
		}
	}

	hasSecret := a.ClientSecret != nil && a.ClientSecret.InnerVal != nil
	switch a.TokenEndpointAuthMethod {
	case "", TokenEndpointAuthClientSecretPost, TokenEndpointAuthClientSecretBasic:
		if !hasSecret {
			result = multierror.Append(result, vc.NewErrorfForField("client_secret",
				"is required when token_endpoint_auth_method is %q",
				a.GetTokenEndpointAuthMethodOrDefault(),
			))
		}
	case TokenEndpointAuthNone:
		if hasSecret {
			result = multierror.Append(result, vc.NewErrorfForField("client_secret",
				"must be omitted when token_endpoint_auth_method is %q",
				TokenEndpointAuthNone,
			))
		}
		if a.Authorization.PKCE == nil {
			result = multierror.Append(result, vc.PushField("authorization").NewErrorfForField("pkce",
				"is required when token_endpoint_auth_method is %q (public clients have no proof-of-possession without PKCE)",
				TokenEndpointAuthNone,
			))
		}
	default:
		result = multierror.Append(result, vc.NewErrorfForField("token_endpoint_auth_method",
			"%q is not a valid method; must be %q, %q, or %q",
			a.TokenEndpointAuthMethod,
			TokenEndpointAuthClientSecretPost, TokenEndpointAuthClientSecretBasic, TokenEndpointAuthNone,
		))
	}

	return result.ErrorOrNil()
}

var _ AuthImpl = (*AuthOAuth2)(nil)
var _ AuthValidator = (*AuthOAuth2)(nil)

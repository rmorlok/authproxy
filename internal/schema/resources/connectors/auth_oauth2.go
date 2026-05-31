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

type OAuth2GrantType string

const (
	OAuth2GrantAuthorizationCode = OAuth2GrantType("authorization_code")
	OAuth2GrantClientCredentials = OAuth2GrantType("client_credentials")
)

type AuthOAuth2 struct {
	Type AuthType `json:"type" yaml:"type"`
	// GrantType selects the OAuth2 grant flow. nil preserves the historical
	// authorization-code behavior.
	GrantType *OAuth2GrantType `json:"grant_type,omitempty" yaml:"grant_type,omitempty"`
	// TokenEndpointAuthMethod selects how client credentials are presented
	// to the token endpoint. nil signals "use the default" and resolves to
	// client_secret_post via GetTokenEndpointAuthMethodOrDefault, matching
	// the proxy's behavior before this field existed. An explicit
	// empty-string value is rejected by the validator — it is not a valid
	// method per RFC 7591 §2 and must not silently fall through to the
	// default.
	TokenEndpointAuthMethod *TokenEndpointAuthMethod `json:"token_endpoint_auth_method,omitempty" yaml:"token_endpoint_auth_method,omitempty"`
	ClientId                *common.StringValue      `json:"client_id,omitempty" yaml:"client_id,omitempty"`
	ClientSecret            *common.StringValue      `json:"client_secret,omitempty" yaml:"client_secret,omitempty"`
	Scopes                  []Scope                  `json:"scopes" yaml:"scopes"`
	Authorization           AuthOauth2Authorization  `json:"authorization" yaml:"authorization"`
	Token                   AuthOauth2Token          `json:"token" yaml:"token"`
	Revocation              *AuthOauth2Revocation    `json:"revocation,omitempty" yaml:"revocation,omitempty"`
}

// NewTokenEndpointAuthMethod returns a pointer to m. Convenience constructor
// for callers that need to set the optional connector field — Go does not
// allow taking the address of a typed-string constant directly.
func NewTokenEndpointAuthMethod(m TokenEndpointAuthMethod) *TokenEndpointAuthMethod {
	return &m
}

func NewOAuth2GrantType(g OAuth2GrantType) *OAuth2GrantType {
	return &g
}

// GetTokenEndpointAuthMethodOrDefault returns the configured method, defaulting
// to client_secret_post only when the field was omitted (nil). This is the
// single place where the nil → post collapse happens; callers that already
// invoke this method receive a resolved, non-empty value and must not apply
// any further default. An explicitly-set empty-string value is preserved as
// empty — it is the validator's job to reject it, not this getter's.
func (a *AuthOAuth2) GetTokenEndpointAuthMethodOrDefault() TokenEndpointAuthMethod {
	if a == nil || a.TokenEndpointAuthMethod == nil {
		return TokenEndpointAuthClientSecretPost
	}
	return *a.TokenEndpointAuthMethod
}

func (a *AuthOAuth2) GetGrantTypeOrDefault() OAuth2GrantType {
	if a == nil || a.GrantType == nil {
		return OAuth2GrantAuthorizationCode
	}
	return *a.GrantType
}

func (a *AuthOAuth2) SupportsRefreshToken() bool {
	return a.GetGrantTypeOrDefault() == OAuth2GrantAuthorizationCode
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

	if a.GetGrantTypeOrDefault() == OAuth2GrantAuthorizationCode {
		checkMustacheTemplate(vc.PushField("authorization").PushField("endpoint"), a.Authorization.Endpoint, preconnectFields, "preconnect", result)
		for k, v := range a.Authorization.QueryOverrides {
			checkMustacheTemplate(vc.PushField("authorization").PushField("query_overrides").PushField(k), v, preconnectFields, "preconnect", result)
		}
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
	grantType := a.GetGrantTypeOrDefault()
	switch grantType {
	case OAuth2GrantAuthorizationCode, OAuth2GrantClientCredentials:
	case "":
		result = multierror.Append(result, vc.NewErrorfForField("grant_type",
			"must not be empty; omit the field to use the default (%q), or set one of %q, %q",
			OAuth2GrantAuthorizationCode,
			OAuth2GrantAuthorizationCode, OAuth2GrantClientCredentials,
		))
		return result.ErrorOrNil()
	default:
		result = multierror.Append(result, vc.NewErrorfForField("grant_type",
			"%q is not a valid grant type; must be %q or %q",
			grantType,
			OAuth2GrantAuthorizationCode, OAuth2GrantClientCredentials,
		))
	}

	if grantType == OAuth2GrantClientCredentials {
		if a.Authorization.Endpoint != "" || len(a.Authorization.QueryOverrides) > 0 || a.Authorization.PKCE != nil {
			result = multierror.Append(result, vc.NewErrorfForField("authorization",
				"must be omitted when grant_type is %q", OAuth2GrantClientCredentials,
			))
		}
		if a.ClientId != nil {
			result = multierror.Append(result, vc.NewErrorfForField("client_id",
				"must be omitted when grant_type is %q; client credentials are collected during connection setup",
				OAuth2GrantClientCredentials,
			))
		}
		if a.ClientSecret != nil {
			result = multierror.Append(result, vc.NewErrorfForField("client_secret",
				"must be omitted when grant_type is %q; client credentials are collected during connection setup",
				OAuth2GrantClientCredentials,
			))
		}
	} else if a.Authorization.Endpoint == "" {
		result = multierror.Append(result, vc.PushField("authorization").NewErrorfForField("endpoint",
			"is required when grant_type is %q", OAuth2GrantAuthorizationCode,
		))
	}

	if grantType == OAuth2GrantAuthorizationCode && a.Authorization.PKCE != nil {
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

	// An explicit empty-string value is rejected here rather than silently
	// resolved to the default — the YAML author wrote *something* and we
	// owe them a clear error rather than a surprising fallback. nil
	// (field omitted) is the only "use the default" signal.
	if a.TokenEndpointAuthMethod != nil && *a.TokenEndpointAuthMethod == "" {
		result = multierror.Append(result, vc.NewErrorfForField("token_endpoint_auth_method",
			"must not be empty; omit the field to use the default (%q), or set one of %q, %q, %q",
			TokenEndpointAuthClientSecretPost,
			TokenEndpointAuthClientSecretPost, TokenEndpointAuthClientSecretBasic, TokenEndpointAuthNone,
		))
		return result.ErrorOrNil()
	}

	hasClientId := a.ClientId != nil && a.ClientId.InnerVal != nil
	hasSecret := a.ClientSecret != nil && a.ClientSecret.InnerVal != nil
	if grantType == OAuth2GrantAuthorizationCode && !hasClientId {
		result = multierror.Append(result, vc.NewErrorfForField("client_id", "is required"))
	}

	method := a.GetTokenEndpointAuthMethodOrDefault()
	switch method {
	case TokenEndpointAuthClientSecretPost, TokenEndpointAuthClientSecretBasic:
		if grantType == OAuth2GrantAuthorizationCode && !hasSecret {
			result = multierror.Append(result, vc.NewErrorfForField("client_secret",
				"is required when token_endpoint_auth_method is %q", method,
			))
		}
	case TokenEndpointAuthNone:
		if hasSecret {
			result = multierror.Append(result, vc.NewErrorfForField("client_secret",
				"must be omitted when token_endpoint_auth_method is %q",
				TokenEndpointAuthNone,
			))
		}
		if grantType == OAuth2GrantAuthorizationCode && a.Authorization.PKCE == nil {
			result = multierror.Append(result, vc.PushField("authorization").NewErrorfForField("pkce",
				"is required when token_endpoint_auth_method is %q (public clients have no proof-of-possession without PKCE)",
				TokenEndpointAuthNone,
			))
		}
	default:
		result = multierror.Append(result, vc.NewErrorfForField("token_endpoint_auth_method",
			"%q is not a valid method; must be %q, %q, or %q",
			method,
			TokenEndpointAuthClientSecretPost, TokenEndpointAuthClientSecretBasic, TokenEndpointAuthNone,
		))
	}

	return result.ErrorOrNil()
}

var _ AuthImpl = (*AuthOAuth2)(nil)
var _ AuthValidator = (*AuthOAuth2)(nil)

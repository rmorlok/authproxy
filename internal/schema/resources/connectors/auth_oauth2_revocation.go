package connectors

type AuthOAuth2RevocationSupportedType string

const (
	// AuthOAuth2RevocationSupportedTypeAccessToken says the 3rd party supports only revoking access tokens
	AuthOAuth2RevocationSupportedTypeAccessToken AuthOAuth2RevocationSupportedType = "access_token"

	// AuthOAuth2RevocationSupportedTypeRefreshToken says the 3rd party supports only revoking refresh tokens
	AuthOAuth2RevocationSupportedTypeRefreshToken AuthOAuth2RevocationSupportedType = "refresh_token"

	// AuthOAuth2RevocationSupportedTypeBoth says the 3rd party supports revoking both refresh and access tokens.
	AuthOAuth2RevocationSupportedTypeBoth AuthOAuth2RevocationSupportedType = "both"
)

// AuthOauth2Revocation configures token revocation per RFC7009 (https://datatracker.ietf.org/doc/html/rfc7009)
type AuthOauth2Revocation struct {
	// Endpoint is the endpoint on the 3rd party to receive the token revocation request. Note that tokens will
	// be sent to this endpoint as part of the revocation process.
	Endpoint string `json:"endpoint" yaml:"endpoint"`

	// SupportedTokens are the tokens to be revoked. If unspecified, both access and refresh tokens are revoked.
	SupportedTokens *AuthOAuth2RevocationSupportedType `json:"supported_tokens,omitempty" yaml:"supported_tokens,omitempty"`

	// QueryOverrides are query parameters that are applied on top of the request. These can be additional query
	// parameters applied on top of the RFC ones, or if the RFC parameters are included here, they will override.
	QueryOverrides map[string]string `json:"query_overrides,omitempty" yaml:"query_overrides,omitempty"`

	// FormOverrides are additional parameters that are included in the form data posted to the 3rd party. This will
	// override data as part of the standard request if there is overlap.
	FormOverrides map[string]string `json:"form_overrides,omitempty" yaml:"form_overrides,omitempty"`
}

func (a *AuthOauth2Revocation) SupportRevokingAccessToken() bool {
	if a == nil {
		return false
	}

	if a.SupportedTokens == nil {
		return true
	}

	return *a.SupportedTokens == AuthOAuth2RevocationSupportedTypeAccessToken ||
		*a.SupportedTokens == AuthOAuth2RevocationSupportedTypeBoth
}

func (a *AuthOauth2Revocation) SupportRevokingRefreshToken() bool {
	if a == nil {
		return false
	}

	if a.SupportedTokens == nil {
		return true
	}

	return *a.SupportedTokens == AuthOAuth2RevocationSupportedTypeRefreshToken ||
		*a.SupportedTokens == AuthOAuth2RevocationSupportedTypeBoth
}

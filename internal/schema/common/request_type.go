package common

import "fmt"

// RequestType identifies the kind of traffic flowing through the proxy. The
// canonical definition lives here so downstream packages (httpf, schema/*,
// runtime enforcement) can reference a single source of truth — including its
// JSON-Schema validation in schema.json.
type RequestType string

const (
	// RequestTypeGlobal is used for context-free internal traffic that does
	// not belong to a specific connection or actor (e.g., system tasks).
	RequestTypeGlobal RequestType = "global"

	// RequestTypeProxy is a user-initiated proxied call to a 3rd-party API.
	RequestTypeProxy RequestType = "proxy"

	// RequestTypeOAuth is a coarse-grained OAuth2 request. It is retained
	// for backward compatibility while the system migrates toward the
	// finer-grained RequestTypeOAuth2* values below.
	RequestTypeOAuth RequestType = "oauth"

	// RequestTypePublic is a public-facing endpoint request (OAuth callbacks,
	// marketplace, etc.).
	RequestTypePublic RequestType = "public"

	// RequestTypeProbe is a connector-defined health probe.
	RequestTypeProbe RequestType = "probe"

	// RequestTypeOAuth2TokenExchange is an OAuth2 authorization-code → token
	// exchange call.
	RequestTypeOAuth2TokenExchange RequestType = "oauth2_token_exchange"

	// RequestTypeOAuth2Refresh is an OAuth2 refresh-token call.
	RequestTypeOAuth2Refresh RequestType = "oauth2_refresh"

	// RequestTypeOAuth2Revocation is an OAuth2 token-revocation call.
	RequestTypeOAuth2Revocation RequestType = "oauth2_revocation"
)

// AllRequestTypes returns every recognised RequestType. Callers that need to
// iterate over the full enum (e.g., schema generators, validators) should use
// this rather than maintaining their own list.
func AllRequestTypes() []RequestType {
	return []RequestType{
		RequestTypeGlobal,
		RequestTypeProxy,
		RequestTypeOAuth,
		RequestTypePublic,
		RequestTypeProbe,
		RequestTypeOAuth2TokenExchange,
		RequestTypeOAuth2Refresh,
		RequestTypeOAuth2Revocation,
	}
}

// IsValidRequestType reports whether t is a recognised RequestType value.
func IsValidRequestType[T string | RequestType](t T) bool {
	switch RequestType(t) {
	case RequestTypeGlobal,
		RequestTypeProxy,
		RequestTypeOAuth,
		RequestTypePublic,
		RequestTypeProbe,
		RequestTypeOAuth2TokenExchange,
		RequestTypeOAuth2Refresh,
		RequestTypeOAuth2Revocation:
		return true
	default:
		return false
	}
}

// Validate returns nil if r is a recognised value, or a descriptive error
// otherwise.
func (r RequestType) Validate() error {
	if !IsValidRequestType(r) {
		return fmt.Errorf("unknown request type %q", string(r))
	}
	return nil
}

// String returns the string representation of the RequestType.
func (r RequestType) String() string {
	return string(r)
}

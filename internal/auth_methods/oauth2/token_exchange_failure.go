package oauth2

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apid"
)

// tokenExchangeCategory identifies the class of failure observed during the
// authorization-code → token exchange leg of the OAuth2 callback. Like
// rejectionCategory, the value is the primary axis for downstream alerting
// and dashboards — existing values must remain stable.
type tokenExchangeCategory string

const (
	// tokenExchangeProviderDenied — the provider returned the user-denial
	// shape (RFC 6749 §4.1.2.1: redirect carries `error=` instead of `code=`).
	// User refused consent or the provider couldn't satisfy the request.
	tokenExchangeProviderDenied tokenExchangeCategory = "provider_denied"
	// tokenExchangeMissingCode — callback had no `code` and no `error`. Either
	// a malformed provider response or a probe.
	tokenExchangeMissingCode tokenExchangeCategory = "missing_code"
	// tokenExchangeNetworkError — the POST to the token endpoint failed at
	// the transport layer (DNS, dial, TLS, read timeout). The provider never
	// produced a status code.
	tokenExchangeNetworkError tokenExchangeCategory = "network_error"
	// tokenExchangeInvalidGrant — provider returned a 4xx with
	// `error=invalid_grant` (RFC 6749 §5.2). Covers expired authorization
	// code, code already used, and redirect_uri mismatch — providers fold
	// all three into this single response.
	tokenExchangeInvalidGrant tokenExchangeCategory = "invalid_grant"
	// tokenExchangeInvalidClient — provider returned `error=invalid_client`.
	// Misconfigured client_id or client_secret on the proxy side.
	tokenExchangeInvalidClient tokenExchangeCategory = "invalid_client"
	// tokenExchangeInvalidRequest — provider returned `error=invalid_request`.
	// Malformed token-endpoint request (missing parameter, duplicate, etc.).
	tokenExchangeInvalidRequest tokenExchangeCategory = "invalid_request"
	// tokenExchangeUnauthorizedClient — provider returned
	// `error=unauthorized_client`. Client is not authorized for the grant
	// type.
	tokenExchangeUnauthorizedClient tokenExchangeCategory = "unauthorized_client"
	// tokenExchangeUnsupportedGrantType — provider returned
	// `error=unsupported_grant_type`.
	tokenExchangeUnsupportedGrantType tokenExchangeCategory = "unsupported_grant_type"
	// tokenExchangeInvalidScope — provider returned `error=invalid_scope`.
	tokenExchangeInvalidScope tokenExchangeCategory = "invalid_scope"
	// tokenExchangeProvider4xxOther — non-200 4xx response that did not
	// include a recognized RFC 6749 §5.2 error code in the body.
	tokenExchangeProvider4xxOther tokenExchangeCategory = "provider_4xx_other"
	// tokenExchangeProvider5xx — provider returned 500/502/503/504. Treated
	// as transient — retry policy applies in production callers.
	tokenExchangeProvider5xx tokenExchangeCategory = "provider_5xx"
	// tokenExchangeMalformedResponse — token endpoint returned 200 but the
	// body could not be parsed as a token response, or the parsed response
	// was missing access_token.
	tokenExchangeMalformedResponse tokenExchangeCategory = "malformed_response"
	// tokenExchangeStateCleanupError — failed to delete the consumed state
	// from Redis prior to exchange. Defensive: we abort the exchange to
	// avoid minting a token against a state we couldn't invalidate.
	tokenExchangeStateCleanupError tokenExchangeCategory = "state_cleanup_error"
	// tokenExchangeInternalError — a proxy-side error not attributable to
	// the provider (config rendering, credential decryption, post-auth
	// state transition, etc.).
	tokenExchangeInternalError tokenExchangeCategory = "internal_error"
)

// tokenExchangeAttrs carries the safe-to-log identifiers associated with a
// token-exchange failure. ProviderError, when populated, is the exact value
// of the `error` field in the provider's RFC 6749 §5.2 error body — it's
// safe to log because the spec defines it as an enumerated, non-secret
// status code. ProviderStatusCode is the HTTP status, also safe.
type tokenExchangeAttrs struct {
	StateId            apid.ID
	ActorId            apid.ID
	ConnectionId       apid.ID
	Namespace          string
	ProviderStatusCode int
	ProviderError      string
	Err                error
}

// tokenExchangeFailureMessage is the single message string for every
// token-exchange failure event. Operators filter on this exact string when
// correlating across categories — keep it stable.
const tokenExchangeFailureMessage = "oauth token exchange failed"

// emitTokenExchangeFailure logs a structured "oauth token exchange failed"
// event at Warn. Only populated attrs are emitted, so callers supply
// whatever they have when the failure is observed.
func emitTokenExchangeFailure(ctx context.Context, logger *slog.Logger, category tokenExchangeCategory, attrs tokenExchangeAttrs) {
	if logger == nil {
		logger = slog.Default()
	}

	b := aplog.NewBuilder(logger).WithCtx(ctx)
	if attrs.ConnectionId != apid.Nil {
		b = b.WithConnectionId(attrs.ConnectionId)
	}
	if attrs.Namespace != "" {
		b = b.WithNamespace(attrs.Namespace)
	}

	args := []any{slog.String("category", string(category))}
	if attrs.StateId != apid.Nil {
		args = append(args, slog.String("state_id", attrs.StateId.String()))
	}
	if attrs.ActorId != apid.Nil {
		args = append(args, slog.String("actor_id", attrs.ActorId.String()))
	}
	if attrs.ProviderStatusCode != 0 {
		args = append(args, slog.Int("provider_status_code", attrs.ProviderStatusCode))
	}
	if attrs.ProviderError != "" {
		args = append(args, slog.String("provider_error", attrs.ProviderError))
	}
	if attrs.Err != nil {
		args = append(args, slog.String("error", attrs.Err.Error()))
	}

	b.Build().WarnContext(ctx, tokenExchangeFailureMessage, args...)
}

// classifyTokenEndpointStatus inspects a non-2xx token endpoint response and
// returns (category, providerErrorCode). Per RFC 6749 §5.2 the token endpoint
// reports errors via JSON body { "error": "..." }; recognized values map to
// stable per-error categories. 5xx responses are categorized as transient
// regardless of body. Unrecognized 4xx responses — including ones with no
// parseable body — become provider_4xx_other so they remain observable.
//
// The returned providerErrorCode is the raw `error` string from the body
// (empty if absent or unparseable). It's safe to surface in logs because RFC
// 6749 §5.2 defines `error` as an enumerated status code, never a secret.
func classifyTokenEndpointStatus(statusCode int, body []byte) (tokenExchangeCategory, string) {
	if statusCode >= 500 {
		return tokenExchangeProvider5xx, ""
	}
	if statusCode >= 400 {
		var parsed struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &parsed)
		switch parsed.Error {
		case "invalid_grant":
			return tokenExchangeInvalidGrant, parsed.Error
		case "invalid_client":
			return tokenExchangeInvalidClient, parsed.Error
		case "invalid_request":
			return tokenExchangeInvalidRequest, parsed.Error
		case "unauthorized_client":
			return tokenExchangeUnauthorizedClient, parsed.Error
		case "unsupported_grant_type":
			return tokenExchangeUnsupportedGrantType, parsed.Error
		case "invalid_scope":
			return tokenExchangeInvalidScope, parsed.Error
		}
		return tokenExchangeProvider4xxOther, parsed.Error
	}
	return tokenExchangeProvider4xxOther, ""
}

// tokenExchangeAttrsFromConn fills the connection/state/actor identifiers
// that are common to every token-exchange failure emitted from the OAuth2
// callback path. Callers add category-specific fields (ProviderStatusCode,
// ProviderError, Err) on top of the returned struct.
func (o *oAuth2Connection) tokenExchangeAttrsFromConn(err error) tokenExchangeAttrs {
	attrs := tokenExchangeAttrs{Err: err}
	if o.connection != nil {
		attrs.ConnectionId = o.connection.GetId()
	}
	if o.state != nil {
		attrs.StateId = o.state.Id
		attrs.ActorId = o.state.ActorId
		attrs.Namespace = o.state.Namespace
	}
	return attrs
}

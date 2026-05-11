package oauth2

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apid"
)

// tokenRefreshCategory identifies the class of failure observed during an
// OAuth2 refresh-token grant. Like tokenExchangeCategory (used for the
// initial code → token exchange leg), the value is the primary axis for
// downstream alerting and dashboards — existing values must remain stable.
//
// Permanent vs transient is implicit in the category: provider_5xx and
// network_error are transient (a retry on the next proxy call may
// succeed); invalid_grant / invalid_client / no_refresh_token /
// malformed_response are permanent (the user must re-authenticate).
type tokenRefreshCategory string

const (
	// tokenRefreshNoRefreshToken — the persisted token row had no refresh
	// token (provider issued an access token only). When the access token
	// then expires, there is no way to obtain a new one without user
	// interaction. Permanent.
	tokenRefreshNoRefreshToken tokenRefreshCategory = "no_refresh_token"
	// tokenRefreshNetworkError — POST to the token endpoint failed at the
	// transport layer (DNS, dial, TLS, read timeout). Transient.
	tokenRefreshNetworkError tokenRefreshCategory = "network_error"
	// tokenRefreshInvalidGrant — provider returned a 4xx with
	// `error=invalid_grant`. Refresh token is revoked, expired, or has
	// been used (rotation enforcement). Permanent — the user must
	// re-authenticate.
	tokenRefreshInvalidGrant tokenRefreshCategory = "invalid_grant"
	// tokenRefreshInvalidClient — provider returned `error=invalid_client`.
	// Misconfigured client_id or client_secret. Permanent from a runtime
	// perspective — refreshes will keep failing until the config is fixed,
	// but flipping the connection unhealthy still produces the right
	// operator signal.
	tokenRefreshInvalidClient tokenRefreshCategory = "invalid_client"
	// tokenRefreshProvider4xxOther — non-200 4xx that did not include a
	// recognized RFC 6749 §5.2 error code. Permanent (treated like
	// invalid_grant for the unhealthy-flip purposes — a retry won't fix
	// it without user action).
	tokenRefreshProvider4xxOther tokenRefreshCategory = "provider_4xx_other"
	// tokenRefreshProvider5xx — provider returned 500/502/503/504.
	// Transient — does not flip the connection unhealthy on its own.
	tokenRefreshProvider5xx tokenRefreshCategory = "provider_5xx"
	// tokenRefreshMalformedResponse — token endpoint returned 200 but the
	// body could not be parsed or had no access_token. Permanent from a
	// recovery standpoint — retrying produces the same garbage.
	tokenRefreshMalformedResponse tokenRefreshCategory = "malformed_response"
	// tokenRefreshInternalError — proxy-side error (decryption, config
	// rendering, credential fetch). Not attributable to the provider.
	// Transient; does not flip unhealthy.
	tokenRefreshInternalError tokenRefreshCategory = "internal_error"
)

// IsPermanent reports whether a refresh failure of this category should flip
// the connection's health_state to unhealthy. Transient categories (5xx,
// network, internal) do not — the next proxy call gets another chance.
func (c tokenRefreshCategory) IsPermanent() bool {
	switch c {
	case tokenRefreshNoRefreshToken,
		tokenRefreshInvalidGrant,
		tokenRefreshInvalidClient,
		tokenRefreshProvider4xxOther,
		tokenRefreshMalformedResponse:
		return true
	}
	return false
}

// tokenRefreshAttrs carries the safe-to-log identifiers for a refresh
// failure or success event. ProviderError and ProviderStatusCode follow the
// same safety reasoning as tokenExchangeAttrs — RFC 6749 §5.2 defines
// `error` as an enumerated, non-secret status code.
type tokenRefreshAttrs struct {
	ConnectionId       apid.ID
	Namespace          string
	ProviderStatusCode int
	ProviderError      string
	Err                error
}

// tokenRefreshFailureMessage / tokenRefreshSuccessMessage are the single
// message strings for the corresponding structured events. Operators filter
// on these exact strings — keep stable.
const (
	tokenRefreshFailureMessage = "oauth token refresh failed"
	tokenRefreshSuccessMessage = "oauth token refresh succeeded"
)

// emitTokenRefreshFailure logs a structured "oauth token refresh failed"
// event at Warn. Only populated attrs are emitted.
func emitTokenRefreshFailure(ctx context.Context, logger *slog.Logger, category tokenRefreshCategory, attrs tokenRefreshAttrs) {
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
	if attrs.ProviderStatusCode != 0 {
		args = append(args, slog.Int("provider_status_code", attrs.ProviderStatusCode))
	}
	if attrs.ProviderError != "" {
		args = append(args, slog.String("provider_error", attrs.ProviderError))
	}
	if attrs.Err != nil {
		args = append(args, slog.String("error", attrs.Err.Error()))
	}

	b.Build().WarnContext(ctx, tokenRefreshFailureMessage, args...)
}

// emitTokenRefreshSucceeded logs a structured "oauth token refresh
// succeeded" event at Info. Dashboards correlate this with prior failures
// to detect recovery without parsing log lines.
func emitTokenRefreshSucceeded(ctx context.Context, logger *slog.Logger, attrs tokenRefreshAttrs) {
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

	b.Build().InfoContext(ctx, tokenRefreshSuccessMessage)
}

// classifyTokenRefreshStatus inspects a non-2xx refresh response and
// returns (category, providerErrorCode). Shares the RFC 6749 §5.2 body
// parsing with classifyTokenEndpointStatus but only recognizes the error
// codes that are meaningful for refresh — invalid_request and friends
// can't occur on a well-formed refresh POST that the proxy itself built.
func classifyTokenRefreshStatus(statusCode int, body []byte) (tokenRefreshCategory, string) {
	if statusCode >= 500 {
		return tokenRefreshProvider5xx, ""
	}
	if statusCode >= 400 {
		var parsed struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &parsed)
		switch parsed.Error {
		case "invalid_grant":
			return tokenRefreshInvalidGrant, parsed.Error
		case "invalid_client":
			return tokenRefreshInvalidClient, parsed.Error
		}
		return tokenRefreshProvider4xxOther, parsed.Error
	}
	return tokenRefreshProvider4xxOther, ""
}

// tokenRefreshAttrsFromConn fills the connection/namespace identifiers
// that are common to every refresh event emitted from the proxy/refresh
// paths.
func (o *oAuth2Connection) tokenRefreshAttrsFromConn(err error) tokenRefreshAttrs {
	attrs := tokenRefreshAttrs{Err: err}
	if o.connection != nil {
		attrs.ConnectionId = o.connection.GetId()
		attrs.Namespace = o.connection.GetNamespace()
	}
	return attrs
}

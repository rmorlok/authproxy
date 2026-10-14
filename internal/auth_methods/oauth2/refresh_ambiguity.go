package oauth2

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/internal/apid"
)

// An "ambiguous" refresh attempt is one that may have reached the provider
// without AuthProxy learning the outcome: the connection dropped or timed
// out after the request was sent, or a gateway in front of the provider
// answered 5xx. If the provider did process it, the refresh token we sent
// has been consumed — for providers that rotate refresh tokens it is now
// invalid — and the replacement it issued is gone. Retrying (immediately or
// on a later refresh) then fails with invalid_grant.
//
// That invalid_grant looks identical to a user revoking access, but the
// operator response is different (it points at network/provider trouble, not
// user action). refreshAccessTokenLocked therefore reports it as
// tokenRefreshInvalidGrantAfterAmbiguousAttempt when it follows an ambiguous
// attempt, either earlier in the same refresh or recorded in Redis by a
// previous refresh against the same token row.

// ambiguousRefreshMarkerTTL is how long a recorded ambiguous attempt can
// upgrade a later invalid_grant. Long enough to span asynq retries and the
// next background refresh cycle; short enough that a genuine revocation weeks
// later is not misattributed.
const ambiguousRefreshMarkerTTL = 24 * time.Hour

// isAmbiguousTransportError reports whether a transport-level failure may
// have happened after the request reached the provider. Failures that
// provably occur before anything is sent — DNS resolution, TCP connect, TLS
// handshake / certificate verification — are not ambiguous; everything else
// (resets, read timeouts, context deadline mid-request) is.
func isAmbiguousTransportError(err error) bool {
	if err == nil {
		return false
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return false
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return false
	}

	var (
		certVerifyErr  *tls.CertificateVerificationError
		recordErr      tls.RecordHeaderError
		alertErr       tls.AlertError
		unknownAuthErr x509.UnknownAuthorityError
		hostnameErr    x509.HostnameError
		certInvalidErr x509.CertificateInvalidError
	)
	if errors.As(err, &certVerifyErr) || errors.As(err, &recordErr) || errors.As(err, &alertErr) ||
		errors.As(err, &unknownAuthErr) || errors.As(err, &hostnameErr) || errors.As(err, &certInvalidErr) {
		return false
	}

	return true
}

// isAmbiguousRefreshOutcome reports whether a single token-endpoint attempt
// (response or transport error) left the provider-side outcome unknown.
func isAmbiguousRefreshOutcome(statusCode int, err error) bool {
	if err != nil {
		return isAmbiguousTransportError(err)
	}
	return statusCode >= 500
}

// ambiguousMarkerTimeout bounds each best-effort Redis call on the marker.
const ambiguousMarkerTimeout = 2 * time.Second

// detachedMarkerCtx returns a short-lived context for a marker read/write
// that survives cancellation of ctx. A refresh cut short by its caller's
// deadline is precisely the ambiguous case worth recording.
func detachedMarkerCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), ambiguousMarkerTimeout)
}

// ambiguousRefreshMarkerKey is the Redis key recording that a refresh of the
// connection's current token ended ambiguously.
func (o *oAuth2Connection) ambiguousRefreshMarkerKey() string {
	return "oauth2-refresh-ambiguous-" + o.connection.GetId().String()
}

// recordAmbiguousRefresh notes that a refresh redeeming tokenId ended without
// a definitive answer. Best effort: a Redis failure only loses diagnostic
// precision, so it is logged and otherwise ignored.
func (o *oAuth2Connection) recordAmbiguousRefresh(ctx context.Context, tokenId apid.ID) {
	if o.r == nil {
		return
	}
	ctx, cancel := detachedMarkerCtx(ctx)
	defer cancel()
	if err := o.r.Set(ctx, o.ambiguousRefreshMarkerKey(), tokenId.String(), ambiguousRefreshMarkerTTL).Err(); err != nil {
		o.logger.WarnContext(ctx, "failed to record ambiguous oauth token refresh", "error", err)
	}
}

// hadAmbiguousRefresh reports whether a previous refresh that redeemed
// tokenId ended ambiguously (and the marker has not expired). Lookup errors
// are treated as "no".
func (o *oAuth2Connection) hadAmbiguousRefresh(ctx context.Context, tokenId apid.ID) bool {
	if o.r == nil {
		return false
	}
	ctx, cancel := detachedMarkerCtx(ctx)
	defer cancel()
	val, err := o.r.Get(ctx, o.ambiguousRefreshMarkerKey()).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			o.logger.WarnContext(ctx, "failed to read ambiguous oauth token refresh marker", "error", err)
		}
		return false
	}
	return val == tokenId.String()
}

// clearAmbiguousRefresh removes the marker after a refresh reached a
// definitive outcome. Best effort.
func (o *oAuth2Connection) clearAmbiguousRefresh(ctx context.Context) {
	if o.r == nil {
		return
	}
	ctx, cancel := detachedMarkerCtx(ctx)
	defer cancel()
	if err := o.r.Del(ctx, o.ambiguousRefreshMarkerKey()).Err(); err != nil {
		o.logger.WarnContext(ctx, "failed to clear ambiguous oauth token refresh marker", "error", err)
	}
}

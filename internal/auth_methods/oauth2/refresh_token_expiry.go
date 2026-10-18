package oauth2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util"
)

// Refresh-token lifetimes.
//
// RFC 6749 §5.1 defines expires_in for the access token only; nothing in the
// core spec tells a client when the refresh token stops working. Several
// providers and drafts fill that gap with extra token-response fields, all
// expressed in seconds from the time of the response:
//
//   - refresh_token_expires_in — GitHub Apps user tokens (rotating, always
//     15897600 = 6 months; https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/refreshing-user-access-tokens),
//     LinkedIn programmatic refresh tokens (fixed 1-year TTL counted down,
//     not extended by refreshing; https://learn.microsoft.com/en-us/linkedin/shared/authentication/programmatic-refresh-tokens),
//     Google when the user grants time-based access
//     (https://developers.google.com/identity/protocols/oauth2/web-server),
//     and draft-watson-oauth-refresh-token-expiration-00.
//   - x_refresh_token_expires_in — Intuit QuickBooks Online (~100 days,
//     rotating; https://help.developer.intuit.com/s/article/Validity-of-Refresh-Token).
//   - refresh_token_timeout — the OAuth WG draft
//     draft-ietf-oauth-refresh-token-expiration (-03): the maximum time the
//     client may hold the refresh token without exchanging it
//     (https://datatracker.ietf.org/doc/draft-ietf-oauth-refresh-token-expiration/).
//
// All three answer "refresh before this or lose the connection", so the
// earliest one present wins. The draft's authorization_expires_in (lifetime
// of the user's consent) is deliberately ignored: refreshing cannot extend
// it, so it does not affect when to refresh.

// refreshTokenExtensionTolerance is how much later a refreshed token's
// expiry must be than its predecessor's to count as "refreshing extended the
// refresh token". Countdown-style providers (LinkedIn) report the same
// absolute expiry give or take rounding of the seconds field and request
// latency.
const refreshTokenExtensionTolerance = time.Minute

// expiresInSeconds is a lifetime in seconds from a token response. It accepts
// JSON numbers and numeric strings, since providers are inconsistent about
// quoting these non-standard fields.
type expiresInSeconds struct {
	// Seconds is the parsed value.
	Seconds int64
	// Present is true when the field was present and parseable.
	Present bool
}

// UnmarshalJSON implements json.Unmarshaler for expiresInSeconds. Null and
// empty strings leave the value absent; anything else non-numeric is an
// error (and makes the whole token response malformed).
func (e *expiresInSeconds) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		return nil
	}

	var raw string
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &raw); err != nil {
			return err
		}
		if raw == "" {
			return nil
		}
	} else {
		raw = string(b)
	}

	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fmt.Errorf("invalid lifetime %q: %w", raw, err)
	}
	e.Seconds = int64(f)
	e.Present = true
	return nil
}

// refreshTokenLifetime returns the shortest positive refresh-token lifetime
// reported by the response, and whether any was reported. Non-positive
// values are treated as absent.
func (r *tokenResponse) refreshTokenLifetime() (time.Duration, bool) {
	var (
		shortest time.Duration
		found    bool
	)
	for _, v := range []expiresInSeconds{
		r.RefreshTokenExpiresIn,
		r.XRefreshTokenExpiresIn,
		r.RefreshTokenTimeout,
	} {
		if !v.Present || v.Seconds <= 0 {
			continue
		}

		d := time.Duration(v.Seconds) * time.Second

		if !found || d < shortest {
			shortest, found = d, true
		}
	}

	return shortest, found
}

// refreshTokenExpiry decides what to persist about the refresh token's
// lifetime for a token response received at now.
//
//   - ExpiresAt comes from the response; if the response is silent and the
//     previous refresh token is being carried forward, its known expiry is
//     carried forward too.
//   - RefreshAt is when the background task should refresh proactively:
//     the configured lead time (capped at half the remaining lifetime)
//     before ExpiresAt. It is left nil when background refresh is disabled,
//     the token has already expired, or this response came from a refresh
//     that did not push the expiry later — repeating such a refresh cannot
//     help (LinkedIn-style fixed TTLs), and leaving it set would make the
//     background task refresh the connection on every run.
//
// persistingNewRefreshToken reports whether the response's refresh token is
// being stored; refreshFrom is the token being refreshed (nil on initial
// exchange). Grants without refresh tokens (client_credentials) get the zero
// value.
func (o *oAuth2Connection) refreshTokenExpiry(
	now time.Time,
	resp *tokenResponse,
	opts tokenPersistOptions,
	persistingNewRefreshToken bool,
	refreshFrom *database.OAuth2Token,
) database.OAuth2RefreshTokenExpiry {
	if !opts.PersistRefreshToken {
		return database.OAuth2RefreshTokenExpiry{}
	}

	var expiresAt *time.Time
	if lifetime, ok := resp.refreshTokenLifetime(); ok {
		expiresAt = util.ToPtr(now.Add(lifetime))
	} else if !persistingNewRefreshToken && refreshFrom != nil &&
		refreshFrom.RefreshTokenExpiresAt != nil {
		expiresAt = util.ToPtr(*refreshFrom.RefreshTokenExpiresAt)
	}

	if expiresAt == nil {
		return database.OAuth2RefreshTokenExpiry{}
	}

	result := database.OAuth2RefreshTokenExpiry{
		ExpiresAt: expiresAt,
	}

	if !o.auth.Token.GetRefreshInBackgroundOrDefault() {
		return result
	}

	remaining := expiresAt.Sub(now)
	if remaining <= 0 {
		return result
	}

	if refreshFrom != nil && refreshFrom.RefreshTokenExpiresAt != nil &&
		!expiresAt.After(refreshFrom.RefreshTokenExpiresAt.Add(refreshTokenExtensionTolerance)) {
		return result
	}

	lead := o.auth.Token.GetRefreshTimeBeforeRefreshTokenExpiryOrDefault()
	if half := remaining / 2; lead > half {
		lead = half
	}

	result.RefreshAt = util.ToPtr(expiresAt.Add(-lead))

	return result
}

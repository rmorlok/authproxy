package key

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/rmorlok/authproxy/internal/util/retry"
)

const (
	vaultRetryInitialBackoff = 2 * time.Second
	vaultRetryMaxBackoff     = 60 * time.Second
	vaultRetryMultiplier     = 2.0
	vaultRetryMaxAttempts    = 5
)

// vaultRetryTransport wraps an http.RoundTripper to retry on 429 responses.
// If a Retry-After header is present (numeric seconds), it overrides the
// backoff; otherwise the exponential strategy is used.
//
// Transport errors are NOT retried — that mirrors the pre-consolidation
// behavior and matches Vault's official client conventions (the SDK
// classifies network failures as caller-handled).
type vaultRetryTransport struct {
	base http.RoundTripper
}

func (t *vaultRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := retry.Do(req.Context(), retry.Options[*http.Response]{
		MaxAttempts: vaultRetryMaxAttempts,
		Backoff:     newVaultBackoff(),
		Classify: func(resp *http.Response, err error) bool {
			// Only retry 429s; transport errors fall through unchanged.
			return err == nil && resp != nil && resp.StatusCode == http.StatusTooManyRequests
		},
		OnRetry: func(_ int, resp *http.Response, _ error) {
			// Drain + close so the connection can return to the pool
			// before the next attempt opens a fresh one. The body of
			// the final returned response is left intact for the caller.
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
		},
		OnRetryWait: parseVaultRetryAfter,
	}, func(_ context.Context) (*http.Response, error) {
		return t.base.RoundTrip(req)
	})

	if err != nil {
		return nil, err
	}
	return res.Value, nil
}

// newVaultBackoff returns the exponential strategy used between attempts
// when no Retry-After header is present. Parameters match the
// pre-consolidation transport: 2s initial, double each step, capped at 60s.
// The randomization factor (0.5) gives ~[interval/2, interval*1.5] jitter —
// slightly wider than the prior hand-rolled "[backoff/2, backoff)" range, but
// the overall back-off shape is preserved and there are no tests that depend
// on the exact distribution.
func newVaultBackoff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = vaultRetryInitialBackoff
	b.MaxInterval = vaultRetryMaxBackoff
	b.Multiplier = vaultRetryMultiplier
	b.RandomizationFactor = 0.5
	return b
}

// parseVaultRetryAfter returns the Retry-After header's duration if the
// header is present and parses as a positive numeric-seconds value (rounded
// up to the nearest second, matching Vault v1.20.0 behavior). Returns 0
// otherwise so retry.Do falls through to the backoff strategy.
func parseVaultRetryAfter(resp *http.Response, _ error) time.Duration {
	if resp == nil {
		return 0
	}
	ra := resp.Header.Get("Retry-After")
	if ra == "" {
		return 0
	}
	seconds, err := strconv.ParseFloat(ra, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(math.Ceil(seconds)) * time.Second
}

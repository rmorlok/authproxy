package config

import (
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

const (
	vaultRetryInitialBackoff = 2 * time.Second
	vaultRetryMaxBackoff     = 60 * time.Second
	vaultRetryMultiplier     = 2.0
	vaultRetryMaxAttempts    = 5
)

// vaultRetryTransport wraps an http.RoundTripper to retry on 429 responses.
// If a Retry-After header is present, it uses that duration; otherwise it
// uses exponential backoff with jitter.
type vaultRetryTransport struct {
	base http.RoundTripper
}

func (t *vaultRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	backoff := vaultRetryInitialBackoff

	for attempt := 0; ; attempt++ {
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusTooManyRequests || attempt >= vaultRetryMaxAttempts-1 {
			return resp, nil
		}

		// Determine how long to wait before retrying.
		wait := t.retryAfterDuration(resp, backoff)

		// Drain and close the body so the connection can be reused.
		resp.Body.Close()

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(wait):
		}

		// Advance exponential backoff for next attempt.
		backoff = time.Duration(float64(backoff) * vaultRetryMultiplier)
		if backoff > vaultRetryMaxBackoff {
			backoff = vaultRetryMaxBackoff
		}
	}
}

// retryAfterDuration parses the Retry-After header if present, otherwise
// returns the given backoff duration with jitter.
func (t *vaultRetryTransport) retryAfterDuration(resp *http.Response, backoff time.Duration) time.Duration {
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if seconds, err := strconv.ParseFloat(ra, 64); err == nil && seconds > 0 {
			// Round up to nearest second, matching Vault v1.20.0 behavior.
			return time.Duration(math.Ceil(seconds)) * time.Second
		}
	}

	// Exponential backoff with jitter: [backoff/2, backoff)
	jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
	return backoff/2 + jitter
}

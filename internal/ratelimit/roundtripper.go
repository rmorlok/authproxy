package ratelimit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
)

// Factory implements httpf.RoundTripperFactory to create rate limiting roundtrippers.
type Factory struct {
	store  *Store
	logger *slog.Logger
}

// NewFactory creates a new rate limiting middleware factory.
func NewFactory(store *Store, logger *slog.Logger) *Factory {
	return &Factory{
		store:  store,
		logger: logger,
	}
}

func (f *Factory) NewRoundTripper(ri httpf.RequestInfo, transport http.RoundTripper) http.RoundTripper {
	// Only apply rate limiting for proxy and probe requests with a connection context
	if ri.ConnectionId == apid.Nil {
		return nil
	}
	if ri.Type != httpf.RequestTypeProxy && ri.Type != httpf.RequestTypeProbe {
		return nil
	}

	// Check if rate limiting is disabled
	if ri.RateLimiting != nil && ri.RateLimiting.Disabled {
		return nil
	}

	return &RoundTripper{
		connectionId: ri.ConnectionId,
		config:       ri.RateLimiting, // nil means use defaults
		store:        f.store,
		transport:    transport,
		logger:       f.logger,
	}
}

// RoundTripper is an http.RoundTripper that enforces 429-based rate limiting per connection.
type RoundTripper struct {
	connectionId apid.ID
	config       *connectors.RateLimiting // nil means use defaults
	store        *Store
	transport    http.RoundTripper
	logger       *slog.Logger
}

func (rt *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	// Check if this connection is currently rate-limited
	remaining, limited, err := rt.store.IsRateLimited(ctx, rt.connectionId)
	if err != nil {
		rt.logger.WarnContext(ctx, "failed to check rate limit state",
			slog.String("connection_id", rt.connectionId.String()),
			slog.String("error", err.Error()),
		)
		// On Redis errors, allow the request through rather than blocking
	} else if limited {
		rt.logger.InfoContext(ctx, "request blocked by rate limiter",
			slog.String("connection_id", rt.connectionId.String()),
			slog.Duration("retry_after", remaining),
		)
		return rt.syntheticTooManyRequests(remaining), nil
	}

	// Execute the actual request
	resp, err := rt.transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// Handle the response
	if resp.StatusCode == http.StatusTooManyRequests {
		rt.handle429(ctx, resp)
	} else {
		// Clear consecutive 429 counter on any non-429 response
		if clearErr := rt.store.ClearConsecutive429Count(ctx, rt.connectionId); clearErr != nil {
			rt.logger.WarnContext(ctx, "failed to clear consecutive 429 count",
				slog.String("connection_id", rt.connectionId.String()),
				slog.String("error", clearErr.Error()),
			)
		}
	}

	return resp, nil
}

func (rt *RoundTripper) handle429(ctx context.Context, resp *http.Response) {
	now := apctx.GetClock(ctx).Now()
	headerNames := rt.getConfig().GetRetryAfterHeaders()

	retryAfter, found := ParseRetryAfter(resp.Header, headerNames, now)

	if !found {
		// No valid retry-after header; use backoff strategy
		retryAfter = rt.computeBackoff(ctx)
	}

	// Cap at max retry-after
	maxRetryAfter := rt.getConfig().GetMaxRetryAfter()
	if retryAfter > maxRetryAfter {
		retryAfter = maxRetryAfter
	}

	// Ensure minimum 1 second
	if retryAfter < 1*time.Second {
		retryAfter = 1 * time.Second
	}

	// Increment consecutive 429 count
	count, err := rt.store.IncrementConsecutive429Count(ctx, rt.connectionId)
	if err != nil {
		rt.logger.WarnContext(ctx, "failed to increment consecutive 429 count",
			slog.String("connection_id", rt.connectionId.String()),
			slog.String("error", err.Error()),
		)
	}

	// Store the rate limit
	if err := rt.store.SetRateLimited(ctx, rt.connectionId, retryAfter); err != nil {
		rt.logger.WarnContext(ctx, "failed to set rate limit",
			slog.String("connection_id", rt.connectionId.String()),
			slog.String("error", err.Error()),
		)
	}

	rt.logger.InfoContext(ctx, "429 received, connection rate-limited",
		slog.String("connection_id", rt.connectionId.String()),
		slog.Duration("retry_after", retryAfter),
		slog.Bool("header_found", found),
		slog.Int("consecutive_429s", count),
	)
}

func (rt *RoundTripper) computeBackoff(ctx context.Context) time.Duration {
	cfg := rt.getConfig()

	if cfg == nil || cfg.ExponentialBackoff == nil {
		return cfg.GetDefaultRetryAfter()
	}

	eb := cfg.ExponentialBackoff

	// Get the current consecutive count (before increment)
	count, err := rt.store.GetConsecutive429Count(ctx, rt.connectionId)
	if err != nil {
		rt.logger.WarnContext(ctx, "failed to get consecutive 429 count for backoff",
			slog.String("connection_id", rt.connectionId.String()),
			slog.String("error", err.Error()),
		)
		return cfg.GetDefaultRetryAfter()
	}

	// Compute: initial * multiplier^count
	initial := eb.GetInitialInterval()
	multiplier := eb.GetMultiplier()
	maxInterval := eb.GetMaxInterval()

	backoff := float64(initial) * math.Pow(multiplier, float64(count))

	// Apply jitter
	jitter := eb.GetJitterFraction()
	if jitter > 0 {
		jitterRange := backoff * jitter
		backoff = backoff - jitterRange + rand.Float64()*2*jitterRange
	}

	d := time.Duration(backoff)
	if d > maxInterval {
		d = maxInterval
	}

	return d
}

func (rt *RoundTripper) getConfig() *connectors.RateLimiting {
	if rt.config != nil {
		return rt.config
	}
	// Return a nil config which will cause the getter methods to return defaults
	return nil
}

func (rt *RoundTripper) syntheticTooManyRequests(retryAfter time.Duration) *http.Response {
	retryAfterSeconds := int(math.Ceil(retryAfter.Seconds()))
	body := fmt.Sprintf(`{"error":"rate limited","retry_after_seconds":%d}`, retryAfterSeconds)

	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"Content-Type":            {"application/json"},
			"Retry-After":             {strconv.Itoa(retryAfterSeconds)},
			"X-Authproxy-Ratelimited": {"true"},
		},
		Body:          io.NopCloser(bytes.NewBufferString(body)),
		ContentLength: int64(len(body)),
	}
}

var _ httpf.RoundTripperFactory = (*Factory)(nil)

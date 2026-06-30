package key

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tclock "k8s.io/utils/clock/testing"
)

// scriptedRoundTripper returns a sequence of canned responses (or errors)
// across successive RoundTrip calls. attempts atomically tracks how many
// times RoundTrip was invoked.
type scriptedRoundTripper struct {
	attempts atomic.Int32
	steps    []rtStep
}

type rtStep struct {
	resp *http.Response
	err  error
}

func (rt *scriptedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	idx := int(rt.attempts.Add(1)) - 1
	if idx >= len(rt.steps) {
		idx = len(rt.steps) - 1
	}
	s := rt.steps[idx]
	if s.resp != nil && s.resp.Body == nil {
		s.resp.Body = http.NoBody
	}
	return s.resp, s.err
}

func newResp(status int, retryAfter string) *http.Response {
	h := http.Header{}
	if retryAfter != "" {
		h.Set("Retry-After", retryAfter)
	}
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       http.NoBody,
	}
}

// fakeClkCtx returns a context whose clock is a FakeClock plus a goroutine
// that steps it whenever the retry helper subscribes. Returns a stop func
// to be called via defer.
func fakeClkCtx(t *testing.T, parent context.Context) (context.Context, func()) {
	t.Helper()
	fakeClk := tclock.NewFakeClock(time.Now())
	stepperCtx, stop := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-stepperCtx.Done():
				return
			default:
			}
			if fakeClk.HasWaiters() {
				fakeClk.Step(time.Minute)
			}
			time.Sleep(time.Millisecond)
		}
	}()
	return apctx.WithClock(parent, fakeClk), stop
}

func TestVaultRetryTransport_429ThenSuccess(t *testing.T) {
	ctx, stop := fakeClkCtx(t, context.Background())
	defer stop()

	rt := &scriptedRoundTripper{steps: []rtStep{
		{resp: newResp(http.StatusTooManyRequests, "")},
		{resp: newResp(http.StatusTooManyRequests, "")},
		{resp: newResp(http.StatusOK, "")},
	}}
	transport := &vaultRetryTransport{base: rt}

	req, err := http.NewRequestWithContext(ctx, "GET", "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(3), rt.attempts.Load())
}

func TestVaultRetryTransport_429ExhaustedReturnsLastResp(t *testing.T) {
	ctx, stop := fakeClkCtx(t, context.Background())
	defer stop()

	steps := make([]rtStep, vaultRetryMaxAttempts)
	for i := range steps {
		steps[i] = rtStep{resp: newResp(http.StatusTooManyRequests, "")}
	}
	rt := &scriptedRoundTripper{steps: steps}
	transport := &vaultRetryTransport{base: rt}

	req, err := http.NewRequestWithContext(ctx, "GET", "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err, "exhaustion returns the final 429 with nil err — caller decides")
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, int32(vaultRetryMaxAttempts), rt.attempts.Load())
}

func TestVaultRetryTransport_TransportErrorNotRetried(t *testing.T) {
	ctx, stop := fakeClkCtx(t, context.Background())
	defer stop()

	wantErr := errors.New("dial tcp: connection refused")
	rt := &scriptedRoundTripper{steps: []rtStep{{err: wantErr}}}
	transport := &vaultRetryTransport{base: rt}

	req, err := http.NewRequestWithContext(ctx, "GET", "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.ErrorIs(t, err, wantErr, "transport errors must not be retried — they short-circuit")
	assert.Nil(t, resp)
	assert.Equal(t, int32(1), rt.attempts.Load())
}

func TestVaultRetryTransport_Non429StatusReturnedImmediately(t *testing.T) {
	ctx, stop := fakeClkCtx(t, context.Background())
	defer stop()

	rt := &scriptedRoundTripper{steps: []rtStep{
		{resp: newResp(http.StatusInternalServerError, "")},
	}}
	transport := &vaultRetryTransport{base: rt}

	req, err := http.NewRequestWithContext(ctx, "GET", "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, int32(1), rt.attempts.Load(), "non-429 statuses are not retried — even 5xx")
}

func TestVaultRetryTransport_RetryAfterHonored(t *testing.T) {
	// Drive the retry helper with a real clock and a Retry-After of 0.1s.
	// We can't easily assert "waited exactly 100ms" without flakiness, but
	// we can assert the request is retried (i.e. parseVaultRetryAfter
	// produced a positive duration that the helper honored).
	rt := &scriptedRoundTripper{steps: []rtStep{
		{resp: newResp(http.StatusTooManyRequests, "0.1")},
		{resp: newResp(http.StatusOK, "")},
	}}
	transport := &vaultRetryTransport{base: rt}

	req, err := http.NewRequestWithContext(context.Background(), "GET", "http://example.invalid", nil)
	require.NoError(t, err)

	start := time.Now()
	resp, err := transport.RoundTrip(req)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(2), rt.attempts.Load())
	// parseVaultRetryAfter ceils to nearest second, so 0.1 → 1s wait.
	assert.GreaterOrEqual(t, elapsed, 500*time.Millisecond,
		"Retry-After ceils to 1s, so the helper should have waited at least ~1s (allow margin)")
}

func TestVaultRetryTransport_CtxCancelDuringBackoff(t *testing.T) {
	// Don't install a clock stepper — backoff sleeps on a real clock and
	// gets cancelled by ctx.
	steps := make([]rtStep, vaultRetryMaxAttempts)
	for i := range steps {
		steps[i] = rtStep{resp: newResp(http.StatusTooManyRequests, "")}
	}
	rt := &scriptedRoundTripper{steps: steps}
	transport := &vaultRetryTransport{base: rt}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Wait for one attempt then cancel.
		for rt.attempts.Load() < 1 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	req, err := http.NewRequestWithContext(ctx, "GET", "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context"),
		"expected ctx cancel to surface; got %v", err)
	assert.Nil(t, resp)
}

func TestParseVaultRetryAfter(t *testing.T) {
	cases := []struct {
		name string
		resp *http.Response
		want time.Duration
	}{
		{"nil response", nil, 0},
		{"no header", newResp(429, ""), 0},
		{"valid integer", newResp(429, "5"), 5 * time.Second},
		{"valid fractional rounds up", newResp(429, "0.1"), 1 * time.Second},
		{"valid fractional rounds up (2.3)", newResp(429, "2.3"), 3 * time.Second},
		{"zero is rejected", newResp(429, "0"), 0},
		{"negative is rejected", newResp(429, "-1"), 0},
		{"unparseable is rejected", newResp(429, "Wed, 21 Oct 2015 07:28:00 GMT"), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseVaultRetryAfter(tc.resp, nil))
		})
	}
}

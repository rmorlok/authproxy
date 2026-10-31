package httpexec

import (
	"math"
	"testing"
	"time"

	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/tools/validation"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// policyLimits uses distinct component ceilings to detect accidentally raising
// a stricter component or changing a limit unrelated to the authored cap.
func policyLimits() Limits {
	return Limits{
		Render: render.Limits{MaxBytes: 32768, MaxDepth: 32, MaxSteps: 10000},
		Validation: validation.Limits{
			MaxInputBytes: 4096, MaxOutputBytes: 1024,
			MaxDepth: 32, MaxValues: 10000, MaxNumberDigits: 128,
		},
		Response: httpcodec.ResponseLimits{
			MaxSourceBytes: 4096, MaxResponseBytes: 1024,
			MaxOutputBytes: 2048, MaxEncodedResultBytes: 16384,
			Transform: javascript.Limits{
				MaxSourceBytes: 4096, MaxInputBytes: 32768, MaxOutputBytes: 256,
				MaxDepth: 32, MaxNodes: 10000, MaxStackDepth: 128, Timeout: time.Second,
			},
		},
		HTTP: httphost.Limits{
			Timeout: 2 * time.Second, MaxRequests: 20, MaxResponseBytes: 4096,
			MaxStreamBytes: 8192, MaxHeaderBytes: 4096,
		},
	}
}

// TestResolveLimitsAppliesIndependentCaps checks every authored field against
// the complete policy so argument caps cannot also restrict configuration,
// source, response-transform input, or unrelated technical bounds.
func TestResolveLimitsAppliesIndependentCaps(t *testing.T) {
	for _, test := range []struct {
		name string
		caps toolschema.ExecutionLimits
		want func(*Limits)
	}{
		{"timeout", toolschema.ExecutionLimits{TimeoutMillis: util.ToPtr(int64(500))}, func(l *Limits) {
			l.HTTP.Timeout, l.Response.Transform.Timeout = 500*time.Millisecond, 500*time.Millisecond
		}},
		{"requests", toolschema.ExecutionLimits{MaxRequests: util.ToPtr(2)}, func(l *Limits) {
			l.HTTP.MaxRequests = 2
		}},
		{"arguments", toolschema.ExecutionLimits{MaxInputBytes: util.ToPtr(int64(100))}, func(l *Limits) {
			l.Validation.MaxInputBytes = 100
		}},
		{"output", toolschema.ExecutionLimits{MaxOutputBytes: util.ToPtr(int64(512))}, func(l *Limits) {
			l.Validation.MaxOutputBytes, l.Response.MaxOutputBytes = 512, 512
		}},
		{"responses", toolschema.ExecutionLimits{MaxResponseBytes: util.ToPtr(int64(2048))}, func(l *Limits) {
			l.HTTP.MaxResponseBytes = 2048
		}},
		{"envelope", toolschema.ExecutionLimits{MaxEncodedResultBytes: util.ToPtr(int64(1024))}, func(l *Limits) {
			l.Response.MaxEncodedResultBytes = 1024
		}},
		{"stack", toolschema.ExecutionLimits{MaxStackDepth: util.ToPtr(16)}, func(l *Limits) {
			l.Response.Transform.MaxStackDepth = 16
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Resolve one cap and compare the whole result, including untouched fields.
			policy := policyLimits()
			want := policy
			test.want(&want)
			originalCaps := test.caps.Clone()
			got, err := resolveLimits(policy, &test.caps)
			require.NoError(t, err)
			require.Equal(t, want, got)
			require.Equal(t, policyLimits(), policy)
			require.Equal(t, originalCaps, &test.caps)
		})
	}
}

// TestResolveLimitsPreservesOperatorCeilings covers omissions and values that
// exceed runtime representations, including an int64 millisecond overflow.
func TestResolveLimitsPreservesOperatorCeilings(t *testing.T) {
	for _, caps := range []*toolschema.ExecutionLimits{
		nil, {},
		{
			TimeoutMillis: util.ToPtr(int64(math.MaxInt64)), MaxRequests: util.ToPtr(math.MaxInt),
			MaxInputBytes: util.ToPtr(int64(math.MaxInt64)), MaxOutputBytes: util.ToPtr(int64(math.MaxInt64)),
			MaxResponseBytes: util.ToPtr(int64(math.MaxInt64)), MaxEncodedResultBytes: util.ToPtr(int64(math.MaxInt64)),
			MaxStackDepth: util.ToPtr(math.MaxInt),
		},
	} {
		// Oversized positive caps must neither overflow nor increase any ceiling.
		policy := policyLimits()
		got, err := resolveLimits(policy, caps)
		require.NoError(t, err)
		require.Equal(t, policy, got)
	}
}

// TestResolveLimitsTimeoutPrecision checks millisecond caps against duration
// policy without rounding an existing shorter duration up or overflowing it.
func TestResolveLimitsTimeoutPrecision(t *testing.T) {
	for _, test := range []struct {
		policy time.Duration
		millis int64
		want   time.Duration
	}{
		{500 * time.Microsecond, 1, 500 * time.Microsecond},
		{1500 * time.Microsecond, 1, time.Millisecond},
		{time.Second, 1000, time.Second},
		{time.Duration(math.MaxInt64), math.MaxInt64, time.Duration(math.MaxInt64)},
		{time.Duration(math.MaxInt64), math.MaxInt64 / int64(time.Millisecond), time.Duration(math.MaxInt64/int64(time.Millisecond)) * time.Millisecond},
		{0, 1, 0},
		{-time.Second, 1, -time.Second},
	} {
		// Only a representable smaller cap may change either timeout.
		policy := policyLimits()
		policy.HTTP.Timeout, policy.Response.Transform.Timeout = test.policy, test.policy
		got, err := resolveLimits(policy, &toolschema.ExecutionLimits{TimeoutMillis: &test.millis})
		require.NoError(t, err)
		require.Equal(t, test.want, got.HTTP.Timeout)
		require.Equal(t, test.want, got.Response.Transform.Timeout)
	}
}

package jsexec

import (
	"math"
	"strconv"
	"testing"
	"time"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/tools/validation"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// policyLimits supplies distinct finite component ceilings so tests detect
// accidental coupling or increases to an already stricter operator allowance.
func policyLimits() Limits {
	return Limits{
		Render: render.Limits{MaxBytes: 32768, MaxDepth: 32, MaxSteps: 10000},
		Validation: validation.Limits{
			MaxInputBytes:   4096,
			MaxOutputBytes:  1024,
			MaxDepth:        32,
			MaxValues:       10000,
			MaxNumberDigits: 128,
		},
		Runtime: javascript.Limits{
			MaxWorkUnits:       64 << 20,
			MaxAllocationUnits: 64 << 20,
			MaxSourceBytes:     4096,
			MaxInputBytes:      32768,
			MaxOutputBytes:     256,
			MaxDepth:           32,
			MaxNodes:           10000,
			MaxStackDepth:      128,
			Timeout:            time.Second,
		},
		HTTP: aphttp.Limits{
			Timeout:          2 * time.Second,
			MaxRequests:      20,
			MaxResponseBytes: 4096,
			MaxStreamBytes:   8192,
			MaxHeaderBytes:   4096,
		},
		MaxRequestBytes:       8192,
		MaxPendingOperations:  4,
		MaxEncodedResultBytes: 16384,
	}
}

// TestResolveLimitsIndependentCaps checks all authored fields without letting
// argument or output caps also restrict source, configuration, workers, or I/O.
func TestResolveLimitsIndependentCaps(t *testing.T) {
	for _, test := range []struct {
		name string
		caps toolschema.ExecutionLimits
		want func(*Limits)
	}{
		{"timeout", toolschema.ExecutionLimits{TimeoutMillis: util.ToPtr(int64(500))}, func(l *Limits) {
			l.HTTP.Timeout, l.Runtime.Timeout = 500*time.Millisecond, 500*time.Millisecond
		}},
		{"requests", toolschema.ExecutionLimits{MaxRequests: util.ToPtr(2)}, func(l *Limits) {
			l.HTTP.MaxRequests = 2
		}},
		{"arguments", toolschema.ExecutionLimits{MaxInputBytes: util.ToPtr(int64(100))}, func(l *Limits) {
			l.Validation.MaxInputBytes = 100
		}},
		{"output keeps stricter runtime", toolschema.ExecutionLimits{MaxOutputBytes: util.ToPtr(int64(512))}, func(l *Limits) {
			l.Validation.MaxOutputBytes = 512
		}},
		{"output lowers both", toolschema.ExecutionLimits{MaxOutputBytes: util.ToPtr(int64(128))}, func(l *Limits) {
			l.Validation.MaxOutputBytes, l.Runtime.MaxOutputBytes = 128, 128
		}},
		{"responses", toolschema.ExecutionLimits{MaxResponseBytes: util.ToPtr(int64(2048))}, func(l *Limits) {
			l.HTTP.MaxResponseBytes = 2048
		}},
		{"envelope", toolschema.ExecutionLimits{MaxEncodedResultBytes: util.ToPtr(int64(1024))}, func(l *Limits) {
			l.MaxEncodedResultBytes = 1024
		}},
		{"stack", toolschema.ExecutionLimits{MaxStackDepth: util.ToPtr(16)}, func(l *Limits) {
			l.Runtime.MaxStackDepth = 16
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Resolving one cap changes only its intended dimensions and neither input.
			policy, originalCaps := policyLimits(), test.caps.Clone()
			want := policy
			test.want(&want)
			got, err := ResolveLimits(policy, &test.caps)
			require.NoError(t, err)
			require.Equal(t, want, got)
			require.Equal(t, policyLimits(), policy)
			require.Equal(t, originalCaps, &test.caps)

			// Later edits to caller-owned pointers cannot alter the resolved value.
			if test.caps.TimeoutMillis != nil {
				*test.caps.TimeoutMillis = 1
			}
			if test.caps.MaxInputBytes != nil {
				*test.caps.MaxInputBytes = 1
			}
			if test.caps.MaxOutputBytes != nil {
				*test.caps.MaxOutputBytes = 1
			}
			require.Equal(t, want, got)
		})
	}
}

// TestResolveLimitsPreservesCeilings never enlarges operator policy
// while allowing omitted caps and large positive values without integer overflow.
func TestResolveLimitsPreservesCeilings(t *testing.T) {
	for _, caps := range []*toolschema.ExecutionLimits{
		nil, {},
		{
			TimeoutMillis: util.ToPtr(int64(math.MaxInt64)), MaxRequests: util.ToPtr(math.MaxInt),
			MaxInputBytes: util.ToPtr(int64(math.MaxInt64)), MaxOutputBytes: util.ToPtr(int64(math.MaxInt64)),
			MaxResponseBytes: util.ToPtr(int64(math.MaxInt64)), MaxEncodedResultBytes: util.ToPtr(int64(math.MaxInt64)),
			MaxStackDepth: util.ToPtr(math.MaxInt),
		},
	} {
		// Even caps beyond the duration or platform-int range leave stricter policy intact.
		policy := policyLimits()
		got, err := ResolveLimits(policy, caps)
		require.NoError(t, err)
		require.Equal(t, policy, got)
	}
}

// TestResolveLimitsRejectsInvalidCaps does not expose a partial policy when any
// authored cap disables a bound, regardless of whether operator policy is lower.
func TestResolveLimitsRejectsInvalidCaps(t *testing.T) {
	for _, value := range []int64{0, -1} {
		for name, caps := range map[string]*toolschema.ExecutionLimits{
			"timeout":   {TimeoutMillis: util.ToPtr(value)},
			"requests":  {MaxRequests: util.ToPtr(int(value))},
			"input":     {MaxInputBytes: util.ToPtr(value)},
			"output":    {MaxOutputBytes: util.ToPtr(value)},
			"responses": {MaxResponseBytes: util.ToPtr(value)},
			"envelope":  {MaxEncodedResultBytes: util.ToPtr(value)},
			"stack":     {MaxStackDepth: util.ToPtr(int(value))},
		} {
			t.Run(name+"/"+strconv.FormatInt(value, 10), func(t *testing.T) {
				// Each zero/negative field is rejected before cap application.
				got, err := ResolveLimits(policyLimits(), caps)
				require.Error(t, err)
				require.Equal(t, Limits{}, got)
			})
		}
	}
}

// TestResolveLimitsTimeoutPrecision does not round shorter operator durations up
// or multiply unrepresentable milliseconds into an overflowing duration.
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
		// Apply the same boundary to both total HTTP and VM deadlines.
		policy := policyLimits()
		policy.HTTP.Timeout, policy.Runtime.Timeout = test.policy, test.policy
		got, err := ResolveLimits(policy, &toolschema.ExecutionLimits{TimeoutMillis: &test.millis})
		require.NoError(t, err)
		require.Equal(t, test.want, got.HTTP.Timeout)
		require.Equal(t, test.want, got.Runtime.Timeout)
	}
}

// TestResolveLimitsDoesNotRepairOperatorPolicy keeps missing bounds invalid so
// compilation can reject them rather than turning authored caps into defaults.
func TestResolveLimitsDoesNotRepairOperatorPolicy(t *testing.T) {
	caps := &toolschema.ExecutionLimits{
		TimeoutMillis: util.ToPtr(int64(100)), MaxRequests: util.ToPtr(1),
		MaxInputBytes: util.ToPtr(int64(128)), MaxOutputBytes: util.ToPtr(int64(128)),
		MaxResponseBytes: util.ToPtr(int64(128)), MaxEncodedResultBytes: util.ToPtr(int64(128)),
		MaxStackDepth: util.ToPtr(8),
	}
	for _, value := range []int{0, -1} {
		// Positive authored values cannot repair any invalid capped operator field.
		policy := policyLimits()
		policy.HTTP.Timeout, policy.Runtime.Timeout = time.Duration(value), time.Duration(value)
		policy.HTTP.MaxRequests, policy.HTTP.MaxResponseBytes = value, int64(value)
		policy.Validation.MaxInputBytes, policy.Validation.MaxOutputBytes = value, value
		policy.Runtime.MaxOutputBytes, policy.Runtime.MaxStackDepth = value, value
		policy.MaxEncodedResultBytes = value
		got, err := ResolveLimits(policy, caps)
		require.NoError(t, err)
		require.Equal(t, policy, got)
		require.Error(t, got.validateHostLimits())
		require.Error(t, got.Runtime.Validate())
	}
}

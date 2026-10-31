package httpexec

import (
	"fmt"
	"time"

	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
)

// resolveLimits lowers operator policy with the definition's optional caps.
// It copies values without retaining authored pointers or changing either input.
// Component compilers still validate the resolved policy: an authored cap cannot
// repair a missing or invalid operator limit, and unused transform limits may
// remain zero when the definition has no response transform.
func resolveLimits(policy Limits, caps *toolschema.ExecutionLimits) (Limits, error) {
	if err := caps.Validate(nil); err != nil {
		return Limits{}, fmt.Errorf("HTTP Tool limits: %w", err)
	}
	if caps == nil {
		return policy, nil
	}

	policy.HTTP.Timeout = capTimeout(policy.HTTP.Timeout, caps.TimeoutMillis)
	policy.Response.Transform.Timeout = capTimeout(policy.Response.Transform.Timeout, caps.TimeoutMillis)
	if caps.MaxRequests != nil {
		policy.HTTP.MaxRequests = min(policy.HTTP.MaxRequests, *caps.MaxRequests)
	}

	// This is the caller's raw argument allowance. Rendering and transform input
	// also contain connection configuration or response data, so retain their
	// independent operator limits rather than capping them by argument size.
	policy.Validation.MaxInputBytes = capBytes(policy.Validation.MaxInputBytes, caps.MaxInputBytes)
	policy.Validation.MaxOutputBytes = capBytes(policy.Validation.MaxOutputBytes, caps.MaxOutputBytes)
	policy.Response.MaxOutputBytes = capBytes(policy.Response.MaxOutputBytes, caps.MaxOutputBytes)
	policy.Response.Transform.MaxOutputBytes = capBytes(policy.Response.Transform.MaxOutputBytes, caps.MaxOutputBytes)

	if caps.MaxResponseBytes != nil {
		policy.HTTP.MaxResponseBytes = min(policy.HTTP.MaxResponseBytes, *caps.MaxResponseBytes)
	}
	policy.Response.MaxResponseBytes = capBytes(policy.Response.MaxResponseBytes, caps.MaxResponseBytes)
	policy.Response.MaxEncodedResultBytes = capBytes(policy.Response.MaxEncodedResultBytes, caps.MaxEncodedResultBytes)
	if caps.MaxStackDepth != nil {
		policy.Response.Transform.MaxStackDepth = min(policy.Response.Transform.MaxStackDepth, *caps.MaxStackDepth)
	}
	return policy, nil
}

// capBytes applies a validated positive byte cap before converting it to int.
// A cap larger than the platform's int range cannot overflow or raise policy.
func capBytes(policy int, cap *int64) int {
	if cap == nil {
		return policy
	}
	return int(min(int64(policy), *cap))
}

// capTimeout converts validated positive milliseconds only when they fit within
// the existing duration. Comparing first avoids overflow for very large authored
// caps and preserves operator durations shorter than one millisecond.
func capTimeout(policy time.Duration, millis *int64) time.Duration {
	if millis != nil && *millis <= int64(policy/time.Millisecond) {
		return time.Duration(*millis) * time.Millisecond
	}
	return policy
}

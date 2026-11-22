package jsexec

import (
	"errors"
	"fmt"
	"time"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/tools/validation"
)

// Limits contains the caller's operator policy for a JavaScript Tool. Authored
// caps may lower selected allowances but cannot raise them. All fields must be
// positive; this package supplies no defaults or implicit unlimited policy.
// These are processing bounds, not hard process-memory or tenant isolation.
type Limits struct {
	// Render bounds copying the admitted connection metadata and validated
	// arguments into a detached snapshot before native JavaScript conversion.
	Render render.Limits

	// Validation bounds argument JSON and successful structured output while
	// preparing them for schema checks. Its input allowance applies only to
	// caller arguments, separately from connection configuration and metadata.
	Validation validation.Limits

	// Runtime bounds source, combined VM input, output, depth, stack, time,
	// and shared native work/allocation for initialization and execution.
	// Its output byte limit applies to ordinary results, complete toolResult
	// and ToolError payloads, and each console record when logging is enabled.
	Runtime javascript.Limits

	// HTTP bounds the invocation deadline, logical request count, aggregate
	// decoded responses, individual response streams, and copied headers.
	HTTP aphttp.Limits

	// MaxRequestBytes bounds one prepared request's URL, method, headers,
	// body, and label overrides before the runtime admits an HTTP worker.
	MaxRequestBytes int

	// MaxPendingOperations bounds concurrent fetch and response-read workers.
	// It is independent of the total logical request allowance in HTTP.
	MaxPendingOperations int

	// MaxEncodedResultBytes bounds the final ToolResult JSON, including
	// identities, metadata, rich blocks, escaping, and explicit error details.
	// It does not replace Runtime.MaxOutputBytes for encoding authored payloads.
	MaxEncodedResultBytes int
}

// ResolveLimits lowers operator policy with the definition's optional caps.
// It retains no authored pointers and changes neither input. Component compilers
// still validate the complete resolved policy; an authored cap cannot repair a
// missing or invalid operator allowance. Fields without an authored counterpart
// retain their independent operator ceilings.
func ResolveLimits(policy Limits, caps *toolschema.ExecutionLimits) (Limits, error) {
	if err := caps.Validate(nil); err != nil {
		return Limits{}, fmt.Errorf("JavaScript Tool limits: %w", err)
	}
	if caps == nil {
		return policy, nil
	}

	policy.HTTP.Timeout = capTimeout(policy.HTTP.Timeout, caps.TimeoutMillis)
	policy.Runtime.Timeout = capTimeout(policy.Runtime.Timeout, caps.TimeoutMillis)
	if caps.MaxRequests != nil {
		policy.HTTP.MaxRequests = min(policy.HTTP.MaxRequests, *caps.MaxRequests)
	}

	// Argument size does not bound configuration or the combined VM snapshot.
	// Native work/allocation also remains independently controlled by Runtime.
	policy.Validation.MaxInputBytes = capBytes(policy.Validation.MaxInputBytes, caps.MaxInputBytes)
	policy.Validation.MaxOutputBytes = capBytes(policy.Validation.MaxOutputBytes, caps.MaxOutputBytes)
	// The runtime applies this cap to results, explicit errors, and log records.
	policy.Runtime.MaxOutputBytes = capBytes(policy.Runtime.MaxOutputBytes, caps.MaxOutputBytes)
	if caps.MaxResponseBytes != nil {
		policy.HTTP.MaxResponseBytes = min(policy.HTTP.MaxResponseBytes, *caps.MaxResponseBytes)
	}
	policy.MaxEncodedResultBytes = capBytes(policy.MaxEncodedResultBytes, caps.MaxEncodedResultBytes)
	if caps.MaxStackDepth != nil {
		policy.Runtime.MaxStackDepth = min(policy.Runtime.MaxStackDepth, *caps.MaxStackDepth)
	}

	return policy, nil
}

// validateHostLimits rejects disabled snapshot, HTTP, worker, request, and
// envelope bounds before compilation exposes a reusable program. The runtime
// and schema compilers validate their own limits separately.
func (l Limits) validateHostLimits() error {
	if l.Render.MaxBytes <= 0 || l.Render.MaxDepth <= 0 || l.Render.MaxSteps <= 0 {
		return errors.New("JavaScript Tool snapshot limits must all be positive")
	}
	if l.HTTP.Timeout <= 0 || l.HTTP.MaxRequests <= 0 || l.HTTP.MaxResponseBytes <= 0 ||
		l.HTTP.MaxStreamBytes <= 0 || l.HTTP.MaxHeaderBytes <= 0 {
		return errors.New("JavaScript Tool HTTP limits must all be positive")
	}
	if l.MaxRequestBytes <= 0 || l.MaxPendingOperations <= 0 || l.MaxEncodedResultBytes <= 0 {
		return errors.New("JavaScript Tool request, worker, and result limits must all be positive")
	}
	return nil
}

// capBytes applies a positive authored byte cap before converting it to int.
// A value beyond the platform's int range cannot overflow or raise policy.
func capBytes(policy int, cap *int64) int {
	if cap == nil {
		return policy
	}
	return int(min(int64(policy), *cap))
}

// capTimeout compares before multiplying milliseconds, avoiding duration
// overflow and preserving shorter policies, including sub-millisecond values.
func capTimeout(policy time.Duration, millis *int64) time.Duration {
	if millis != nil && *millis <= int64(policy/time.Millisecond) {
		return time.Duration(*millis) * time.Millisecond
	}
	return policy
}

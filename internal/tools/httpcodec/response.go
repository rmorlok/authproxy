package httpcodec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/util"
)

// ErrEncodedResultLimit means the encoded-result allowance cannot hold even a
// safe failure envelope. No partial or oversized ToolResult accompanies it.
var ErrEncodedResultLimit = errors.New("encoded result byte limit cannot hold a tool error")

// ResponseLimits contains resolved positive policy limits for one response
// program. Callers apply operator ceilings before compilation. These limits
// bound data processing; they are not a hard memory-isolation guarantee.
type ResponseLimits struct {
	// MaxSourceBytes caps the combined expression source, rule messages, codes,
	// and selector data. It does not count JSON/YAML serialization framing.
	MaxSourceBytes int

	// MaxResponseBytes caps a successful response's decoded body supplied to Map.
	// ContentType is independently capped at this size before MIME parsing.
	// Status errors ignore both fields. The executor must still bound upstream
	// reads/decompression and account for all responses in a multi-request
	// invocation; this stateless mapper does neither.
	MaxResponseBytes int

	// MaxOutputBytes caps successful content before base64 or envelope encoding:
	// raw JSON bytes, UTF-8 text bytes, or decoded binary bytes. Error bodies are
	// never copied into output, even if they fit this allowance. For transforms,
	// this caps the returned JSON, allowing larger input bodies to be projected down.
	MaxOutputBytes int

	// MaxEncodedResultBytes caps json.Marshal of the complete ToolResult,
	// including identities, metadata, JSON escaping, and base64 expansion.
	MaxEncodedResultBytes int

	// Transform supplies the pure expression runtime's limits, including combined
	// snapshot/response input bytes and execution timeout. All must be positive
	// when a transform is configured; otherwise this field is unused. Runtime
	// limits remain separate from the successful content and envelope caps above.
	Transform javascript.Limits
}

// ResponseInput is a buffered HTTP response plus captured invocation facts.
// The response may come from the upstream or local connection middleware.
// It is an internal mapping input, not a serialized API contract. Map borrows
// its data only during the call and returns independently owned output.
type ResponseInput struct {
	// StatusCode is the received HTTP status, for example 200 or 429. Status
	// rules inspect it before successful body decoding.
	StatusCode int

	// ContentType is the response's Content-Type value, such as application/json.
	// Empty selects application/octet-stream; content is never sniffed.
	ContentType string

	// Body contains the complete bytes after transport content decoding. For
	// example, gzip decompression belongs to the executor/transport, not Map.
	// Empty bytes mean absent content, distinct from the JSON bytes "null".
	Body []byte

	// Headers supplies response metadata to transforms as lowercase names with
	// arrays of values. It is ignored by default passthrough and status errors.
	// ContentType, not this map, controls body decoding. The caller must not mutate
	// the map or value slices during Map; no references enter the JavaScript VM.
	//
	// Map does not filter sensitive fields: every supplied header is visible to
	// the transform and can be projected into Tool output. Executors must pass
	// the already-filtered httphost.Response.Header (or apply the same filter),
	// which removes cookie, authentication, and hop-by-hop fields. Never pass the
	// raw upstream http.Response.Header here.
	Headers http.Header

	// Context is the immutable params/connection snapshot from admission. It is
	// required only for a successful response with a configured transform. The
	// executor must capture it consistently with the result identities below.
	Context *render.Context

	// ToolID and Revision identify the executable revision captured at admission.
	ToolID   string
	Revision uint64

	// InvocationID is the captured tiv_ identity. Binary resource identifiers
	// derive from it rather than any upstream URL or Content-Location header.
	InvocationID string

	// Metadata carries the captured connection and optional source revisions.
	// Pointer fields are copied so later caller edits cannot change the result.
	Metadata api.ToolResultMetadata

	// ExecutionState is the executor's knowledge of upstream execution, used
	// verbatim on mapping failures. notStarted requires evidence that no target
	// request was sent, such as a local rate-limit rejection before any send.
	// An HTTP status alone establishes neither notStarted nor retry safety.
	// Missing and unknown states are invalid; Map does not infer a state.
	ExecutionState api.ToolExecutionState
}

// Response is an immutable status-mapping and content program with an optional
// pure JavaScript projection. It does not perform requests or read response
// streams. Separate invocations may reuse it concurrently without sharing state.
type Response struct {
	// rules holds copied exact, class, and default errors. For example, an exact
	// 429 rule overrides a class-4 rule regardless of their definition order.
	rules *responseRules

	// limits stores the policy captured at compilation; Map keeps no counters
	// or mutable invocation state on the reusable program.
	limits ResponseLimits

	// transform is compiled without running authored code. Each successful
	// invocation evaluates it in a fresh VM after status and input checks.
	transform *javascript.Expression
}

// CompileResponse captures optional response rules and a pure expression.
// A nil definition selects default passthrough for 2xx and UPSTREAM_FAILED for
// other statuses. Configured transforms require positive Transform limits;
// compilation performs no authored execution or I/O.
func CompileResponse(
	source *tools.HTTPResponse,
	limits ResponseLimits,
) (*Response, error) {
	if limits.MaxSourceBytes <= 0 ||
		limits.MaxResponseBytes <= 0 ||
		limits.MaxOutputBytes <= 0 ||
		limits.MaxEncodedResultBytes <= 0 {
		return nil, errors.New("response limits must all be positive")
	}

	rules, err := compileResponseRules(source, limits.MaxSourceBytes)
	if err != nil {
		return nil, err
	}

	var transform *javascript.Expression
	if source != nil && source.TransformJavascript != nil {
		transform, err = javascript.CompileExpression(*source.TransformJavascript, limits.Transform)
		if err != nil {
			return nil, err
		}
	}

	return &Response{
		rules:     rules,
		limits:    limits,
		transform: transform,
	}, nil
}

// NeedsBody reports whether this status can reach successful body mapping.
// Executors check it before reading or decompressing a response: invalid and
// non-2xx statuses, and explicit error rules for 2xx statuses, need no body.
// A true result permits reading; it does not promise that decoding, projection,
// or output validation will succeed. This method never evaluates a transform.
// Nil and uninitialized programs return false and remain invalid for Map.
func (r *Response) NeedsBody(status int) bool {
	if r == nil || r.rules == nil || status < 200 || status >= 300 {
		return false
	}
	_, matched := r.rules.lookup(status)
	return !matched
}

// Map translates a complete buffered response into a detached ToolResult.
// Supplied identities must be valid, and the caller must have authorized the
// invocation. Status rules run before body decoding, including rules for 2xx.
// Upstream error bodies and arbitrary headers never enter public errors.
//
// Protocol, content, and size failures produce bounded ToolResult errors with
// the supplied execution state. Invalid arguments, cancellation, or an allowance
// too small for the safe error envelope return a Go error and no result. The
// caller remains responsible for output-schema validation, aggregate execution
// budgets, and any higher-level API serialization around this result.
func (r *Response) Map(
	ctx context.Context,
	input ResponseInput,
) (*api.ToolResult, error) {
	if r == nil || r.rules == nil || ctx == nil {
		return nil, errors.New("response program and context are required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	switch input.ExecutionState {
	case api.ToolExecutionStateNotStarted, api.ToolExecutionStateMayHaveExecuted, api.ToolExecutionStateCompleted:
	default:
		return nil, errors.New("response execution state must be notStarted, mayHaveExecuted, or completed")
	}

	// ID validators check resource prefixes, not storage size or UTF-8. Bound
	// these required fields before validation/serialization, and never let JSON
	// encoding silently replace bytes in a captured correlation identity.
	remainingIdentityBytes := r.limits.MaxEncodedResultBytes
	for _, identity := range []string{input.ToolID, input.InvocationID, input.Metadata.ConnectionID} {
		if len(identity) > remainingIdentityBytes {
			return nil, ErrEncodedResultLimit
		}

		remainingIdentityBytes -= len(identity)
		if !utf8.ValidString(identity) {
			return nil, errors.New("response invocation identities must be valid UTF-8")
		}
	}

	result := &api.ToolResult{
		ToolID:       input.ToolID,
		Revision:     input.Revision,
		InvocationID: input.InvocationID,
		Content:      []api.ToolContentBlock{},
		Metadata:     input.Metadata,
	}
	result.Metadata.ToolSetGeneration = util.CloneValue(input.Metadata.ToolSetGeneration)
	result.Metadata.CatalogRevision = util.CloneValue(input.Metadata.CatalogRevision)

	if err := result.Validate(nil); err != nil {
		return nil, errors.New("response invocation identity or metadata is invalid")
	}

	var failure *api.ToolResultError
	switch {
	case input.StatusCode < 100 || input.StatusCode > 599:
		failure = &api.ToolResultError{
			Code:    "UPSTREAM_PROTOCOL_ERROR",
			Message: "The upstream response has an invalid HTTP status.",
		}
	default:
		if rule, matched := r.rules.lookup(input.StatusCode); matched {
			failure = &api.ToolResultError{
				Code:      rule.code,
				Message:   rule.message,
				Retryable: rule.retryable,
			}
		} else if input.StatusCode < 200 || input.StatusCode >= 300 {
			failure = &api.ToolResultError{
				Code:    "UPSTREAM_FAILED",
				Message: "The upstream service returned an unsuccessful HTTP status.",
			}
		}

		if failure != nil {
			// Status alone is safe diagnostic data. Never copy a provider's
			// status text, error body, headers, or URL into the public failure.
			// Unused body/MIME size cannot replace an authored status error.
			failure.Details = []byte(fmt.Sprintf(`{"status":%d}`, input.StatusCode))
		} else if len(input.Body) > r.limits.MaxResponseBytes || len(input.ContentType) > r.limits.MaxResponseBytes {
			failure = &api.ToolResultError{Code: "OUTPUT_TOO_LARGE", Message: "The upstream response exceeds the response byte limit."}
		} else if r.transform != nil {
			if input.Context == nil {
				return nil, errors.New("response transform requires a captured invocation context")
			}
			var err error
			failure, err = r.mapTransform(ctx, input, result)
			if err != nil {
				return nil, err
			}
		} else if len(input.Body) > r.limits.MaxOutputBytes {
			failure = &api.ToolResultError{Code: "OUTPUT_TOO_LARGE", Message: "The response content exceeds the output byte limit."}
		} else {
			failure = mapResponseBody(result, input.ContentType, input.Body, r.limits.MaxEncodedResultBytes)
		}
	}

	if failure != nil {
		setResponseFailure(result, failure, input.ExecutionState)
	}

	return finishResponseResult(
		ctx,
		result,
		input.ExecutionState,
		r.limits.MaxEncodedResultBytes,
	)
}

// setResponseFailure discards all partial content before installing a safe
// error. The caller supplies a fresh failure owned by this invocation.
func setResponseFailure(
	result *api.ToolResult,
	failure *api.ToolResultError,
	state api.ToolExecutionState,
) {
	result.StructuredContent = nil
	result.Content = []api.ToolContentBlock{}
	result.IsError = true
	failure.ExecutionState = state
	result.Error = failure
}

// finishResponseResult bounds the complete serialized result, not just its
// payload. An oversized success or authored error is replaced once by a small
// platform failure; if that cannot fit either, no result is returned.
func finishResponseResult(
	ctx context.Context,
	result *api.ToolResult,
	state api.ToolExecutionState,
	maxBytes int,
) (*api.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, errors.New("cannot encode HTTP Tool result")
	}

	if len(encoded) > maxBytes {
		setResponseFailure(
			result,
			&api.ToolResultError{
				Code:    "OUTPUT_TOO_LARGE",
				Message: "The encoded Tool result exceeds the result byte limit.",
			},
			state,
		)

		encoded, err = json.Marshal(result)
		if err != nil || len(encoded) > maxBytes {
			return nil, ErrEncodedResultLimit
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

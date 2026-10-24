package httpcodec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
)

// ErrEncodedResultLimit means the encoded-result allowance cannot hold even a
// safe failure envelope. No partial or oversized ToolResult accompanies it.
var ErrEncodedResultLimit = errors.New("encoded result byte limit cannot hold a tool error")

// ResponseLimits contains resolved positive policy limits for one response
// program. Callers apply operator ceilings before compilation. These limits
// bound data processing; they are not a hard memory-isolation guarantee.
type ResponseLimits struct {
	// MaxSourceBytes caps the raw rule messages, codes, and selector data retained
	// during compilation. It does not count JSON/YAML serialization framing.
	MaxSourceBytes int

	// MaxResponseBytes caps the decoded upstream body supplied to one Map call.
	// ContentType is independently capped at this size before MIME parsing.
	// The executor must bound reads/decompression and account for all responses
	// in a multi-request invocation; this stateless mapper does neither.
	MaxResponseBytes int

	// MaxOutputBytes caps successful content before base64 or envelope encoding:
	// raw JSON bytes, UTF-8 text bytes, or decoded binary bytes. Error bodies are
	// never copied into output, even if they fit this allowance.
	MaxOutputBytes int

	// MaxEncodedResultBytes caps json.Marshal of the complete ToolResult,
	// including identities, metadata, JSON escaping, and base64 expansion.
	MaxEncodedResultBytes int
}

// ResponseInput is a buffered upstream response plus captured invocation facts.
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
	// verbatim on mapping failures. It must be mayHaveExecuted or completed;
	// receipt of an HTTP response cannot establish notStarted. Map does not
	// infer side effects or retry safety from an HTTP status.
	ExecutionState api.ToolExecutionState
}

// Response is an immutable status-mapping and default-content program. It does
// not perform requests, read response streams, or evaluate JavaScript. Separate
// invocations may reuse it concurrently without sharing mutable counters.
type Response struct {
	// rules holds copied exact, class, and default errors. For example, an exact
	// 429 rule overrides a class-4 rule regardless of their definition order.
	rules *responseRules

	// limits stores the policy captured at compilation; Map keeps no counters
	// or mutable invocation state on the reusable program.
	limits ResponseLimits
}

// CompileResponse captures the optional response rules under positive limits.
// A nil definition selects default passthrough for 2xx and UPSTREAM_FAILED for
// other statuses. Configured JavaScript transforms are rejected until a pure
// transform runtime is supplied; they are never silently ignored.
func CompileResponse(source *tools.HTTPResponse, limits ResponseLimits) (*Response, error) {
	if limits.MaxSourceBytes <= 0 || limits.MaxResponseBytes <= 0 || limits.MaxOutputBytes <= 0 || limits.MaxEncodedResultBytes <= 0 {
		return nil, errors.New("response limits must all be positive")
	}
	rules, err := compileResponseRules(source, limits.MaxSourceBytes)
	if err != nil {
		return nil, err
	}
	return &Response{rules: rules, limits: limits}, nil
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
func (r *Response) Map(ctx context.Context, input ResponseInput) (*api.ToolResult, error) {
	if r == nil || r.rules == nil || ctx == nil {
		return nil, errors.New("response program and context are required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input.ExecutionState != api.ToolExecutionStateMayHaveExecuted && input.ExecutionState != api.ToolExecutionStateCompleted {
		return nil, errors.New("response execution state must be mayHaveExecuted or completed")
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
		ToolID: input.ToolID, Revision: input.Revision, InvocationID: input.InvocationID,
		Content: []api.ToolContentBlock{}, Metadata: input.Metadata,
	}
	result.Metadata.ToolSetGeneration = util.CloneValue(input.Metadata.ToolSetGeneration)
	result.Metadata.CatalogRevision = util.CloneValue(input.Metadata.CatalogRevision)
	if err := result.Validate(nil); err != nil {
		return nil, errors.New("response invocation identity or metadata is invalid")
	}

	var failure *api.ToolResultError
	switch {
	case len(input.Body) > r.limits.MaxResponseBytes || len(input.ContentType) > r.limits.MaxResponseBytes:
		failure = &api.ToolResultError{Code: "OUTPUT_TOO_LARGE", Message: "The upstream response exceeds the response byte limit."}
	case input.StatusCode < 100 || input.StatusCode > 599:
		failure = &api.ToolResultError{Code: "UPSTREAM_PROTOCOL_ERROR", Message: "The upstream response has an invalid HTTP status."}
	default:
		if rule, matched := r.rules.lookup(input.StatusCode); matched {
			failure = &api.ToolResultError{Code: rule.code, Message: rule.message, Retryable: rule.retryable}
		} else if input.StatusCode < 200 || input.StatusCode >= 300 {
			failure = &api.ToolResultError{Code: "UPSTREAM_FAILED", Message: "The upstream service returned an unsuccessful HTTP status."}
		}
		if failure != nil {
			// Status alone is safe diagnostic data. Never copy a provider's
			// status text, error body, headers, or URL into the public failure.
			failure.Details = []byte(fmt.Sprintf(`{"status":%d}`, input.StatusCode))
		} else if len(input.Body) > r.limits.MaxOutputBytes {
			failure = &api.ToolResultError{Code: "OUTPUT_TOO_LARGE", Message: "The response content exceeds the output byte limit."}
		} else {
			failure = mapResponseBody(result, input.ContentType, input.Body, r.limits.MaxEncodedResultBytes)
		}
	}
	if failure != nil {
		setResponseFailure(result, failure, input.ExecutionState)
	}
	return finishResponseResult(ctx, result, input.ExecutionState, r.limits.MaxEncodedResultBytes)
}

// setResponseFailure discards all partial content before installing a safe
// error. The caller supplies a fresh failure owned by this invocation.
func setResponseFailure(result *api.ToolResult, failure *api.ToolResultError, state api.ToolExecutionState) {
	result.StructuredContent = nil
	result.Content = []api.ToolContentBlock{}
	result.IsError = true
	failure.ExecutionState = state
	result.Error = failure
}

// finishResponseResult bounds the complete serialized result, not just its
// payload. An oversized success or authored error is replaced once by a small
// platform failure; if that cannot fit either, no result is returned.
func finishResponseResult(ctx context.Context, result *api.ToolResult, state api.ToolExecutionState, maxBytes int) (*api.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, errors.New("cannot encode HTTP Tool result")
	}
	if len(encoded) > maxBytes {
		setResponseFailure(result, &api.ToolResultError{Code: "OUTPUT_TOO_LARGE", Message: "The encoded Tool result exceeds the result byte limit."}, state)
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

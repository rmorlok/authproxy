package httpcodec

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
)

// errTransformHeaders classifies malformed upstream metadata without retaining
// a header name or value that could contain provider or connection data.
var errTransformHeaders = errors.New("invalid response transform headers")

// mapTransform projects a successful, bounded response using a fresh pure VM.
// It populates result only after conversion succeeds; callers install execution
// state on safe failures and enforce the complete serialized result allowance.
func (r *Response) mapTransform(
	ctx context.Context,
	input ResponseInput,
	result *api.ToolResult,
) (*api.ToolResultError, error) {
	decoded, failure := decodeResponseBody(input.ContentType, input.Body)
	if failure != nil {
		return failure, nil
	}

	response, err := encodeTransformResponse(ctx, input, decoded, r.limits.Transform)
	var projected common.RawJSON
	if err == nil {
		projected, err = r.transform.Evaluate(ctx, javascript.Input{
			Context: input.Context, Response: response,
		})
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		switch {
		case errors.Is(err, errTransformHeaders):
			return &api.ToolResultError{Code: "UPSTREAM_PROTOCOL_ERROR", Message: "Provider response has invalid headers."}, nil
		case errors.Is(err, javascript.ErrLimitExceeded):
			return &api.ToolResultError{Code: "EXECUTION_LIMIT_EXCEEDED", Message: "The response transform exceeded an execution limit."}, nil
		default:
			// Do not expose source, thrown values, response data, or private context.
			return &api.ToolResultError{Code: "OUTPUT_INVALID", Message: "The response transform could not produce a valid result."}, nil
		}
	}
	if len(projected) > r.limits.MaxOutputBytes {
		return &api.ToolResultError{Code: "OUTPUT_TOO_LARGE", Message: "The transformed response exceeds the output byte limit."}, nil
	}

	if projected[0] == '"' {
		// The expression encoder emits JSON string escapes that Unquote accepts,
		// except lone UTF-16 surrogates. Reject those: a Go text block cannot carry
		// them, and json.Unmarshal would silently replace them with U+FFFD.
		text, err := strconv.Unquote(string(projected))
		if err != nil {
			return &api.ToolResultError{Code: "OUTPUT_INVALID", Message: "The response transform returned a string that cannot be represented as text."}, nil
		}
		result.Content = append(result.Content, api.ToolContentBlock{Type: api.ToolContentTypeText, Text: &text})
	}
	result.StructuredContent = projected
	return nil, nil
}

// encodeTransformResponse builds the native-JSON input contract without an
// intermediate decoded body graph. Empty content omits body; JSON null remains
// explicit. The runtime subsequently shares its input allowance with Context.
func encodeTransformResponse(
	ctx context.Context,
	input ResponseInput,
	decoded decodedResponseBody,
	limits javascript.Limits,
) (common.RawJSON, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	headers, err := transformHeaders(ctx, input.Headers, limits)
	if err != nil {
		return nil, err
	}
	buffer := transformJSONBuffer{ctx: ctx, maxBytes: limits.MaxInputBytes}
	buffer.write(`{"status":`)
	buffer.write(strconv.Itoa(input.StatusCode))
	buffer.write(`,"headers":{`)
	// Normalized names are bounded before collecting or sorting them. Stable
	// ordering also makes Object.keys(response.headers) independent of Go maps.
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for i, name := range names {
		if i > 0 {
			buffer.write(",")
		}
		buffer.quoted(name)
		buffer.write(":[")
		for j, value := range headers[name] {
			if j > 0 {
				buffer.write(",")
			}
			buffer.quoted(value)
			if buffer.err != nil {
				return nil, buffer.err
			}
		}
		buffer.write("]")
		if buffer.err != nil {
			return nil, buffer.err
		}
	}
	buffer.write(`},"mediaType":`)
	buffer.quoted(decoded.mediaType)
	if decoded.kind != responseBodyEmpty {
		buffer.write(`,"body":`)
		if buffer.err != nil {
			return nil, buffer.err
		}
		switch decoded.kind {
		case responseBodyJSON:
			// Check capacity before converting bytes to a string for the buffer.
			if len(decoded.body) > buffer.remaining() {
				return nil, javascript.ErrLimitExceeded
			}
			buffer.write(string(decoded.body))
		case responseBodyText:
			if len(decoded.body) > buffer.remaining() {
				return nil, javascript.ErrLimitExceeded
			}
			buffer.quoted(string(decoded.body))
		default:
			buffer.write(`{"encoding":"base64","data":"`)
			if buffer.err != nil {
				return nil, buffer.err
			}
			if !responseBase64Fits(len(decoded.body), buffer.remaining()) {
				return nil, javascript.ErrLimitExceeded
			}
			buffer.write(base64.StdEncoding.EncodeToString(decoded.body))
			buffer.write(`"}`)
		}
	}
	buffer.write("}")
	if buffer.err != nil {
		return nil, buffer.err
	}
	return common.RawJSON(buffer.bytes), nil
}

// transformHeaders bounds raw metadata before normalization or JSON escaping.
// It returns lowercase names with borrowed value slices, used only during this
// call. Duplicate names ignoring case are rejected instead of being merged in
// map iteration order. Repeated values in one entry retain their original order.
func transformHeaders(ctx context.Context, source http.Header, limits javascript.Limits) (http.Header, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Even an empty value list needs a name, quotes, colon, and array brackets.
	// Bound width before allocating maps or iterating many zero-byte values.
	if len(source) > limits.MaxNodes || len(source) > limits.MaxInputBytes/6 {
		return nil, javascript.ErrLimitExceeded
	}
	remainingBytes, remainingValues := limits.MaxInputBytes, limits.MaxNodes
	headers := make(http.Header, len(source))
	for name, values := range source {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(name) > remainingBytes || len(values) > remainingValues {
			return nil, javascript.ErrLimitExceeded
		}
		remainingBytes -= len(name)
		remainingValues -= len(values)
		if !tools.IsHTTPToken(name) {
			return nil, errTransformHeaders
		}
		name = strings.ToLower(name)
		if _, exists := headers[name]; exists {
			return nil, errTransformHeaders
		}
		for _, value := range values {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			// Every value requires at least two JSON quotes, even when empty.
			if remainingBytes < 2 || len(value) > remainingBytes-2 {
				return nil, javascript.ErrLimitExceeded
			}
			remainingBytes -= len(value) + 2
			if !validRequestHeaderValue(value) {
				return nil, errTransformHeaders
			}
		}
		headers[name] = values
	}
	return headers, nil
}

// transformJSONBuffer appends already-safe JSON fragments under an exact byte
// cap. Its sticky error avoids further work after cancellation or exhaustion.
// String escaping can allocate up to six bytes per bounded input byte; this is
// bounded preparation work, not a hard heap allocation quota.
type transformJSONBuffer struct {
	// ctx is checked before each fragment or string is processed.
	ctx context.Context
	// maxBytes bounds the serialized response supplied to the expression runtime.
	maxBytes int
	// bytes is invocation-owned JSON and is returned only after complete encoding.
	bytes []byte
	// err retains the first cancellation or byte-limit failure.
	err error
}

// remaining returns unused capacity, or zero after an earlier failure.
func (b *transformJSONBuffer) remaining() int {
	if b.err != nil {
		return 0
	}
	return b.maxBytes - len(b.bytes)
}

// write appends a validated JSON fragment without exceeding the response cap.
func (b *transformJSONBuffer) write(fragment string) {
	if b.err != nil {
		return
	}
	if b.err = b.ctx.Err(); b.err != nil {
		return
	}
	if len(fragment) > b.remaining() {
		b.err = javascript.ErrLimitExceeded
		return
	}
	b.bytes = append(b.bytes, fragment...)
}

// quoted checks minimum size before JSON escaping a known-valid UTF-8 string,
// then charges its exact escaped representation before growing the buffer.
func (b *transformJSONBuffer) quoted(value string) {
	if b.err != nil {
		return
	}
	if b.err = b.ctx.Err(); b.err != nil {
		return
	}
	if b.remaining() < 2 || len(value) > b.remaining()-2 {
		b.err = javascript.ErrLimitExceeded
		return
	}
	encoded, _ := json.Marshal(value)
	b.write(string(encoded))
}

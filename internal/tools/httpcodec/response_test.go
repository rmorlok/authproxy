package httpcodec_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// responseLimits leaves room for ordinary payloads and their result envelopes.
func responseLimits() httpcodec.ResponseLimits {
	return httpcodec.ResponseLimits{
		MaxSourceBytes: 4096, MaxResponseBytes: 8192,
		MaxOutputBytes: 8192, MaxEncodedResultBytes: 16384,
	}
}

// responseInput represents one completed request with captured invocation identity.
func responseInput() httpcodec.ResponseInput {
	return httpcodec.ResponseInput{
		StatusCode: 200, ContentType: "application/json", Body: []byte(`{"ok":true}`),
		ToolID: "tol_01example0000001", Revision: 7, InvocationID: "tiv_01example0000001",
		Metadata:       api.ToolResultMetadata{ConnectionID: "cxn_01example0000001"},
		ExecutionState: api.ToolExecutionStateCompleted,
	}
}

// compileResponse requires a reusable program for a valid authored definition.
func compileResponse(t *testing.T, source *tools.HTTPResponse, limits httpcodec.ResponseLimits) *httpcodec.Response {
	t.Helper()
	program, err := httpcodec.CompileResponse(source, limits)
	require.NoError(t, err)
	require.NotNil(t, program)
	return program
}

// mapResponse also checks that every successful mapping emits a valid envelope.
func mapResponse(t *testing.T, program *httpcodec.Response, input httpcodec.ResponseInput) *api.ToolResult {
	t.Helper()
	result, err := program.Map(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NoError(t, result.Validate(nil))
	require.NotNil(t, result.Content)
	require.Equal(t, input.ToolID, result.ToolID)
	require.Equal(t, input.Revision, result.Revision)
	require.Equal(t, input.InvocationID, result.InvocationID)
	require.Equal(t, input.Metadata, result.Metadata)
	return result
}

// TestResponseStatusMappings applies authored precedence before interpreting
// provider content, including an explicit error mapping for a successful status.
func TestResponseStatusMappings(t *testing.T) {
	program := compileResponse(t, &tools.HTTPResponse{Errors: []tools.HTTPErrorRule{
		{Default: true, Code: "PROVIDER_OTHER", Message: "The provider returned an unexpected status."},
		{StatusClass: util.ToPtr(4), Code: "PROVIDER_CLIENT", Message: "The provider declined the request."},
		{Statuses: []int{404, 410}, Code: "ITEM_MISSING", Message: "The item is unavailable.", Retryable: util.ToPtr(true)},
		{Statuses: []int{200}, Code: "UNEXPECTED_SUCCESS", Message: "This operation must return a different status."},
	}}, responseLimits())
	for _, test := range []struct {
		status    int
		code      string
		retryable bool
	}{
		{404, "ITEM_MISSING", true}, {410, "ITEM_MISSING", true},
		{429, "PROVIDER_CLIENT", false}, {503, "PROVIDER_OTHER", false},
		{201, "PROVIDER_OTHER", false}, {200, "UNEXPECTED_SUCCESS", false},
	} {
		t.Run(fmt.Sprint(test.status), func(t *testing.T) {
			// Neither invalid MIME metadata nor a private upstream body enters the error.
			input := responseInput()
			input.StatusCode = test.status
			input.ContentType = "private-invalid-media-type"
			input.Body = []byte("private-provider-body")
			input.ExecutionState = api.ToolExecutionStateMayHaveExecuted
			result := mapResponse(t, program, input)
			require.True(t, result.IsError)
			require.Equal(t, test.code, result.Error.Code)
			require.Equal(t, test.retryable, result.Error.Retryable)
			require.Equal(t, input.ExecutionState, result.Error.ExecutionState)
			require.JSONEq(t, fmt.Sprintf(`{"status":%d}`, test.status), string(result.Error.Details))
			require.Empty(t, result.Content)
			require.Nil(t, result.StructuredContent)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "private-")
		})
	}
}

// TestResponseDefaultStatusErrors keeps unmatched HTTP failures distinct from
// invalid statuses without exposing the unparsed provider response.
func TestResponseDefaultStatusErrors(t *testing.T) {
	program := compileResponse(t, nil, responseLimits())
	for _, status := range []int{101, 302, 401, 500, 99, 600} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			input := responseInput()
			input.StatusCode, input.ContentType = status, "private-invalid-media-type"
			input.Body = []byte("private-provider-body")
			result := mapResponse(t, program, input)
			require.True(t, result.IsError)
			if status < 100 || status > 599 {
				require.Equal(t, "UPSTREAM_PROTOCOL_ERROR", result.Error.Code)
			} else {
				require.Equal(t, "UPSTREAM_FAILED", result.Error.Code)
				require.JSONEq(t, fmt.Sprintf(`{"status":%d}`, status), string(result.Error.Details))
			}
			require.False(t, result.Error.Retryable)
			require.Equal(t, input.ExecutionState, result.Error.ExecutionState)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "private-")
		})
	}
}

// TestResponseJSONAndEmptyBody preserves JSON values exactly and distinguishes
// an absent response payload from an explicit JSON null or empty JSON string.
func TestResponseJSONAndEmptyBody(t *testing.T) {
	program := compileResponse(t, nil, responseLimits())
	for _, payload := range []string{"", "null", "false", `""`, "9007199254740993", "1.234567890123456789", `{"items":[]}`} {
		t.Run(payload, func(t *testing.T) {
			input := responseInput()
			input.ContentType, input.Body = "application/vnd.example+json; charset=utf-8", []byte(payload)
			result := mapResponse(t, program, input)
			require.False(t, result.IsError)
			require.Empty(t, result.Content)
			if payload == "" {
				require.Nil(t, result.StructuredContent)
			} else {
				require.Equal(t, payload, string(result.StructuredContent))
			}
		})
	}
}

// TestResponseRichContent turns finite text and binary bytes into typed blocks
// while identifying generic embedded data without adding a download endpoint.
func TestResponseRichContent(t *testing.T) {
	program := compileResponse(t, nil, responseLimits())
	for _, test := range []struct {
		name, mediaType string
		payload         []byte
		kind            api.ToolContentType
	}{
		{"text", "text/plain; charset=utf-8", []byte("Hello, 世界\n"), api.ToolContentTypeText},
		{"image", "image/png", []byte{0, 255, 128}, api.ToolContentTypeImage},
		{"audio", "audio/wav", []byte{0, 255, 128}, api.ToolContentTypeAudio},
		{"document", "application/pdf", []byte{0, 255, 128}, api.ToolContentTypeResource},
		{"undeclared", "", []byte("unclassified data"), api.ToolContentTypeResource},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := responseInput()
			input.ContentType, input.Body = test.mediaType, test.payload
			result := mapResponse(t, program, input)
			require.False(t, result.IsError)
			require.Nil(t, result.StructuredContent)
			require.Len(t, result.Content, 1)
			block := result.Content[0]
			require.Equal(t, test.kind, block.Type)
			switch test.kind {
			case api.ToolContentTypeText:
				require.Equal(t, string(test.payload), *block.Text)
				require.Empty(t, block.MIMEType)
			case api.ToolContentTypeImage, api.ToolContentTypeAudio:
				require.Equal(t, test.mediaType, block.MIMEType)
				require.Equal(t, base64.StdEncoding.EncodeToString(test.payload), *block.Data)
			case api.ToolContentTypeResource:
				require.Empty(t, block.MIMEType)
				require.Equal(t, base64.StdEncoding.EncodeToString(test.payload), *block.Resource.Blob)
				if test.mediaType != "" {
					require.Equal(t, test.mediaType, block.Resource.MIMEType)
				}
				uri, err := url.Parse(block.Resource.URI)
				require.NoError(t, err)
				require.True(t, uri.IsAbs())
				require.Contains(t, block.Resource.URI, input.InvocationID)
			}
		})
	}
}

// TestResponseResourceURIEscapesInvocationID keeps prefix-valid identifiers
// intact in the envelope while escaping them inside the generated resource URI.
func TestResponseResourceURIEscapesInvocationID(t *testing.T) {
	program := compileResponse(t, nil, responseLimits())
	for _, test := range []struct {
		name, invocationID, uriID string
	}{
		{"space", "tiv_with space", "tiv_with%20space"},
		{"newline", "tiv_with\nline", "tiv_with%0Aline"},
		{"percent", "tiv_%2f", "tiv_%252f"},
		{"URI delimiters", "tiv_?query#fragment", "tiv_%3Fquery%23fragment"},
		{"slash", "tiv_part/part", "tiv_part%2Fpart"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Existing ID validation permits arbitrary suffixes after a known prefix.
			input := responseInput()
			input.InvocationID, input.ContentType, input.Body = test.invocationID, "application/pdf", []byte("document")
			result := mapResponse(t, program, input)
			require.False(t, result.IsError)
			require.Equal(t, test.invocationID, result.InvocationID)
			require.Len(t, result.Content, 1)
			require.Equal(t, "urn:authproxy:tool-invocation:"+test.uriID+":response", result.Content[0].Resource.URI)
		})
	}
}

// TestResponseInvalidOutput reports malformed upstream content safely while
// retaining the caller's knowledge of execution for every failure category.
func TestResponseInvalidOutput(t *testing.T) {
	program := compileResponse(t, nil, responseLimits())
	for _, state := range []api.ToolExecutionState{api.ToolExecutionStateMayHaveExecuted, api.ToolExecutionStateCompleted} {
		for _, test := range []struct {
			name, mediaType, payload, code string
		}{
			{"JSON syntax", "application/json", `{"private-token":`, "OUTPUT_INVALID"},
			{"JSON trailing value", "application/json", `{} "private-token"`, "OUTPUT_INVALID"},
			{"JSON encoding", "application/json", "\"private-token\xff\"", "OUTPUT_INVALID"},
			{"text encoding", "text/plain", "private-token\xff", "OUTPUT_INVALID"},
			{"MIME syntax", "private-token-invalid", "private-token", "UPSTREAM_PROTOCOL_ERROR"},
		} {
			t.Run(string(state)+"/"+test.name, func(t *testing.T) {
				input := responseInput()
				input.ContentType, input.Body = test.mediaType, []byte(test.payload)
				input.ExecutionState = state
				result := mapResponse(t, program, input)
				require.True(t, result.IsError)
				require.Equal(t, test.code, result.Error.Code)
				require.Equal(t, state, result.Error.ExecutionState)
				require.False(t, result.Error.Retryable)
				require.Empty(t, result.Content)
				require.Nil(t, result.StructuredContent)
				encoded, err := json.Marshal(result)
				require.NoError(t, err)
				require.NotContains(t, string(encoded), "private-token")
			})
		}
	}
}

// TestResponseDecodedByteLimits separates upstream buffering allowances from
// emitted content limits and applies the former even to mapped HTTP errors.
func TestResponseDecodedByteLimits(t *testing.T) {
	input := responseInput()
	input.ContentType, input.Body = "text/plain", []byte(strings.Repeat("x", 32))
	for _, field := range []string{"response", "output"} {
		t.Run(field, func(t *testing.T) {
			limits := responseLimits()
			if field == "response" {
				limits.MaxResponseBytes = len(input.Body)
			} else {
				limits.MaxOutputBytes = len(input.Body)
			}
			// The exact decoded allowance succeeds, and one byte less discards all content.
			result := mapResponse(t, compileResponse(t, nil, limits), input)
			require.False(t, result.IsError)
			if field == "response" {
				limits.MaxResponseBytes--
			} else {
				limits.MaxOutputBytes--
			}
			result = mapResponse(t, compileResponse(t, nil, limits), input)
			require.Equal(t, "OUTPUT_TOO_LARGE", result.Error.Code)
			require.Equal(t, input.ExecutionState, result.Error.ExecutionState)
			require.Empty(t, result.Content)
		})
	}

	// Status mapping avoids decoding private content but cannot bypass its byte cap.
	limits := responseLimits()
	limits.MaxResponseBytes = len(input.Body) - 1
	input.StatusCode = 500
	result := mapResponse(t, compileResponse(t, nil, limits), input)
	require.Equal(t, "OUTPUT_TOO_LARGE", result.Error.Code)

	// A long content type is independently bounded even for an empty response body.
	input.Body, input.ContentType = nil, strings.Repeat("x", limits.MaxResponseBytes+1)
	result = mapResponse(t, compileResponse(t, nil, limits), input)
	require.Equal(t, "OUTPUT_TOO_LARGE", result.Error.Code)
}

// TestResponseEncodedEnvelopeLimit includes JSON escaping, binary expansion,
// and captured metadata rather than counting only the original payload bytes.
func TestResponseEncodedEnvelopeLimit(t *testing.T) {
	for _, test := range []struct {
		name, mediaType string
		payload         []byte
	}{
		{"escaped text", "text/plain", []byte(strings.Repeat("<>&\n", 128))},
		{"escaped JSON", "application/json", []byte(`{"text":"` + strings.Repeat("<>&", 128) + `"}`)},
		{"base64", "image/png", bytes.Repeat([]byte{0, 128, 255}, 128)},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := responseInput()
			input.ContentType, input.Body = test.mediaType, test.payload
			input.Metadata.ToolSetGeneration, input.Metadata.CatalogRevision = util.ToPtr(uint64(2)), util.ToPtr(uint64(9))
			baseline := mapResponse(t, compileResponse(t, nil, responseLimits()), input)
			encoded, err := json.Marshal(baseline)
			require.NoError(t, err)

			// The complete serialized envelope fits exactly, including escaping overhead.
			limits := responseLimits()
			limits.MaxEncodedResultBytes = len(encoded)
			result := mapResponse(t, compileResponse(t, nil, limits), input)
			require.Equal(t, baseline, result)
			limits.MaxEncodedResultBytes--
			result = mapResponse(t, compileResponse(t, nil, limits), input)
			require.Equal(t, "OUTPUT_TOO_LARGE", result.Error.Code)
			require.Equal(t, input.ExecutionState, result.Error.ExecutionState)
			require.Nil(t, result.StructuredContent)
			require.Empty(t, result.Content)
			fallback, err := json.Marshal(result)
			require.NoError(t, err)
			require.LessOrEqual(t, len(fallback), limits.MaxEncodedResultBytes)
		})
	}

	// Authored public errors are bounded too; replace rather than truncate them.
	limits := responseLimits()
	limits.MaxEncodedResultBytes = 512
	source := &tools.HTTPResponse{Errors: []tools.HTTPErrorRule{{Statuses: []int{404}, Code: "MISSING", Message: strings.Repeat("Long safe message. ", 100)}}}
	input := responseInput()
	input.StatusCode, input.ExecutionState = 404, api.ToolExecutionStateMayHaveExecuted
	result := mapResponse(t, compileResponse(t, source, limits), input)
	require.Equal(t, "OUTPUT_TOO_LARGE", result.Error.Code)
	require.Equal(t, input.ExecutionState, result.Error.ExecutionState)
	require.NotContains(t, result.Error.Message, "Long safe message.")

	// An allowance too small for the safe error envelope returns no oversized result.
	limits = responseLimits()
	limits.MaxEncodedResultBytes = 1
	result, err := compileResponse(t, nil, limits).Map(context.Background(), responseInput())
	require.Error(t, err)
	require.Nil(t, result)
}

// TestResponseCompileRejectsUnsupportedDefinitions leaves transform execution
// to a future runtime and disallows ambiguous rules or custom platform codes.
func TestResponseCompileRejectsUnsupportedDefinitions(t *testing.T) {
	for _, source := range []*tools.HTTPResponse{
		{TransformJavascript: util.ToPtr("response.body")},
		{Errors: []tools.HTTPErrorRule{{Statuses: []int{404}, StatusClass: util.ToPtr(4), Code: "MISSING", Message: "Missing."}}},
		{Errors: []tools.HTTPErrorRule{{Statuses: []int{404}, Code: "FIRST", Message: "First."}, {Statuses: []int{404}, Code: "SECOND", Message: "Second."}}},
	} {
		program, err := httpcodec.CompileResponse(source, responseLimits())
		require.Error(t, err)
		require.Nil(t, program)
	}
	for _, code := range []string{
		"INPUT_INVALID", "CONNECTION_UNAVAILABLE", "UPSTREAM_AUTH_FAILED", "UPSTREAM_FAILED",
		"UPSTREAM_PROTOCOL_ERROR", "UNSUPPORTED_CAPABILITY", "EXECUTION_LIMIT_EXCEEDED", "OUTPUT_INVALID", "OUTPUT_TOO_LARGE",
	} {
		program, err := httpcodec.CompileResponse(&tools.HTTPResponse{Errors: []tools.HTTPErrorRule{{Statuses: []int{404}, Code: code, Message: "Reserved."}}}, responseLimits())
		require.Error(t, err, code)
		require.Nil(t, program)
	}
	program, err := httpcodec.CompileResponse(nil, httpcodec.ResponseLimits{})
	require.Error(t, err)
	require.Nil(t, program)
}

// TestResponseRejectsInvalidInvocationContext cannot produce a public result
// without valid correlation data or knowledge that an upstream request occurred.
func TestResponseRejectsInvalidInvocationContext(t *testing.T) {
	program := compileResponse(t, nil, responseLimits())
	for _, test := range []struct {
		name   string
		change func(*httpcodec.ResponseInput)
	}{
		{"tool ID", func(i *httpcodec.ResponseInput) { i.ToolID = i.Metadata.ConnectionID }},
		{"invocation ID", func(i *httpcodec.ResponseInput) { i.InvocationID = i.ToolID }},
		{"revision", func(i *httpcodec.ResponseInput) { i.Revision = 0 }},
		{"connection ID", func(i *httpcodec.ResponseInput) { i.Metadata.ConnectionID = i.ToolID }},
		{"catalog provenance", func(i *httpcodec.ResponseInput) { i.Metadata.CatalogRevision = util.ToPtr(uint64(9)) }},
		{"missing state", func(i *httpcodec.ResponseInput) { i.ExecutionState = "" }},
		{"not started", func(i *httpcodec.ResponseInput) { i.ExecutionState = api.ToolExecutionStateNotStarted }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := responseInput()
			test.change(&input)
			result, err := program.Map(context.Background(), input)
			require.Error(t, err)
			require.Nil(t, result)
		})
	}
	result, err := program.Map(nil, responseInput())
	require.Error(t, err)
	require.Nil(t, result)
	result, err = (*httpcodec.Response)(nil).Map(context.Background(), responseInput())
	require.Error(t, err)
	require.Nil(t, result)
	result, err = (&httpcodec.Response{}).Map(context.Background(), responseInput())
	require.Error(t, err)
	require.Nil(t, result)

	// Cancellation remains a Go context error instead of manufacturing a Tool failure.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = program.Map(ctx, responseInput())
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
}

// TestResponseIdentityEncodingAndSize rejects invalid UTF-8 rather than letting
// JSON replace correlation bytes and bounds all three identifiers together.
func TestResponseIdentityEncodingAndSize(t *testing.T) {
	program := compileResponse(t, nil, responseLimits())
	for _, test := range []struct {
		name   string
		change func(*httpcodec.ResponseInput)
	}{
		{"tool", func(i *httpcodec.ResponseInput) { i.ToolID = "tol_private-id\xff" }},
		{"invocation", func(i *httpcodec.ResponseInput) { i.InvocationID = "tiv_private-id\xff" }},
		{"connection", func(i *httpcodec.ResponseInput) { i.Metadata.ConnectionID = "cxn_private-id\xff" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The prefixes are valid, so encoding validation must reject these bytes.
			input := responseInput()
			test.change(&input)
			result, err := program.Map(context.Background(), input)
			require.Error(t, err)
			require.Nil(t, result)
			require.NotContains(t, err.Error(), "private-id")
		})
	}

	// Each identifier fits separately, but their aggregate exceeds the allowance.
	limits := responseLimits()
	limits.MaxEncodedResultBytes = 128
	input := responseInput()
	input.ToolID = "tol_" + strings.Repeat("a", 60)
	input.InvocationID = "tiv_" + strings.Repeat("b", 60)
	input.Metadata.ConnectionID = "cxn_" + strings.Repeat("c", 60)
	result, err := compileResponse(t, nil, limits).Map(context.Background(), input)
	require.ErrorIs(t, err, httpcodec.ErrEncodedResultLimit)
	require.Nil(t, result)
}

// TestResponseOwnership detaches compiled rules and returned payload/provenance
// from mutable caller storage and from results produced by later mappings.
func TestResponseOwnership(t *testing.T) {
	source := &tools.HTTPResponse{Errors: []tools.HTTPErrorRule{{Statuses: []int{404}, Code: "ITEM_MISSING", Message: "Missing.", Retryable: util.ToPtr(true)}}}
	program := compileResponse(t, source, responseLimits())
	source.Errors[0].Statuses[0], source.Errors[0].Code = 500, "CHANGED"
	*source.Errors[0].Retryable = false
	input := responseInput()
	input.StatusCode = 404
	result := mapResponse(t, program, input)
	require.Equal(t, "ITEM_MISSING", result.Error.Code)
	require.True(t, result.Error.Retryable)

	// Mutating source bytes and metadata after mapping cannot change captured output.
	input = responseInput()
	input.Metadata.ToolSetGeneration, input.Metadata.CatalogRevision = util.ToPtr(uint64(2)), util.ToPtr(uint64(9))
	result = mapResponse(t, program, input)
	input.Body[0] = '['
	*input.Metadata.ToolSetGeneration, *input.Metadata.CatalogRevision = 3, 10
	require.Equal(t, `{"ok":true}`, string(result.StructuredContent))
	require.Equal(t, uint64(2), *result.Metadata.ToolSetGeneration)
	require.Equal(t, uint64(9), *result.Metadata.CatalogRevision)

	// A result's caller owns its payload and cannot corrupt the reusable program.
	result.StructuredContent[0] = '['
	*result.Metadata.ToolSetGeneration = 99
	input = responseInput()
	result = mapResponse(t, program, input)
	require.Equal(t, `{"ok":true}`, string(result.StructuredContent))
	require.Nil(t, result.Metadata.ToolSetGeneration)
}

// TestResponseConcurrentReuse maps independently owned inputs through one
// compiled policy without sharing result slices or per-invocation state.
func TestResponseConcurrentReuse(t *testing.T) {
	program := compileResponse(t, nil, responseLimits())
	failures := make(chan error, 16)
	var workers sync.WaitGroup
	for i := range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			input := responseInput()
			input.Body = []byte(fmt.Sprintf(`{"worker":%d}`, i))
			result, err := program.Map(context.Background(), input)
			if err != nil {
				failures <- err
				return
			}
			if result == nil || result.IsError || !bytes.Equal(result.StructuredContent, input.Body) {
				failures <- fmt.Errorf("worker %d received another invocation's output", i)
				return
			}
			if err := result.Validate(nil); err != nil {
				failures <- err
				return
			}
			result.StructuredContent[0] = '['
		}()
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
}

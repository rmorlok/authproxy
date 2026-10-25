package httpcodec

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/stretchr/testify/require"
)

// transformInputLimits permits normal metadata while keeping preparation caps
// explicit. The expression runtime separately shares these caps with context.
func transformInputLimits() javascript.Limits {
	return javascript.Limits{
		MaxSourceBytes: 4096, MaxInputBytes: 4096, MaxOutputBytes: 4096,
		MaxDepth: 32, MaxNodes: 256, MaxStackDepth: 64, Timeout: time.Second,
	}
}

func TestEncodeTransformResponseExactByteLimits(t *testing.T) {
	for _, test := range []struct {
		name, mediaType, body, want string
		status                      int
	}{
		{
			name: "empty", mediaType: "application/json", status: 204,
			want: `{"status":204,"headers":{},"mediaType":"application/json"}`,
		},
		{
			name: "JSON spelling", mediaType: "application/json", status: 200,
			body: " \n9007199254740993.0100\t",
			want: "{\"status\":200,\"headers\":{},\"mediaType\":\"application/json\",\"body\": \n9007199254740993.0100\t}",
		},
		{
			name: "quoted text", mediaType: "text/plain", status: 200, body: "<\"\n\\é",
			want: `{"status":200,"headers":{},"mediaType":"text/plain","body":"\u003c\"\n\\é"}`,
		},
		{
			name: "one byte base64", mediaType: "application/octet-stream", status: 200, body: "\xff",
			want: `{"status":200,"headers":{},"mediaType":"application/octet-stream","body":{"encoding":"base64","data":"/w=="}}`,
		},
		{
			name: "two byte base64", mediaType: "image/png", status: 200, body: "\x00\xff",
			want: `{"status":200,"headers":{},"mediaType":"image/png","body":{"encoding":"base64","data":"AP8="}}`,
		},
		{
			name: "three byte base64", mediaType: "audio/wav", status: 200, body: "\x00\xff\x01",
			want: `{"status":200,"headers":{},"mediaType":"audio/wav","body":{"encoding":"base64","data":"AP8B"}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Real body classification supplies validated MIME and borrowed bytes.
			body := []byte(test.body)
			decoded, failure := decodeResponseBody(test.mediaType, body)
			require.Nil(t, failure)
			input := ResponseInput{StatusCode: test.status}
			limits := transformInputLimits()
			limits.MaxInputBytes = len(test.want)

			// The exact serialized limit includes framing, quotes, and base64 padding.
			result, err := encodeTransformResponse(context.Background(), input, decoded, limits)
			require.NoError(t, err)
			require.Equal(t, test.want, string(result))
			require.True(t, json.Valid(result))

			// Each smaller allowance fails without exposing partial JSON.
			for _, cap := range []int{len(test.want) - 1, len(test.want) / 2, 1} {
				limits.MaxInputBytes = cap
				partial, err := encodeTransformResponse(context.Background(), input, decoded, limits)
				require.ErrorIs(t, err, javascript.ErrLimitExceeded)
				require.Nil(t, partial)
			}

			// Encoded output is detached even while the decoded view borrows the body.
			if len(body) > 0 {
				body[0] ^= 0xff
				require.Equal(t, test.want, string(result))
			}
		})
	}
}

func TestEncodeTransformResponseHeaders(t *testing.T) {
	// Mixed header order, repeated values, empty values, and empty lists all have
	// explicit representations. Content-Type is metadata, not the body decoder.
	headers := http.Header{
		"X-Zero":       nil,
		"X-Repeat":     {"second", "first"},
		"X-Empty":      {""},
		"X-Quoted":     {"<\"\t\\"},
		"Content-Type": {"text/plain"},
	}
	decoded, failure := decodeResponseBody("application/json; charset=", []byte(`null`))
	require.Nil(t, failure)
	input := ResponseInput{StatusCode: 200, Headers: headers}
	want := `{"status":200,"headers":{"content-type":["text/plain"],"x-empty":[""],"x-quoted":["\u003c\"\t\\"],"x-repeat":["second","first"],"x-zero":[]},"mediaType":"application/json","body":null}`
	limits := transformInputLimits()
	limits.MaxInputBytes = len(want)
	result, err := encodeTransformResponse(context.Background(), input, decoded, limits)
	require.NoError(t, err)
	require.Equal(t, want, string(result))

	// Original maps and slices stay untouched; later changes cannot alter JSON.
	require.Equal(t, []string{"second", "first"}, headers["X-Repeat"])
	require.NotContains(t, headers, "x-repeat")
	headers["X-Repeat"][0] = "changed"
	delete(headers, "X-Empty")
	require.Equal(t, want, string(result))

	// Header escaping participates in the complete serialized byte allowance.
	input.Headers = http.Header{"X-Quoted": {"<\"\t\\"}}
	encoded, err := encodeTransformResponse(context.Background(), input, decoded, transformInputLimits())
	require.NoError(t, err)
	limits.MaxInputBytes = len(encoded) - 1
	partial, err := encodeTransformResponse(context.Background(), input, decoded, limits)
	require.ErrorIs(t, err, javascript.ErrLimitExceeded)
	require.Nil(t, partial)
}

func TestTransformHeadersRejectsMalformedMetadata(t *testing.T) {
	for _, headers := range []http.Header{
		{"": {"private-provider-value"}},
		{"private field": {"private-provider-value"}},
		{"X-Private": {"private-provider-value"}, "x-private": {"other-private-value"}},
		{"X-Private": {"private-provider-value\r\nInjected: true"}},
		{"X-Private": {"private-provider-value\x00"}},
		{"X-Private": {"private-provider-value\x7f"}},
		{"X-Private": {"private-provider-value" + string([]byte{0xff})}},
		{"X-Privaté": {"private-provider-value"}},
	} {
		// Fixed protocol errors disclose neither malformed names nor private data.
		normalized, err := transformHeaders(context.Background(), headers, transformInputLimits())
		require.ErrorIs(t, err, errTransformHeaders)
		require.Nil(t, normalized)
		require.NotContains(t, err.Error(), "private")

		// Encoder callers receive no partial response when metadata is invalid.
		decoded, failure := decodeResponseBody("application/json", []byte(`{"safe":true}`))
		require.Nil(t, failure)
		encoded, err := encodeTransformResponse(context.Background(), ResponseInput{StatusCode: 200, Headers: headers}, decoded, transformInputLimits())
		require.ErrorIs(t, err, errTransformHeaders)
		require.Nil(t, encoded)
	}
}

func TestTransformHeadersBoundsMetadataWork(t *testing.T) {
	// Names plus value bytes and required quotes have an exact raw preflight cap.
	limits := transformInputLimits()
	headers := http.Header{"X-Limit": {"secret"}}
	limits.MaxInputBytes = len("X-Limit") + len("secret") + 2
	normalized, err := transformHeaders(context.Background(), headers, limits)
	require.NoError(t, err)
	require.Equal(t, http.Header{"x-limit": {"secret"}}, normalized)
	limits.MaxInputBytes--
	normalized, err = transformHeaders(context.Background(), headers, limits)
	require.ErrorIs(t, err, javascript.ErrLimitExceeded)
	require.Nil(t, normalized)

	// Empty maps and value lists cannot bypass width or aggregate-value bounds.
	for _, test := range []struct {
		name    string
		headers http.Header
		bytes   int
		nodes   int
	}{
		{"header width", http.Header{"A": nil, "B": nil}, 4096, 1},
		{"header framing", http.Header{"A": nil, "B": nil}, 11, 256},
		{"empty value count", http.Header{"A": {"", "", ""}}, 4096, 2},
		{"shared value count", http.Header{"A": {"", ""}, "B": {"", ""}}, 4096, 3},
		{"empty value bytes", http.Header{"A": {"", "", "", ""}}, 8, 256},
		{"long value", http.Header{"A": {strings.Repeat("x", 100)}}, 64, 256},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := transformInputLimits()
			limits.MaxInputBytes, limits.MaxNodes = test.bytes, test.nodes
			normalized, err := transformHeaders(context.Background(), test.headers, limits)
			require.ErrorIs(t, err, javascript.ErrLimitExceeded)
			require.Nil(t, normalized)
		})
	}
}

func TestTransformResponseCancellation(t *testing.T) {
	// Cancellation must survive empty metadata and nonempty body-size checks.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, headers := range []http.Header{nil, {"X-Value": {"value"}}} {
		normalized, err := transformHeaders(canceled, headers, transformInputLimits())
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, normalized)
		for _, mediaType := range []string{"application/json", "text/plain", "application/octet-stream"} {
			decoded, failure := decodeResponseBody(mediaType, []byte(`"body"`))
			require.Nil(t, failure)
			encoded, err := encodeTransformResponse(canceled, ResponseInput{StatusCode: 200, Headers: headers}, decoded, transformInputLimits())
			require.ErrorIs(t, err, context.Canceled)
			require.Nil(t, encoded)
		}
	}
}

func TestTransformJSONBufferStickyErrors(t *testing.T) {
	// Quoting reserves exact serialized bytes, including HTML and control escapes.
	want := `"\u003c\"\n"`
	buffer := transformJSONBuffer{ctx: context.Background(), maxBytes: len(want)}
	buffer.quoted("<\"\n")
	require.NoError(t, buffer.err)
	require.Equal(t, want, string(buffer.bytes))
	require.Zero(t, buffer.remaining())

	// Once exhausted, no later fragment or string modifies the accumulated bytes.
	buffer.write(" ")
	require.ErrorIs(t, buffer.err, javascript.ErrLimitExceeded)
	buffer.write("")
	buffer.quoted("private-provider-value")
	require.ErrorIs(t, buffer.err, javascript.ErrLimitExceeded)
	require.Equal(t, want, string(buffer.bytes))

	// A canceled append preserves the earlier prefix and its cancellation error.
	ctx, cancel := context.WithCancel(context.Background())
	buffer = transformJSONBuffer{ctx: ctx, maxBytes: 256}
	buffer.write("{")
	cancel()
	buffer.quoted("private-provider-value")
	buffer.write("}")
	require.ErrorIs(t, buffer.err, context.Canceled)
	require.Equal(t, "{", string(buffer.bytes))
	require.Zero(t, buffer.remaining())

	// Escaping that exceeds the cap fails before appending any quoted string.
	buffer = transformJSONBuffer{ctx: context.Background(), maxBytes: len(want) - 1}
	buffer.quoted("<\"\n")
	require.ErrorIs(t, buffer.err, javascript.ErrLimitExceeded)
	require.Empty(t, buffer.bytes)
}

// FuzzTransformResponseEncoding exercises bounded framing across JSON, text,
// and arbitrary binary bodies with provider-controlled header values.
func FuzzTransformResponseEncoding(f *testing.F) {
	f.Add([]byte(`{"id":9007199254740993}`), "safe", uint16(256), uint8(0))
	f.Add([]byte("<\"\n"), "quoted\"\tvalue", uint16(128), uint8(1))
	f.Add([]byte{0, 0xff, 1}, "safe", uint16(64), uint8(2))
	f.Add([]byte{}, "private\r\nvalue", uint16(256), uint8(2))
	f.Fuzz(func(t *testing.T, body []byte, header string, cap uint16, mode uint8) {
		if len(body) > 2048 || len(header) > 512 {
			return
		}
		mediaType := []string{"application/json", "text/plain", "application/octet-stream"}[mode%3]
		decoded, failure := decodeResponseBody(mediaType, body)
		if failure != nil {
			return
		}
		limits := transformInputLimits()
		limits.MaxInputBytes = int(cap%2048) + 1
		encoded, err := encodeTransformResponse(context.Background(), ResponseInput{StatusCode: 200, Headers: http.Header{"X-Provider": {header}}}, decoded, limits)
		if err != nil {
			require.True(t, errors.Is(err, javascript.ErrLimitExceeded) || errors.Is(err, errTransformHeaders))
			require.Nil(t, encoded)
			return
		}

		// Every successful encoding is complete bounded JSON with unchanged data.
		require.LessOrEqual(t, len(encoded), limits.MaxInputBytes)
		var response map[string]common.RawJSON
		require.NoError(t, json.Unmarshal(encoded, &response))
		require.Equal(t, common.RawJSON("200"), response["status"])
		var headers map[string][]string
		require.NoError(t, json.Unmarshal(response["headers"], &headers))
		require.Equal(t, map[string][]string{"x-provider": {header}}, headers)
		if len(body) == 0 {
			require.NotContains(t, response, "body")
			return
		}
		switch mode % 3 {
		case 0:
			require.Contains(t, string(encoded), `,"body":`+string(body)+`}`)
		case 1:
			var text string
			require.NoError(t, json.Unmarshal(response["body"], &text))
			require.Equal(t, string(body), text)
		case 2:
			var binary map[string]string
			require.NoError(t, json.Unmarshal(response["body"], &binary))
			require.Equal(t, "base64", binary["encoding"])
			roundTrip, err := base64.StdEncoding.DecodeString(binary["data"])
			require.NoError(t, err)
			require.Equal(t, body, roundTrip)
		}
	})
}

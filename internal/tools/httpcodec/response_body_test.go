package httpcodec

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/stretchr/testify/require"
)

// responseBodyResult supplies valid correlation fields and empty success content.
func responseBodyResult() *api.ToolResult {
	return &api.ToolResult{
		ToolID: "tol_01example0000001", Revision: 7, InvocationID: "tiv_01example0000001",
		Content: []api.ToolContentBlock{}, Metadata: api.ToolResultMetadata{ConnectionID: "cxn_01example0000001"},
	}
}

// TestMapResponseBodyJSONPreservesValues retains JSON scalars, null, whitespace,
// and numbers that cannot be represented precisely by a floating-point value.
func TestMapResponseBodyJSONPreservesValues(t *testing.T) {
	for _, mediaType := range []string{"application/json", "Application/Problem+JSON; charset=UTF-8", "text/json"} {
		for _, body := range []string{`null`, `false`, `"text"`, `9007199254740993.0100`, `1e9999`, " \n[null,false,9007199254740993]\t", `{"value":42}`} {
			t.Run(mediaType+"/"+body, func(t *testing.T) {
				// Mapping copies one complete JSON value without decoding its numbers.
				result := responseBodyResult()
				failure := mapResponseBody(result, mediaType, []byte(body), 4096)
				require.Nil(t, failure)
				require.Equal(t, body, string(result.StructuredContent))
				require.NotNil(t, result.Content)
				require.Empty(t, result.Content)
				require.NoError(t, result.Validate(nil))
			})
		}
	}
}

// TestMapResponseBodyEmpty distinguishes omitted content from a supplied JSON
// null and never emits an empty text, image, audio, or binary resource block.
func TestMapResponseBodyEmpty(t *testing.T) {
	for _, mediaType := range []string{"", "application/json", "text/plain", "image/png", "audio/wav", "application/pdf"} {
		for _, body := range [][]byte{nil, {}} {
			// Empty bodies retain the initially empty result even with a JSON MIME type.
			result := responseBodyResult()
			require.Nil(t, mapResponseBody(result, mediaType, body, 1))
			require.Nil(t, result.StructuredContent)
			require.NotNil(t, result.Content)
			require.Empty(t, result.Content)
		}
	}

	// Explicit JSON null remains present in the serialized public envelope.
	result := responseBodyResult()
	require.Nil(t, mapResponseBody(result, "application/json", []byte("null"), 4))
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"structuredContent":null`)
}

// TestMapResponseBodyTextPreservesSupportedCharsets accepts declared text only
// when its bytes satisfy UTF-8 or US-ASCII, without HTML or template processing.
func TestMapResponseBodyTextPreservesSupportedCharsets(t *testing.T) {
	for _, test := range []struct{ mediaType, body string }{
		{"text/plain", "雪 and café\n"},
		{"Text/HTML; charset=UTF-8", `<script>{{cfg.secret}}</script>`},
		{"text/plain; charset=utf8", "résumé"},
		{"text/csv; charset=US-ASCII", "name,value\r\nAda,42\x00\x7f"},
	} {
		t.Run(test.mediaType, func(t *testing.T) {
			// Supported text becomes a plain text block with the exact original bytes.
			result := responseBodyResult()
			require.Nil(t, mapResponseBody(result, test.mediaType, []byte(test.body), 4096))
			require.Nil(t, result.StructuredContent)
			require.Len(t, result.Content, 1)
			require.Equal(t, api.ToolContentTypeText, result.Content[0].Type)
			require.Equal(t, test.body, *result.Content[0].Text)
			require.Empty(t, result.Content[0].MIMEType)
			require.NoError(t, result.Validate(nil))
		})
	}
}

// TestMapResponseBodyBinaryVariants preserves exact bytes and authored MIME
// parameters in typed media blocks or invocation-scoped embedded resources.
func TestMapResponseBodyBinaryVariants(t *testing.T) {
	for _, test := range []struct {
		mediaType string
		wantMIME  string
		kind      api.ToolContentType
	}{
		{"Image/PNG; profile=original", "Image/PNG; profile=original", api.ToolContentTypeImage},
		{"audio/wav", "audio/wav", api.ToolContentTypeAudio},
		{"application/pdf", "application/pdf", api.ToolContentTypeResource},
		{"", "application/octet-stream", api.ToolContentTypeResource},
		{"text/plain; charset=iso-8859-1", "text/plain; charset=iso-8859-1", api.ToolContentTypeResource},
		{"text/plain; charset=utf-16", "text/plain; charset=utf-16", api.ToolContentTypeResource},
	} {
		t.Run(test.wantMIME, func(t *testing.T) {
			// Unsupported text encodings stay binary, including bytes invalid as UTF-8.
			body := []byte{0, 0xff, 0x80, '\n', 0xe9}
			result := responseBodyResult()
			require.Nil(t, mapResponseBody(result, test.mediaType, body, 4096))
			require.Nil(t, result.StructuredContent)
			require.Len(t, result.Content, 1)
			block := result.Content[0]
			require.Equal(t, test.kind, block.Type)
			var encoded string
			if test.kind == api.ToolContentTypeResource {
				// The resource identifier contains only the captured invocation ID.
				require.NotNil(t, block.Resource)
				require.Equal(t, "urn:authproxy:tool-invocation:tiv_01example0000001:response", block.Resource.URI)
				require.Equal(t, test.wantMIME, block.Resource.MIMEType)
				require.Nil(t, block.Resource.Text)
				require.Empty(t, block.MIMEType)
				encoded = *block.Resource.Blob
			} else {
				require.Equal(t, test.wantMIME, block.MIMEType)
				encoded = *block.Data
			}
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			require.NoError(t, err)
			require.Equal(t, body, decoded)
			require.NoError(t, result.Validate(nil))
		})
	}
}

// TestMapResponseBodyResourceEscapesInvocationID preserves supported identifiers
// containing whitespace or reserved URI characters without creating an invalid URI.
func TestMapResponseBodyResourceEscapesInvocationID(t *testing.T) {
	// The existing ID contract accepts this full value, so mapping must preserve it.
	invocationID := "tiv_space and\nslash/?#%雪"
	result := responseBodyResult()
	result.InvocationID = invocationID
	require.NoError(t, result.Validate(nil))
	body := []byte{0, 0xff, '\n'}
	require.Nil(t, mapResponseBody(result, "application/octet-stream", body, 4))
	require.Len(t, result.Content, 1)
	resource := result.Content[0].Resource
	require.NotNil(t, resource)

	// Escaping only the identifier component keeps the URI valid and reversible.
	require.Equal(t, "urn:authproxy:tool-invocation:tiv_space%20and%0Aslash%2F%3F%23%25%E9%9B%AA:response", resource.URI)
	parsed, err := url.Parse(resource.URI)
	require.NoError(t, err)
	require.True(t, parsed.IsAbs())
	component := strings.TrimSuffix(strings.TrimPrefix(resource.URI, "urn:authproxy:tool-invocation:"), ":response")
	decodedID, err := url.PathUnescape(component)
	require.NoError(t, err)
	require.Equal(t, invocationID, decodedID)
	require.Equal(t, invocationID, result.InvocationID)
	decodedBody, err := base64.StdEncoding.DecodeString(*resource.Blob)
	require.NoError(t, err)
	require.Equal(t, body, decodedBody)
	require.NoError(t, result.Validate(nil))

	// The same identity and payload generate the same identifier and representation.
	repeated := responseBodyResult()
	repeated.InvocationID = invocationID
	require.Nil(t, mapResponseBody(repeated, "application/octet-stream", body, 4))
	require.Equal(t, result, repeated)
}

// TestMapResponseBodyInvalidDataIsPrivate returns fixed safe failures without
// exposing header/body contents, fabricating execution state, or changing data.
func TestMapResponseBodyInvalidDataIsPrivate(t *testing.T) {
	for _, test := range []struct {
		name, mediaType, body, code string
	}{
		{"malformed media", "private-provider-header", "private-provider-body", "UPSTREAM_PROTOCOL_ERROR"},
		{"header controls", "text/plain\r\nprivate-provider-header", "private-provider-body", "UPSTREAM_PROTOCOL_ERROR"},
		{"malformed media empty body", "private-provider-header", "", "UPSTREAM_PROTOCOL_ERROR"},
		{"wildcard", "text/*", "private-provider-body", "UPSTREAM_PROTOCOL_ERROR"},
		{"invalid JSON", "application/json", `{"secret":"private-provider-body"`, "OUTPUT_INVALID"},
		{"multiple JSON values", "application/json", `"private-provider-body" true`, "OUTPUT_INVALID"},
		{"JSON whitespace only", "application/json", " \n\t", "OUTPUT_INVALID"},
		{"invalid UTF-8 in JSON", "application/json", "\"private-provider-body\xff\"", "OUTPUT_INVALID"},
		{"invalid default text", "text/plain", "private-provider-body\xff", "OUTPUT_INVALID"},
		{"invalid UTF-8 text", "text/plain; charset=utf-8", "private-provider-body\xff", "OUTPUT_INVALID"},
		{"non-ASCII text", "text/plain; charset=us-ascii", "private-provider-body é", "OUTPUT_INVALID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Capture the original result so every error must be an atomic no-op.
			result := responseBodyResult()
			before := result.Clone()
			failure := mapResponseBody(result, test.mediaType, []byte(test.body), 4096)
			require.NotNil(t, failure)
			require.Equal(t, test.code, failure.Code)
			require.NotEmpty(t, failure.Message)
			require.Equal(t, &api.ToolResultError{Code: failure.Code, Message: failure.Message}, failure)
			require.NotContains(t, failure.Message, "private-provider")
			require.NotContains(t, failure.Message, test.mediaType)
			require.Equal(t, before, result)
		})
	}
}

// TestMapResponseBodyOwnsPayload detaches all successful representations from
// the caller's reusable input buffer, including JSON's original raw bytes.
func TestMapResponseBodyOwnsPayload(t *testing.T) {
	for _, mediaType := range []string{"application/json", "text/plain", "image/png", "audio/wav", "application/octet-stream"} {
		t.Run(mediaType, func(t *testing.T) {
			body := []byte(`"original"`)
			result := responseBodyResult()
			require.Nil(t, mapResponseBody(result, mediaType, body, 4096))
			before := result.Clone()

			// Reusing the upstream buffer cannot rewrite any returned content.
			for i := range body {
				body[i] = '!'
			}
			require.Equal(t, before, result)
			if result.StructuredContent != nil {
				// Mutating the result likewise cannot reach the original upstream buffer.
				result.StructuredContent[0] = '?'
				require.Equal(t, byte('!'), body[0])
			}
		})
	}
}

// TestMapResponseBodyBase64Limits charges padding and encoded expansion before
// allocating image, audio, or generic resource output, with no partial content.
func TestMapResponseBodyBase64Limits(t *testing.T) {
	for _, mediaType := range []string{"image/png", "audio/wav", "application/octet-stream", "text/plain; charset=utf-16"} {
		for length := 1; length <= 7; length++ {
			// The exact base64 size fits, including final padding for short groups.
			body := []byte(strings.Repeat("x", length))
			limit := base64.StdEncoding.EncodedLen(length)
			result := responseBodyResult()
			require.Nil(t, mapResponseBody(result, mediaType, body, limit))
			require.Len(t, result.Content, 1)

			// One fewer byte must fail before any rich block is attached.
			result = responseBodyResult()
			failure := mapResponseBody(result, mediaType, body, limit-1)
			require.NotNil(t, failure)
			require.Equal(t, "OUTPUT_TOO_LARGE", failure.Code)
			require.Empty(t, result.Content)
			require.Nil(t, result.StructuredContent)
		}
	}
}

// TestResponseBase64FitsAvoidsOverflow checks arithmetic near the platform's
// largest integer without allocating correspondingly large response buffers.
func TestResponseBase64FitsAvoidsOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	largestSource := maxInt / 4 * 3
	for _, test := range []struct {
		sourceBytes, limit int
		fits               bool
	}{
		{0, 0, true}, {1, 0, false}, {1, 3, false}, {1, 4, true}, {3, 4, true}, {4, 7, false}, {4, 8, true},
		{-1, 4, false}, {1, -1, false}, {largestSource, maxInt, true}, {largestSource + 1, maxInt, false}, {maxInt, maxInt, false},
	} {
		// Group-count comparison cannot overflow when encoded length would exceed int.
		require.Equal(t, test.fits, responseBase64Fits(test.sourceBytes, test.limit))
	}
}

// FuzzMapResponseBody checks arbitrary provider bytes and MIME metadata produce
// deterministic, valid rich content or an atomic failure with a fixed message.
func FuzzMapResponseBody(f *testing.F) {
	for _, seed := range []struct {
		mediaType string
		body      []byte
		limit     uint16
	}{
		{"application/json", []byte(`null`), 64},
		{"application/problem+json", []byte(`false`), 64},
		{"application/json", []byte(`9007199254740993.0100`), 64},
		{"application/json", []byte{'"', 0xff, '"'}, 64},
		{"text/plain", []byte("café\n{{cfg.secret}}"), 64},
		{"text/plain; charset=us-ascii", []byte{0xff}, 64},
		{"text/plain; charset=utf-16", []byte{0xff, 0, 0x80}, 64},
		{"image/png", []byte{0, 0xff, 0x80}, 4},
		{"audio/wav", []byte{0, 0xff, 0x80}, 3},
		{"private-header\r\n", []byte("private-body"), 64},
		{"", []byte{0, 0xff}, 64},
		{"application/json", nil, 0},
	} {
		f.Add(seed.mediaType, seed.body, seed.limit)
	}
	f.Fuzz(func(t *testing.T, mediaType string, body []byte, encodedLimit uint16) {
		if len(mediaType)+len(body) > 8*1024 {
			return
		}
		// Repeated conversion of the same bounded inputs must have the same outcome.
		limit := int(encodedLimit) % (16 * 1024)
		result, repeated := responseBodyResult(), responseBodyResult()
		before := result.Clone()
		failure := mapResponseBody(result, mediaType, body, limit)
		repeatedFailure := mapResponseBody(repeated, mediaType, body, limit)
		require.Equal(t, failure, repeatedFailure)
		require.Equal(t, result, repeated)
		if failure != nil {
			// A finite public-message set cannot echo attacker-controlled body/header data.
			require.Equal(t, before, result)
			require.Contains(t, []api.ToolResultError{
				{Code: "UPSTREAM_PROTOCOL_ERROR", Message: "Provider response has an invalid content type."},
				{Code: "OUTPUT_INVALID", Message: "Provider response body is not valid JSON."},
				{Code: "OUTPUT_INVALID", Message: "Provider response body is not valid text."},
				{Code: "OUTPUT_TOO_LARGE", Message: "Provider response body exceeds the encoded output limit."},
			}, *failure)
			return
		}

		// Every accepted representation validates and retains the original payload bytes.
		require.NoError(t, result.Validate(nil))
		if result.StructuredContent != nil {
			require.True(t, json.Valid(result.StructuredContent))
			require.Equal(t, body, []byte(result.StructuredContent))
		}
		for _, block := range result.Content {
			if block.Type == api.ToolContentTypeText {
				require.Equal(t, string(body), *block.Text)
				continue
			}
			encoded := block.Data
			if block.Type == api.ToolContentTypeResource {
				encoded = block.Resource.Blob
			}
			require.NotNil(t, encoded)
			require.LessOrEqual(t, len(*encoded), limit)
			decoded, err := base64.StdEncoding.DecodeString(*encoded)
			require.NoError(t, err)
			require.Equal(t, body, decoded)
		}
	})
}

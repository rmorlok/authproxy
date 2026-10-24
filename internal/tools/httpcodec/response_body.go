package httpcodec

import (
	"encoding/base64"
	"encoding/json"
	"mime"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
)

// mapResponseBody converts a bounded successful response into rich result data.
// The caller supplies a fresh result with validated identities and nonnil Content,
// bounds input/output bytes, and sets execution state on any returned failure.
// JSON bytes are copied exactly; text and encoded binary content own their data.
// Errors contain only safe code/message fields and leave the result unchanged.
// maxEncodedBytes bounds base64 allocation; the caller bounds the final envelope.
func mapResponseBody(result *api.ToolResult, mediaType string, body []byte, maxEncodedBytes int) *api.ToolResultError {
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	base, err := tools.ParseBodyMediaType(mediaType)
	if err != nil {
		return &api.ToolResultError{Code: "UPSTREAM_PROTOCOL_ERROR", Message: "Provider response has an invalid content type."}
	}
	// An empty response has no content, including when JSON was declared.
	if len(body) == 0 {
		return nil
	}

	major, subtype, _ := strings.Cut(base, "/")
	if subtype == "json" || strings.HasSuffix(subtype, "+json") {
		if !utf8.Valid(body) || !json.Valid(body) {
			return &api.ToolResultError{Code: "OUTPUT_INVALID", Message: "Provider response body is not valid JSON."}
		}
		result.StructuredContent = slices.Clone(body)
		return nil
	}

	if major == "text" {
		_, params, _ := mime.ParseMediaType(mediaType)
		charset := strings.ToLower(params["charset"])
		if charset == "" || charset == "utf-8" || charset == "utf8" || charset == "us-ascii" {
			if !utf8.Valid(body) || (charset == "us-ascii" && !isASCIIResponseBody(body)) {
				return &api.ToolResultError{Code: "OUTPUT_INVALID", Message: "Provider response body is not valid text."}
			}
			text := string(body)
			result.Content = append(result.Content, api.ToolContentBlock{Type: api.ToolContentTypeText, Text: &text})
			return nil
		}
		// Unsupported charsets remain opaque bytes instead of being decoded
		// incorrectly or silently replaced by UTF-8 replacement characters.
	}

	if !responseBase64Fits(len(body), maxEncodedBytes) {
		return &api.ToolResultError{Code: "OUTPUT_TOO_LARGE", Message: "Provider response body exceeds the encoded output limit."}
	}
	encoded := base64.StdEncoding.EncodeToString(body)
	switch major {
	case "image":
		result.Content = append(result.Content, api.ToolContentBlock{Type: api.ToolContentTypeImage, Data: &encoded, MIMEType: mediaType})
	case "audio":
		result.Content = append(result.Content, api.ToolContentBlock{Type: api.ToolContentTypeAudio, Data: &encoded, MIMEType: mediaType})
	default:
		// Supported IDs may contain URI-reserved characters. Escape the complete
		// value as one component without changing the captured invocation identity.
		resourceURI := "urn:authproxy:tool-invocation:" + url.PathEscape(result.InvocationID) + ":response"
		result.Content = append(result.Content, api.ToolContentBlock{
			Type: api.ToolContentTypeResource,
			Resource: &api.ToolResourceContent{
				URI: resourceURI, MIMEType: mediaType, Blob: &encoded,
			},
		})
	}
	return nil
}

// isASCIIResponseBody checks the declared US-ASCII charset without changing
// payload bytes; ASCII control characters remain valid text content.
func isASCIIResponseBody(body []byte) bool {
	for _, value := range body {
		if value > 0x7f {
			return false
		}
	}
	return true
}

// responseBase64Fits compares complete four-byte encoding groups before any
// multiplication, avoiding overflow for both the encoded size and the limit.
// Empty input requires no capacity; negative lengths or limits are invalid.
func responseBase64Fits(sourceBytes, maxEncodedBytes int) bool {
	if sourceBytes < 0 || maxEncodedBytes < 0 {
		return false
	}
	groups := sourceBytes / 3
	if sourceBytes%3 != 0 {
		groups++
	}
	return groups <= maxEncodedBytes/4
}

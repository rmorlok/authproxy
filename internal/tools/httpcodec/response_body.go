package httpcodec

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
)

// responseBodyKind identifies the validated representation shared by default
// rich-result mapping and response transforms. Binary kinds retain opaque bytes.
type responseBodyKind uint8

const (
	responseBodyEmpty  responseBodyKind = iota // No body, including empty declared JSON.
	responseBodyJSON                           // One valid UTF-8 JSON value, without numeric conversion.
	responseBodyText                           // Valid UTF-8 or declared US-ASCII text.
	responseBodyImage                          // Opaque image bytes.
	responseBodyAudio                          // Opaque audio bytes.
	responseBodyBinary                         // Other bytes, including unsupported or ambiguous text charsets.
)

// decodedResponseBody classifies a bounded provider body without copying or
// reencoding its bytes. Callers must keep the source buffer unchanged while
// using this value and copy data retained beyond that buffer's lifetime.
type decodedResponseBody struct {
	// kind selects how consumers interpret the validated bytes.
	kind responseBodyKind
	// mediaType is the effective valid MIME type. Missing types default to
	// application/octet-stream; malformed optional parameters are discarded.
	mediaType string
	// body borrows the caller's bytes, preserving JSON spelling and precision.
	body []byte
}

// decodeResponseBody validates MIME metadata and JSON/text encodings before
// classifying the body. It does not parse JSON into Go values, transcode text,
// or apply size limits; callers must bound the supplied metadata and body first.
// Empty bodies still require valid MIME metadata. Failures return no decoded
// value and contain only fixed, safe code/message fields.
func decodeResponseBody(
	mediaType string,
	body []byte,
) (decodedResponseBody, *api.ToolResultError) {
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}

	base, params, parameterErr := mime.ParseMediaType(mediaType)
	_, baseErr := tools.ParseBodyMediaType(base)

	if !tools.IsValidMultipartMetadata(mediaType) || baseErr != nil ||
		(parameterErr != nil && !errors.Is(parameterErr, mime.ErrInvalidMediaParameter)) {
		return decodedResponseBody{}, &api.ToolResultError{
			Code:    "UPSTREAM_PROTOCOL_ERROR",
			Message: "Provider response has an invalid content type.",
		}
	}

	if parameterErr != nil {
		// A valid base type still identifies JSON and binary media when optional
		// parameters are malformed. Publish only that valid type; the original
		// value would make rich results invalid. Unknown text charsets stay binary.
		mediaType = base
	}

	// An empty response has no content, including when JSON was declared.
	decoded := decodedResponseBody{kind: responseBodyEmpty, mediaType: mediaType, body: body}
	if len(body) == 0 {
		return decoded, nil
	}

	major, subtype, _ := strings.Cut(base, "/")
	if subtype == "json" || strings.HasSuffix(subtype, "+json") {
		if !utf8.Valid(body) || !json.Valid(body) {
			return decodedResponseBody{}, &api.ToolResultError{
				Code:    "OUTPUT_INVALID",
				Message: "Provider response body is not valid JSON.",
			}
		}

		decoded.kind = responseBodyJSON
		return decoded, nil
	}

	if major == "text" && parameterErr == nil {
		charset := strings.ToLower(params["charset"])
		if charset == "" ||
			charset == "utf-8" ||
			charset == "utf8" ||
			charset == "us-ascii" {

			if !utf8.Valid(body) || (charset == "us-ascii" && !isASCIIResponseBody(body)) {
				return decodedResponseBody{}, &api.ToolResultError{Code: "OUTPUT_INVALID", Message: "Provider response body is not valid text."}
			}

			decoded.kind = responseBodyText
			return decoded, nil
		}
		// Unsupported charsets remain opaque bytes instead of being decoded
		// incorrectly or silently replaced by UTF-8 replacement characters.
	}

	switch major {
	case "image":
		decoded.kind = responseBodyImage
	case "audio":
		decoded.kind = responseBodyAudio
	default:
		decoded.kind = responseBodyBinary
	}
	return decoded, nil
}

// mapResponseBody converts a bounded successful response into rich result data.
// The caller supplies a fresh result with validated identities and nonnil Content,
// bounds input/output bytes, and sets execution state on any returned failure.
// JSON bytes are copied exactly; text and encoded binary content own their data.
// Errors contain only safe code/message fields and leave the result unchanged.
// maxEncodedBytes bounds base64 allocation; the caller bounds the final envelope.
func mapResponseBody(
	result *api.ToolResult,
	mediaType string,
	body []byte,
	maxEncodedBytes int,
) *api.ToolResultError {
	decoded, failure := decodeResponseBody(mediaType, body)
	if failure != nil {
		return failure
	}
	switch decoded.kind {
	case responseBodyEmpty:
		return nil
	case responseBodyJSON:
		result.StructuredContent = slices.Clone(decoded.body)
		return nil
	case responseBodyText:
		text := string(decoded.body)
		result.Content = append(result.Content, api.ToolContentBlock{
			Type: api.ToolContentTypeText,
			Text: &text,
		})
		return nil
	}

	if !responseBase64Fits(len(decoded.body), maxEncodedBytes) {
		return &api.ToolResultError{
			Code:    "OUTPUT_TOO_LARGE",
			Message: "Provider response body exceeds the encoded output limit.",
		}
	}

	encoded := base64.StdEncoding.EncodeToString(decoded.body)
	switch decoded.kind {
	case responseBodyImage:
		result.Content = append(result.Content, api.ToolContentBlock{
			Type: api.ToolContentTypeImage,
			Data: &encoded, MIMEType: decoded.mediaType,
		})
	case responseBodyAudio:
		result.Content = append(result.Content, api.ToolContentBlock{
			Type:     api.ToolContentTypeAudio,
			Data:     &encoded,
			MIMEType: decoded.mediaType,
		})
	default:
		// Supported IDs may contain URI-reserved characters. Escape the complete
		// value as one component without changing the captured invocation identity.
		resourceURI := "urn:authproxy:tool-invocation:" + url.PathEscape(result.InvocationID) + ":response"
		result.Content = append(result.Content, api.ToolContentBlock{
			Type: api.ToolContentTypeResource,
			Resource: &api.ToolResourceContent{
				URI:      resourceURI,
				MIMEType: decoded.mediaType,
				Blob:     &encoded,
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

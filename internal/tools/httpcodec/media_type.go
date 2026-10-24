package httpcodec

import (
	"errors"
	"mime"
	"strings"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
)

// parseBodyMediaType validates a concrete Content-Type and returns its lowercase
// base type for codec selection. The caller may retain the original parameters
// when writing the header. ParseMediaType also accepts disposition tokens, so
// type/subtype presence and wildcard rejection need separate checks.
func parseBodyMediaType(value string) (string, error) {
	if !validPartMetadata(value) {
		return "", errors.New("body media type must be valid UTF-8 without control characters")
	}
	base, _, err := mime.ParseMediaType(value)
	major, minor, hasSlash := strings.Cut(base, "/")
	if err != nil || !hasSlash || !tools.IsHTTPToken(major) || !tools.IsHTTPToken(minor) || strings.Contains(base, "*") {
		return "", errors.New("body media type must be a concrete type/subtype")
	}
	return base, nil
}

// validPartMetadata accepts UTF-8 text that is safe to include in MIME headers
// after quoting. All ASCII control bytes, including tabs and DEL, are rejected;
// payload text is validated separately and may contain ordinary newlines.
func validPartMetadata(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < ' ' || value[i] == 0x7f {
			return false
		}
	}
	return true
}

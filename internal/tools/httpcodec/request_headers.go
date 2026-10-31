package httpcodec

import (
	"errors"
	"mime"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"golang.org/x/net/http/httpguts"
)

// requestHeaderNames returns sorted literal names after checking HTTP token
// syntax, case-insensitive uniqueness, and transport-owned header exclusions.
// Framing and hop-by-hop settings cannot be authored as ordinary Tool headers.
func requestHeaderNames(source map[string]string) ([]string, error) {
	names := make([]string, 0, len(source))
	seen := make(map[string]struct{}, len(source))
	for name := range source {
		if !tools.IsHTTPToken(name) {
			return nil, errors.New("request header names must be HTTP tokens")
		}
		canonical := http.CanonicalHeaderKey(name)
		if _, exists := seen[canonical]; exists {
			return nil, errors.New("request header names must be unique ignoring case")
		}
		switch canonical {
		case "Content-Length", "Transfer-Encoding", "Trailer", "Trailers", "Connection", "Keep-Alive",
			"Proxy-Authenticate", "Proxy-Authorization", "Te", "Upgrade":
			return nil, errors.New("request framing and hop-by-hop headers are managed by the transport")
		}
		seen[canonical] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// prepareRequestHeaders validates already-rendered values and returns detached
// canonical headers plus a Host override for http.Request.Host. It reconciles
// the authored Content-Type with the body's encoding and keeps codec-owned
// parameters, especially multipart boundaries. No credentials are applied here;
// the connection proxy retains final header and query credential precedence.
func prepareRequestHeaders(
	source map[string]string, // already rendered headers
	body RenderedBody,
	mode bodyMode,
) (http.Header, string, error) {
	names, err := requestHeaderNames(source)
	if err != nil {
		return nil, "", err
	}

	headers := make(http.Header, len(source)+1)

	var host string
	for _, name := range names {
		value := source[name]

		// Make sure the header is valid
		if !validRequestHeaderValue(value) {
			return nil, "", errors.New("request header values must be valid UTF-8 without prohibited control characters")
		}

		// Special validation for the host header
		if strings.EqualFold(name, "Host") {
			if err := validateRequestAuthority(value); err != nil {
				return nil, "", err
			}
			host = value
			continue
		}
		headers.Set(name, value)
	}

	// Track if content type was set explicitly
	_, authored := headers["Content-Type"]

	// Compute the final content type based on the value supplied, enhancing it
	// with information from the rendered body, etc. Handles things like multipart
	contentType, err := requestContentType(
		headers.Get("Content-Type"),
		authored,
		body.ContentType,
		mode,
	)
	if err != nil {
		return nil, "", err
	}

	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}

	return headers, host, nil
}

// validRequestHeaderValue permits HTTP horizontal tabs and UTF-8 text while
// rejecting CR/LF, DEL, and every other ASCII control character. It does not
// trim values or reinterpret strings that resemble template syntax.
func validRequestHeaderValue(value string) bool {
	return utf8.ValidString(value) && httpguts.ValidHeaderFieldValue(value)
}

// requestContentType selects the request's Content-Type while preserving the
// body's declared parameters and encoder-owned multipart boundary.
//
// authored is the literal header during compilation or its rendered value during
// invocation. It is empty when omitted. present records whether the header was
// supplied, distinguishing omission from an explicitly empty, invalid header.
//
// generated is the body's declared or encoded media type, such as
// "application/json" or "text/plain; charset=utf-8". Multipart compilation uses a
// temporary boundary only to check compatibility; invocation supplies the actual
// boundary matching the encoded body. Raw and absent bodies ignore generated.
//
// mode identifies the body encoding and selects its compatibility rules. Raw and
// absent bodies accept any concrete authored type; structured JSON accepts json
// or +json subtypes; text, form, and multipart retain their generated base type.
//
// Without an authored header, the result is generated, or empty for raw and
// absent bodies. Otherwise it retains generated parameters and compatible
// authored additions. Errors return an empty string for invalid media types or
// modes, incompatible types or parameter values, an authored multipart boundary,
// or a missing generated boundary.
func requestContentType(
	authored string, // The originally set content type
	present bool, // If the content type header existed previously
	generated string, // The content type from the rendered body
	mode bodyMode,
) (string, error) {
	var authoredBase string
	var authoredParams map[string]string

	if present {
		var err error
		authoredBase, err = tools.ParseBodyMediaType(authored)
		if err != nil {
			return "", err
		}

		_, authoredParams, _ = mime.ParseMediaType(authored)
	}

	if mode == bodyAbsent || mode == bodyBinary {
		return authored, nil
	}

	switch mode {
	case bodyJSON, bodyText, bodyForm, bodyMultipart:
	default:
		return "", errors.New("request body mode is invalid")
	}

	generatedBase, err := tools.ParseBodyMediaType(generated)
	if err != nil {
		return "", err
	}

	_, generatedParams, _ := mime.ParseMediaType(generated)
	if mode == bodyMultipart && generatedParams["boundary"] == "" {
		return "", errors.New("multipart content type must contain a generated boundary")
	}

	if !present {
		return generated, nil
	}
	if mode == bodyJSON {
		_, subtype, _ := strings.Cut(authoredBase, "/")
		if subtype != "json" && !strings.HasSuffix(subtype, "+json") {
			return "", errors.New("structured JSON body requires a JSON content type")
		}
	} else if authoredBase != generatedBase {
		return "", errors.New("authored content type conflicts with body encoding")
	}

	if mode == bodyMultipart {
		if _, exists := authoredParams["boundary"]; exists {
			return "", errors.New("multipart boundary must be supplied by the body encoder")
		}
	}

	for name, value := range generatedParams {
		if other, exists := authoredParams[name]; exists && !compatibleMediaParameter(name, value, other) {
			return "", errors.New("authored content type parameters conflict with body encoding")
		}

		// Preserve declared parameter values when an equivalent spelling was
		// authored (for example charset=UTF-8 versus charset=utf-8).
		authoredParams[name] = value
	}

	if len(generatedParams) == 0 {
		return authored, nil
	}

	result := mime.FormatMediaType(authoredBase, authoredParams)
	if result == "" {
		return "", errors.New("request content type parameters cannot be encoded")
	}

	return result, nil
}

// compatibleMediaParameter compares parameter values without changing their
// semantics. Charset names are case-insensitive; other parameters retain exact
// value matching so boundaries, profiles, and custom selectors cannot change.
func compatibleMediaParameter(name, declared, authored string) bool {
	if name == "charset" {
		return strings.EqualFold(declared, authored)
	}
	return declared == authored
}

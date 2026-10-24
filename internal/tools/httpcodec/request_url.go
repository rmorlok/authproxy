package httpcodec

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/http/httpguts"
)

// prepareRequestURL validates a rendered HTTP destination and appends encoded
// structured query fields without re-encoding the original query or path. Keys
// may repeat within either source, but a key present in both is ambiguous and
// rejected. URL userinfo remains supported by the underlying HTTP client.
// Errors never include the destination or query values.
func prepareRequestURL(rawURL, encodedQuery string) (*url.URL, error) {
	if !validRequestURLText(rawURL) || !validRequestURLText(encodedQuery) {
		return nil, errors.New("request URL and query must be valid UTF-8 without unescaped whitespace or control characters")
	}
	if strings.Contains(encodedQuery, "#") {
		return nil, errors.New("structured query must contain valid encoded parameters")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Opaque != "" || strings.Contains(rawURL, "#") {
		return nil, errors.New("request URL must be an absolute HTTP or HTTPS URL without a fragment")
	}
	if err := validateRequestAuthority(parsed.Host); err != nil {
		return nil, err
	}
	baseNames, err := requestQueryNames(parsed.RawQuery)
	if err != nil {
		return nil, errors.New("request URL query must contain valid encoded parameters")
	}
	additionalNames, err := requestQueryNames(encodedQuery)
	if err != nil {
		return nil, errors.New("structured query must contain valid encoded parameters")
	}
	for name := range additionalNames {
		if _, exists := baseNames[name]; exists {
			return nil, errors.New("request URL and structured query must not define the same parameter")
		}
	}
	if encodedQuery != "" {
		if parsed.RawQuery != "" {
			parsed.RawQuery += "&"
		}
		parsed.RawQuery += encodedQuery
	}
	return parsed, nil
}

// requestQueryNames validates percent escapes and returns decoded parameter
// names without treating semicolons as form separators or rejecting them as
// data. Only ampersands separate fields, and the first equals sign separates a
// name from its value. The original query remains untouched for transmission.
func requestQueryNames(query string) (map[string]struct{}, error) {
	names := make(map[string]struct{})
	for _, field := range strings.Split(query, "&") {
		if field == "" {
			continue
		}
		rawName, value, _ := strings.Cut(field, "=")
		name, err := url.QueryUnescape(rawName)
		if err != nil {
			return nil, errors.New("query parameter names must contain valid percent escapes")
		}
		if _, err := url.QueryUnescape(value); err != nil {
			return nil, errors.New("query parameter values must contain valid percent escapes")
		}
		names[name] = struct{}{}
	}
	return names, nil
}

// validRequestURLText accepts Unicode URL text while requiring ASCII whitespace
// and controls to be percent-encoded. In particular, a raw query space cannot
// survive into the HTTP request target as an extra request-line separator.
func validRequestURLText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] <= ' ' || value[i] == 0x7f {
			return false
		}
	}
	return true
}

// validateRequestAuthority accepts a nonempty registered host name or bracketed
// IPv6 address, with an optional decimal port from zero through 65535. IPv6 zone
// identifiers are supported. It validates syntax without DNS resolution or a
// hostname allowlist, and excludes userinfo, paths, and other URL components.
func validateRequestAuthority(authority string) error {
	if authority == "" || !utf8.ValidString(authority) || strings.ContainsAny(authority, "/\\?#@") {
		return errors.New("request host must be a valid authority")
	}
	for _, char := range authority {
		if char < ' ' || char == 0x7f || unicode.IsSpace(char) {
			return errors.New("request host must be a valid authority")
		}
	}
	// Match net/http's IDN and Host-header checks before retaining the
	// original spelling. Converting for validation does not rewrite routing.
	ascii, err := httpguts.PunycodeHostPort(authority)
	if err != nil || !httpguts.ValidHostHeader(ascii) {
		return errors.New("request host must be a valid HTTP authority")
	}

	var port string
	hasPort := false
	if strings.HasPrefix(authority, "[") {
		closeBracket := strings.IndexByte(authority, ']')
		if closeBracket < 0 {
			return errors.New("request host must contain a valid bracketed IPv6 address")
		}
		address, err := netip.ParseAddr(authority[1:closeBracket])
		if err != nil || !address.Is6() {
			return errors.New("request host must contain a valid bracketed IPv6 address")
		}
		tail := authority[closeBracket+1:]
		if tail != "" {
			if !strings.HasPrefix(tail, ":") {
				return errors.New("request host must be a valid authority")
			}
			hasPort, port = true, tail[1:]
		}
	} else {
		if strings.ContainsAny(authority, "[]%") || strings.Count(authority, ":") > 1 {
			return errors.New("request host must be a valid authority")
		}
		var host string
		host, port, hasPort = strings.Cut(authority, ":")
		if host == "" {
			return errors.New("request host must include a host name")
		}
		// net/url already checks destination hosts. Apply its same registered
		// name syntax to a Host override without changing or normalizing it.
		parsed, err := url.Parse("http://" + host)
		if err != nil || parsed.Host != host || parsed.User != nil || parsed.Path != "" {
			return errors.New("request host must be a valid authority")
		}
	}
	if hasPort {
		if port == "" {
			return errors.New("request host port must be a decimal value from zero through 65535")
		}
		for _, char := range port {
			if char < '0' || char > '9' {
				return errors.New("request host port must be a decimal value from zero through 65535")
			}
		}
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return errors.New("request host port must be a decimal value from zero through 65535")
		}
	}
	return nil
}

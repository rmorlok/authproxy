package httpcodec

import (
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRequestHeaderNames checks literal sorted names and rejection of ambiguous
// casing, invalid HTTP syntax, and transport-owned framing/hop settings.
func TestRequestHeaderNames(t *testing.T) {
	// Header names are literal tokens and keep their authored spelling for lookup.
	names, err := requestHeaderNames(map[string]string{"z-test": "1", "Authorization": "2", "$value": "3"})
	require.NoError(t, err)
	require.Equal(t, []string{"$value", "Authorization", "z-test"}, names)

	// Canonicalization cannot silently select one conflicting map entry.
	for _, source := range []map[string]string{{"X-Test": "1", "x-test": "2"}, {"": "private-config"}, {"private-config\r\n": "1"}, {"Bad Name": "1"}, {"{{cfg.header}}": "1"}, {"é": "1"}} {
		names, err := requestHeaderNames(source)
		require.Error(t, err)
		require.Nil(t, names)
		require.NotContains(t, err.Error(), "private-config")
	}

	// Reject both standard Trailer and the legacy plural spelling, in any case.
	for _, name := range []string{"Content-Length", "Transfer-Encoding", "Trailer", "Trailers", "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Upgrade"} {
		names, err := requestHeaderNames(map[string]string{strings.ToLower(name): "private-config"})
		require.ErrorContains(t, err, "transport")
		require.Nil(t, names)
	}
}

// TestPrepareRequestHeadersPreservesValuesAndSeparatesHost verifies ordinary
// values and credential candidates survive while Host uses its dedicated field.
func TestPrepareRequestHeadersPreservesValuesAndSeparatesHost(t *testing.T) {
	// Values are inert data; the connection proxy later applies credentials.
	source := map[string]string{"host": "bücher.example:8443", "authorization": "Bearer authored", "x-value": "  a\tb ☃ {{cfg.literal}}  ", "X-Empty": ""}
	headers, host, err := prepareRequestHeaders(source, RenderedBody{}, bodyAbsent)
	require.NoError(t, err)
	require.Equal(t, "bücher.example:8443", host)
	require.NotContains(t, headers, "Host")
	require.Equal(t, "Bearer authored", headers.Get("Authorization"))
	require.Equal(t, source["x-value"], headers.Get("X-Value"))
	require.Equal(t, []string{""}, headers["X-Empty"])

	// A Host override changes the HTTP authority, not the network destination.
	request, err := http.NewRequest(http.MethodGet, "https://network.example/path", nil)
	require.NoError(t, err)
	request.Header, request.Host = headers, host
	require.Equal(t, "network.example", request.URL.Host)
	require.Equal(t, host, request.Host)

	// Returned header storage is independent of the caller's source map.
	headers.Set("X-Value", "changed")
	require.Equal(t, "  a\tb ☃ {{cfg.literal}}  ", source["x-value"])
}

// TestPrepareRequestHeadersRejectsInvalidValues fails the complete preparation
// when a rendered value contains invalid text, header controls, or Host syntax.
func TestPrepareRequestHeadersRejectsInvalidValues(t *testing.T) {
	for _, source := range []map[string]string{
		{"X-Value": "private-config\r\nInjected: yes"}, {"X-Value": "private-config\x00"}, {"X-Value": "private-config\x1f"}, {"X-Value": "private-config\x7f"},
		{"X-Value": string([]byte{0xff})}, {"Host": ""}, {"Host": "private-config/path"}, {"Host": "user@private-config"},
		{"Host": "private-config:65536"}, {"Content-Type": ""}, {"Content-Type": "private-config"}, {"Content-Type": "text/*"},
	} {
		// Include an earlier valid value to ensure no partial header map escapes.
		source["A-Valid"] = "okay"
		headers, host, err := prepareRequestHeaders(source, RenderedBody{}, bodyAbsent)
		require.Error(t, err)
		require.Nil(t, headers)
		require.Empty(t, host)
		require.NotContains(t, err.Error(), "private-config")
	}
}

// TestPrepareRequestHeadersContentTypes exercises defaults, JSON-compatible
// authored types, literal declared parameters, and multipart boundary ownership.
func TestPrepareRequestHeadersContentTypes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       bodyMode
		generated  string
		authored   string
		wantBase   string
		wantParams map[string]string
	}{
		{"absent", bodyAbsent, "", "", "", nil},
		{"raw authored", bodyBinary, "", "image/png", "image/png", map[string]string{}},
		{"absent authored", bodyAbsent, "", "text/plain; charset=utf-8", "text/plain", map[string]string{"charset": "utf-8"}},
		{"JSON default", bodyJSON, "application/json", "", "application/json", map[string]string{}},
		{"JSON vendor", bodyJSON, "application/json", "Application/Vnd.Example+JSON; profile=custom", "application/vnd.example+json", map[string]string{"profile": "custom"}},
		{"form extra parameter", bodyForm, "application/x-www-form-urlencoded", "application/x-www-form-urlencoded; charset=utf-8", "application/x-www-form-urlencoded", map[string]string{"charset": "utf-8"}},
		{"text declared default", bodyText, "text/plain; charset=utf-8", "", "text/plain", map[string]string{"charset": "utf-8"}},
		{"text merged", bodyText, `text/plain; charset=utf-8; note="{{cfg.literal}}"`, "TEXT/PLAIN; charset=UTF-8; level=1", "text/plain", map[string]string{"charset": "utf-8", "note": "{{cfg.literal}}", "level": "1"}},
		{"multipart default", bodyMultipart, "multipart/form-data; boundary=generated", "", "multipart/form-data", map[string]string{"boundary": "generated"}},
		{"multipart merged", bodyMultipart, "multipart/form-data; boundary=generated", "multipart/form-data; charset=utf-8", "multipart/form-data", map[string]string{"boundary": "generated", "charset": "utf-8"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The helper combines existing metadata without template evaluation.
			source := map[string]string{}
			if tc.authored != "" {
				source["content-type"] = tc.authored
			}
			headers, host, err := prepareRequestHeaders(source, RenderedBody{ContentType: tc.generated}, tc.mode)
			require.NoError(t, err)
			require.Empty(t, host)
			if tc.wantBase == "" {
				require.Empty(t, headers.Get("Content-Type"))
				return
			}
			base, params, err := mime.ParseMediaType(headers.Get("Content-Type"))
			require.NoError(t, err)
			require.Equal(t, tc.wantBase, base)
			require.Equal(t, tc.wantParams, params)
		})
	}
}

// TestPrepareRequestHeadersContentTypeConflicts rejects media types that would
// misdescribe encoded bytes or override declared parameters and MIME boundaries.
func TestPrepareRequestHeadersContentTypeConflicts(t *testing.T) {
	for _, tc := range []struct {
		mode      bodyMode
		generated string
		authored  string
	}{
		{bodyJSON, "application/json", "text/plain"},
		{bodyForm, "application/x-www-form-urlencoded", "application/json"},
		{bodyText, "application/json", "application/problem+json"},
		{bodyText, "text/plain; charset=utf-8", "text/plain; charset=iso-8859-1"},
		{bodyText, "text/plain; profile=Exact", "text/plain; profile=exact"},
		{bodyMultipart, "multipart/form-data; boundary=generated", "multipart/form-data; boundary=generated"},
		{bodyMultipart, "multipart/form-data; boundary=generated", "multipart/form-data; boundary=private-config"},
		{bodyMultipart, "multipart/form-data; boundary=generated", "multipart/form-data; boundary*=UTF-8''private-config"},
		{bodyMultipart, "multipart/form-data", "multipart/form-data"},
		{bodyText, "private-config", "text/plain"},
		{bodyMode("invalid"), "", "text/plain"},
	} {
		// Neither headers nor a Host override escapes when content types conflict.
		headers, host, err := prepareRequestHeaders(map[string]string{"Host": "example.test", "Content-Type": tc.authored}, RenderedBody{ContentType: tc.generated}, tc.mode)
		require.Error(t, err)
		require.Nil(t, headers)
		require.Empty(t, host)
		require.NotContains(t, err.Error(), "private-config")
	}
}

package httpcodec

import (
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPrepareRequestURLPreservesEncoding checks that request assembly appends
// fields while preserving literal URL spelling and repeated-value order.
func TestPrepareRequestURLPreservesEncoding(t *testing.T) {
	// Neither the escaped path segment nor existing query spelling is rewritten.
	prepared, err := prepareRequestURL("https://example.test/a%2fb?z=one%20two&z=three+four&token=%2f", "tag=first&tag=second&active=false")
	require.NoError(t, err)
	require.Equal(t, "/a%2fb", prepared.RawPath)
	require.Equal(t, "z=one%20two&z=three+four&token=%2f&tag=first&tag=second&active=false", prepared.RawQuery)
	require.Equal(t, "https://example.test/a%2fb?z=one%20two&z=three+four&token=%2f&tag=first&tag=second&active=false", prepared.String())
	require.Equal(t, []string{"first", "second"}, prepared.Query()["tag"])

	// An omitted optional field emits nothing and therefore causes no collision.
	prepared, err = prepareRequestURL("https://example.test/?cursor=existing", "")
	require.NoError(t, err)
	require.Equal(t, "cursor=existing", prepared.RawQuery)

	// An explicit trailing query marker remains present without added fields.
	prepared, err = prepareRequestURL("https://example.test/path?", "")
	require.NoError(t, err)
	require.True(t, prepared.ForceQuery)
	require.Equal(t, "https://example.test/path?", prepared.String())
}

// TestPrepareRequestURLPreservesSemicolons treats semicolons as ordinary query
// data while retaining ampersand-delimited fields and decoded-name collisions.
func TestPrepareRequestURLPreservesSemicolons(t *testing.T) {
	// Semicolons in names and values survive alongside newly encoded fields.
	prepared, err := prepareRequestURL("https://example.test/?q=a;b&semi;name=one=two", "next=x;y&tag=a%3Bb")
	require.NoError(t, err)
	require.Equal(t, "q=a;b&semi;name=one=two&next=x;y&tag=a%3Bb", prepared.RawQuery)
	require.Equal(t, "https://example.test/?q=a;b&semi;name=one=two&next=x;y&tag=a%3Bb", prepared.String())

	// A percent-encoded spelling still identifies the same parameter name.
	prepared, err = prepareRequestURL("https://example.test/?semi;name=private-config", "semi%3Bname=other")
	require.ErrorContains(t, err, "same parameter")
	require.Nil(t, prepared)
	require.NotContains(t, err.Error(), "private-config")
}

// TestPrepareRequestURLSupportedDestinations covers HTTP client capabilities
// without DNS lookup, including IDNs, userinfo, IPv6 zones, and port limits.
func TestPrepareRequestURLSupportedDestinations(t *testing.T) {
	for _, raw := range []string{
		"http://localhost/path", "https://example.test:443/path", "HTTP://example.test/path",
		"http://127.0.0.1:8080/", "http://example.test:0/", "http://example.test:00080/", "http://example.test:65535/",
		"https://[2001:db8::1]:8443/", "http://[::ffff:192.0.2.1]/", "http://[fe80::1%25en0]:8080/",
		"https://bücher.example/straße", "https://service_name.example/path", "https://example.test./path",
		"https://user:private-config@example.test/path", "https://us%40er:p%3Ass@example.test/path",
		"https://example.test/a%20b?q=one%20two", "https://example.test/a%23b?q=%23fragment",
	} {
		t.Run(raw, func(t *testing.T) {
			// Successful validation leaves a request net/http can construct.
			prepared, err := prepareRequestURL(raw, "")
			require.NoError(t, err)
			request, err := http.NewRequest(http.MethodGet, prepared.String(), nil)
			require.NoError(t, err)
			require.Equal(t, prepared.Host, request.URL.Host)
		})
	}

	// URL userinfo remains intact for the proxy's existing HTTP-client behavior.
	prepared, err := prepareRequestURL("https://us%40er:p%3Ass@example.test/path", "")
	require.NoError(t, err)
	require.Equal(t, "us@er", prepared.User.Username())
	password, present := prepared.User.Password()
	require.True(t, present)
	require.Equal(t, "p:ss", password)
}

// TestPrepareRequestURLRejectsInvalidInput checks final URL and query failures
// without returning a partial destination or disclosing resolved source values.
func TestPrepareRequestURLRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		url   string
		query string
	}{
		{"empty", "", ""},
		{"relative", "/private-config", ""},
		{"scheme relative", "//private-config/path", ""},
		{"other scheme", "ftp://private-config/path", ""},
		{"opaque", "https:private-config", ""},
		{"no hostname", "https://:443/private-config", ""},
		{"no authority", "https:///private-config", ""},
		{"fragment", "https://example.test/#private-config", ""},
		{"empty fragment", "https://example.test/private-config#", ""},
		{"bad path escape", "https://example.test/private-config%xx", ""},
		{"bad query escape", "https://example.test/?private-config=%xx", ""},
		{"bad structured escape", "https://example.test/", "private-config=%xx"},
		{"bad semicolon value escape", "https://example.test/?x=private-config;%xx", ""},
		{"bad semicolon name escape", "https://example.test/?private-config;%xx=value", ""},
		{"bad structured semicolon escape", "https://example.test/", "x=private-config;%xx"},
		{"structured fragment", "https://example.test/", "x=private-config#fragment"},
		{"query collision", "https://example.test/?x=private-config", "x=other"},
		{"encoded key collision", "https://example.test/?%78=private-config", "x=other"},
		{"repeated key collision", "https://example.test/?x=1&x=private-config", "x=other&x=again"},
		{"raw space", "https://example.test/?x=private-config value", ""},
		{"raw newline", "https://example.test/private-config\n", ""},
		{"raw DEL", "https://example.test/private-config\x7f", ""},
		{"invalid UTF-8", "https://example.test/" + string([]byte{0xff}), ""},
		{"invalid structured UTF-8", "https://example.test/", string([]byte{0xff})},
		{"bad bracket IP", "https://[private-config]/", ""},
		{"bracket IPv4", "https://[127.0.0.1]/", ""},
		{"unbracketed IPv6", "https://2001:db8::1/", ""},
		{"empty port", "https://private-config:/", ""},
		{"large port", "https://private-config:65536/", ""},
		{"nonnumeric port", "https://private-config:port/", ""},
		{"negative port", "https://private-config:-1/", ""},
		{"host backslash", "https://private-config\\example/", ""},
		{"host bad escape", "https://private-config%25example/", ""},
		{"host forbidden byte", "https://private-config|example/", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Invalid destinations fail before a caller receives any request URL.
			prepared, err := prepareRequestURL(tc.url, tc.query)
			require.Error(t, err)
			require.Nil(t, prepared)
			require.NotContains(t, err.Error(), "private-config")
		})
	}
}

// TestValidateRequestAuthority checks Host overrides cannot smuggle another URL
// component while preserving the HTTP client's ordinary authority forms.
func TestValidateRequestAuthority(t *testing.T) {
	// Host override input has no URL parser in front of it, so validate directly.
	for _, authority := range []string{"api.example", "api.example:443", "127.0.0.1:0", "[::1]", "[fe80::1%en0]:8080", "bücher.example:443", "service_name.example"} {
		require.NoError(t, validateRequestAuthority(authority), authority)
	}
	for _, authority := range []string{"", "private-config/path", "private-config?query", "private-config#fragment", "user@private-config", "private-config:secret@example.test", "private-config\\path", "private-config ", "private-config\t", "private-config\r\nX-Test: yes", "private-config\x00", "private-config\x7f", "private-config\u00a0", "private-config|example", "private-config%20example", "[::1]junk", "[::1]:", "[::1]:65536", "[::1", "::1", "[127.0.0.1]", string([]byte{0xff})} {
		err := validateRequestAuthority(authority)
		require.Error(t, err, authority)
		require.NotContains(t, err.Error(), "private-config")
	}
}

// FuzzPrepareRequestURL ensures every accepted destination remains parseable
// after appending query fields, and serialized URLs cannot gain a fragment.
func FuzzPrepareRequestURL(f *testing.F) {
	f.Add("https://example.test/a%2fb?keep=a%20b", "tag=x&tag=y")
	f.Add("https://user:password@[::1]:443/path", "")
	f.Add("https://example.test/?x=1", "x=2")
	f.Add("https://example.test/?q=a;b&semi;name=one=two", "next=x;y")
	f.Fuzz(func(t *testing.T, rawURL, query string) {
		// Bound fuzz input like the request compiler's source admission budget.
		if len(rawURL)+len(query) > 8192 {
			t.Skip()
		}
		prepared, err := prepareRequestURL(rawURL, query)
		if err != nil {
			require.Nil(t, prepared)
			return
		}
		// Re-parsing cannot change query bytes or introduce a fragment.
		reparsed, err := url.Parse(prepared.String())
		require.NoError(t, err)
		require.Empty(t, reparsed.Fragment)
		require.Equal(t, prepared.RawQuery, reparsed.RawQuery)
		request, err := http.NewRequest(http.MethodGet, prepared.String(), nil)
		require.NoError(t, err)
		require.NoError(t, request.Write(io.Discard))
	})
}

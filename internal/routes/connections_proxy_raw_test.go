package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeReq builds a synthetic *http.Request carrying the supplied
// envelope headers. The body is left empty — these tests only exercise
// header parsing.
func makeReq(t *testing.T, headers map[string][]string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "https://api.local/connections/cxn_1/_proxy_raw", strings.NewReader(""))
	for k, vv := range headers {
		for _, v := range vv {
			r.Header.Add(k, v)
		}
	}
	return r
}

func TestParseRawProxyEnvelope_RequiresUpstreamURL(t *testing.T) {
	_, err := parseRawProxyEnvelope(makeReq(t, nil))
	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadRequest, err.Status)
	assert.Contains(t, err.ResponseMsg, HeaderUpstreamURL)
}

func TestParseRawProxyEnvelope_RejectsRelativeURL(t *testing.T) {
	_, err := parseRawProxyEnvelope(makeReq(t, map[string][]string{
		HeaderUpstreamURL: {"/just/a/path"},
	}))
	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadRequest, err.Status)
}

func TestParseRawProxyEnvelope_RejectsNonHTTPScheme(t *testing.T) {
	for _, u := range []string{"ftp://example.com/x", "ws://example.com/x", "file:///etc/passwd"} {
		t.Run(u, func(t *testing.T) {
			_, err := parseRawProxyEnvelope(makeReq(t, map[string][]string{
				HeaderUpstreamURL: {u},
			}))
			require.NotNil(t, err)
			assert.Equal(t, http.StatusBadRequest, err.Status)
		})
	}
}

func TestParseRawProxyEnvelope_AcceptsHTTPAndHTTPS(t *testing.T) {
	for _, u := range []string{"http://api.example.com/v1/x", "https://api.example.com/v1/x?q=1"} {
		t.Run(u, func(t *testing.T) {
			parsed, err := parseRawProxyEnvelope(makeReq(t, map[string][]string{
				HeaderUpstreamURL: {u},
			}))
			require.Nil(t, err)
			assert.Equal(t, u, parsed.upstreamURL.String())
		})
	}
}

// TestParseLabelHeaders_RoundTripsKeysContainingSlash is load-bearing:
// the whole reason we use a repeated header rather than a single
// comma-joined value is so label keys can contain `/` and other
// characters that aren't safe inside a comma-delimited list. If this
// regresses, namespace-scoped labels (org/key) silently lose their
// separator.
func TestParseLabelHeaders_RoundTripsKeysContainingSlash(t *testing.T) {
	labels, err := parseLabelHeaders([]string{
		"customer=acme",
		"team/region=us-east",
		"org/project/env=prod",
	})
	require.Nil(t, err)
	assert.Equal(t, map[string]string{
		"customer":        "acme",
		"team/region":     "us-east",
		"org/project/env": "prod",
	}, labels)
}

func TestParseLabelHeaders_SplitsOnFirstEquals(t *testing.T) {
	labels, err := parseLabelHeaders([]string{"key=a=b=c"})
	require.Nil(t, err)
	assert.Equal(t, "a=b=c", labels["key"])
}

func TestParseLabelHeaders_EmptyValueAllowed(t *testing.T) {
	labels, err := parseLabelHeaders([]string{"k="})
	require.Nil(t, err)
	assert.Equal(t, "", labels["k"])
}

func TestParseLabelHeaders_RejectsMissingSeparator(t *testing.T) {
	_, err := parseLabelHeaders([]string{"keyonly"})
	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadRequest, err.Status)
}

func TestParseLabelHeaders_RejectsLeadingEquals(t *testing.T) {
	_, err := parseLabelHeaders([]string{"=v"})
	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadRequest, err.Status)
}

func TestParseLabelHeaders_SkipsEmptyEntries(t *testing.T) {
	labels, err := parseLabelHeaders([]string{"", "  ", "k=v"})
	require.Nil(t, err)
	assert.Equal(t, map[string]string{"k": "v"}, labels)
}

func TestParseLabelHeaders_Nil(t *testing.T) {
	labels, err := parseLabelHeaders(nil)
	require.Nil(t, err)
	assert.Nil(t, labels)
}

// TestCopyInboundHeadersForRawProxy ensures Authorization, the envelope
// headers, and the RFC 7230 hop-by-hop set are stripped while a generic
// caller header (Content-Type, Accept) passes through.
func TestCopyInboundHeadersForRawProxy(t *testing.T) {
	src := http.Header{}
	src.Set("Authorization", "Bearer secret")
	src.Set(HeaderUpstreamURL, "https://api.example.com/x")
	src.Add(HeaderLabel, "k=v")
	src.Set("Content-Type", "application/json")
	src.Set("Accept", "text/event-stream")
	src.Set("Connection", "X-Custom-Hop")
	src.Set("X-Custom-Hop", "should-be-stripped")
	src.Set("Transfer-Encoding", "chunked")
	src.Set("Host", "api.local")
	src.Set("Content-Length", "42")
	src.Set("X-Customer-Trace", "trace-1")

	dst := http.Header{}
	copyInboundHeadersForRawProxy(dst, src)

	assert.Equal(t, "application/json", dst.Get("Content-Type"), "passthrough header preserved")
	assert.Equal(t, "text/event-stream", dst.Get("Accept"), "passthrough header preserved")
	assert.Equal(t, "trace-1", dst.Get("X-Customer-Trace"), "arbitrary caller header preserved")

	assert.Empty(t, dst.Get("Authorization"), "Authorization must be stripped — connector replaces it")
	assert.Empty(t, dst.Get(HeaderUpstreamURL), "envelope header must not leak to upstream")
	assert.Empty(t, dst.Get(HeaderLabel), "envelope header must not leak to upstream")
	assert.Empty(t, dst.Get("Connection"), "hop-by-hop stripped")
	assert.Empty(t, dst.Get("X-Custom-Hop"), "header named by Connection: ... stripped")
	assert.Empty(t, dst.Get("Transfer-Encoding"), "hop-by-hop stripped")
	assert.Empty(t, dst.Get("Host"), "Host set by http.NewRequest from outbound URL")
	assert.Empty(t, dst.Get("Content-Length"), "Content-Length set separately from inbound ContentLength")
}

func TestCopyInboundHeadersForRawProxy_PreservesRepeatedValues(t *testing.T) {
	src := http.Header{}
	src.Add("X-Multi", "one")
	src.Add("X-Multi", "two")

	dst := http.Header{}
	copyInboundHeadersForRawProxy(dst, src)

	assert.Equal(t, []string{"one", "two"}, dst.Values("X-Multi"))
}

package main

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSigner returns a jwt.Signer that stamps a fixed bearer token —
// good enough to assert that signing happens, without setting up the
// real key plumbing.
func stubSigner(token string) jwt.Signer { return jwt.NewSigner(token) }

// findURLArg has to pick the URL out of an arbitrary curl argv. Three
// forms matter: positional URL, --url <value>, --url=<value>. Anything
// that doesn't parse as an http(s) URL is ignored — flags whose values
// happen to look URL-ish (e.g. --data @body) must not be picked up.
func TestFindURLArg(t *testing.T) {
	t.Run("positional URL", func(t *testing.T) {
		idx, u, err := findURLArg([]string{"-X", "POST", "https://api.example.com/x", "-d", "{}"})
		require.NoError(t, err)
		assert.Equal(t, 2, idx)
		assert.Equal(t, "https://api.example.com/x", u.String())
	})
	t.Run("--url separated value", func(t *testing.T) {
		idx, u, err := findURLArg([]string{"--url", "https://api.example.com/y"})
		require.NoError(t, err)
		assert.Equal(t, 1, idx)
		assert.Equal(t, "https://api.example.com/y", u.String())
	})
	t.Run("--url=value attached", func(t *testing.T) {
		idx, u, err := findURLArg([]string{"-X", "GET", "--url=https://api.example.com/z"})
		require.NoError(t, err)
		assert.Equal(t, 2, idx)
		assert.Equal(t, "https://api.example.com/z", u.String())
	})
	t.Run("missing URL", func(t *testing.T) {
		_, _, err := findURLArg([]string{"-X", "POST", "-d", "@body"})
		require.Error(t, err)
	})
	t.Run("relative path is not a URL", func(t *testing.T) {
		_, _, err := findURLArg([]string{"/just/a/path"})
		require.Error(t, err)
	})
	t.Run("file URL is not picked up", func(t *testing.T) {
		_, _, err := findURLArg([]string{"file:///etc/hosts"})
		require.Error(t, err)
	})
}

// TestProxyCmd_InterspersedFalse_PreservesCurlArgs proves that cobra's
// SetInterspersed(false) keeps curl's own flags out of cobra's flag
// parser, so `ap proxy --connection cxn_x curl -X POST https://...`
// lands `["curl", "-X", "POST", "https://..."]` in RunE without
// cobra trying to claim -X.
func TestProxyCmd_InterspersedFalse_PreservesCurlArgs(t *testing.T) {
	var ran []string
	cmd := cmdProxy()
	// Swap in a RunE that just captures args so we don't try to
	// resolve a real signer or open a socket.
	cmd.RunE = func(c *cobra.Command, args []string) error { ran = args; return nil }

	cmd.SetArgs([]string{"--connection", "cxn_x", "curl", "-X", "POST", "https://api.example.com/v1/x", "--data", "@body.json"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{"curl", "-X", "POST", "https://api.example.com/v1/x", "--data", "@body.json"}, ran)
}

// TestProxyCmd_NoPositional_RunsListenerMode confirms the default
// listener path: zero positional args, just the configured flags.
func TestProxyCmd_NoPositional_RunsListenerMode(t *testing.T) {
	var ran []string
	cmd := cmdProxy()
	cmd.RunE = func(c *cobra.Command, args []string) error { ran = args; return nil }
	cmd.SetArgs([]string{"--connection", "cxn_x", "--upstream-base", "https://api.example.com"})
	require.NoError(t, cmd.Execute())
	assert.Empty(t, ran, "no positional args means listener mode")
}

// cmdRawProxyAlias must reuse cmdSigningProxy's flag surface so the old
// command keeps working byte-for-byte. The deprecation warning lives in
// RunE — verified by inspection rather than test since it goes to
// stderr.
func TestRawProxyAlias_KeepsSamePrimaryFlags(t *testing.T) {
	signing := cmdSigningProxy()
	alias := cmdRawProxyAlias()

	assert.Equal(t, "raw-proxy", alias.Use)
	assert.True(t, alias.Hidden, "alias must be hidden from help")
	for _, name := range []string{"proxyTo", "enableLoginRedirect", "port", "ip", "proto"} {
		assert.NotNil(t, signing.Flag(name), "signing-proxy must have flag %s", name)
		assert.NotNil(t, alias.Flag(name), "raw-proxy alias must have flag %s", name)
	}
}

// TestRawProxyHandler_DerivesUpstreamFromBase covers the common path —
// caller hits /v1/foo, --upstream-base is https://upstream/, handler
// sets X-AuthProxy-Upstream-URL to https://upstream/v1/foo and signs
// the outbound request before forwarding to the AuthProxy server.
func TestRawProxyHandler_DerivesUpstreamFromBase(t *testing.T) {
	// Fake AuthProxy /_proxy_raw server. The handler's outbound request
	// lands here; we assert the envelope and echo a small body back.
	authproxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/connections/cxn_test/_proxy_raw", r.URL.Path)
		assert.Equal(t, "https://upstream.example.com/v1/foo?q=1", r.Header.Get("X-AuthProxy-Upstream-URL"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "ap-curl-test", r.Header.Get("X-Caller"))
		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, "hello", string(body))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer authproxy.Close()

	base, _ := url.Parse("https://upstream.example.com/")
	signer := stubSigner("test-token")
	h := rawProxyHandler(authproxy.URL, "cxn_test", base, signer)

	req := httptest.NewRequest(http.MethodPost, "/v1/foo?q=1", strings.NewReader("hello"))
	req.Header.Set("X-Caller", "ap-curl-test")
	rr := httptest.NewRecorder()
	h(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Equal(t, `{"ok":true}`, rr.Body.String())
}

// TestRawProxyHandler_HeaderSuppliedUpstream covers the case where no
// --upstream-base is configured: the caller carries the envelope
// header through unchanged.
func TestRawProxyHandler_HeaderSuppliedUpstream(t *testing.T) {
	gotUpstream := ""
	authproxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUpstream = r.Header.Get("X-AuthProxy-Upstream-URL")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer authproxy.Close()

	h := rawProxyHandler(authproxy.URL, "cxn_abc", nil, stubSigner("t"))
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("X-AuthProxy-Upstream-URL", "https://other.example.com/explicit")
	rr := httptest.NewRecorder()
	h(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Equal(t, "https://other.example.com/explicit", gotUpstream)
}

// TestRawProxyHandler_MissingUpstreamReturns502 covers operator error:
// no --upstream-base, no caller-supplied header. The handler refuses
// rather than guessing.
func TestRawProxyHandler_MissingUpstreamReturns502(t *testing.T) {
	h := rawProxyHandler("http://does-not-matter", "cxn_abc", nil, stubSigner("t"))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	assert.Equal(t, http.StatusBadGateway, rr.Code)
}

// TestRawProxyHandler_StreamsChunkedResponse proves the SSE-shaped path:
// upstream emits tokens with explicit flushes, each one must reach the
// client before the next is emitted. If the proxy buffered the full
// response the loop would deadlock.
func TestRawProxyHandler_StreamsChunkedResponse(t *testing.T) {
	tokens := []string{"data: a\n\n", "data: b\n\n", "data: c\n\n"}

	authproxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		for _, tok := range tokens {
			_, _ = w.Write([]byte(tok))
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer authproxy.Close()

	base, _ := url.Parse("https://upstream.example.com")
	h := rawProxyHandler(authproxy.URL, "cxn_sse", base, stubSigner("t"))

	// Serve the handler on a real socket so we can drive a real
	// streaming client against it.
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/stream")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// Read each token with a tight deadline. If the response were
	// buffered we'd block until upstream closed.
	reader := bufio.NewReader(resp.Body)
	for i := range tokens {
		deadline := time.Now().Add(2 * time.Second)
		// Pull "data: x\n\n" — two lines.
		for j := 0; j < 2; j++ {
			done := make(chan struct {
				line string
				err  error
			}, 1)
			go func() {
				l, err := reader.ReadString('\n')
				done <- struct {
					line string
					err  error
				}{l, err}
			}()
			select {
			case r := <-done:
				require.NoErrorf(t, r.err, "token %d line %d", i, j)
			case <-time.After(time.Until(deadline)):
				t.Fatalf("token %d line %d: stream stalled — handler is buffering", i, j)
			}
		}
	}
}

// TestRawProxyHandler_StripsHopByHopHeaders ensures we don't forward
// hop-by-hop framing across the proxy hop (RFC 7230 §6.1).
func TestRawProxyHandler_StripsHopByHopHeaders(t *testing.T) {
	got := http.Header{}
	authproxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer authproxy.Close()

	h := rawProxyHandler(authproxy.URL, "cxn_x", nil, stubSigner("t"))
	req := httptest.NewRequest(http.MethodGet, "/y", nil)
	req.Header.Set("X-AuthProxy-Upstream-URL", "https://upstream.example.com/y")
	req.Header.Set("Connection", "close")
	req.Header.Set("Keep-Alive", "timeout=5")
	req.Header.Set("X-Pass-Through", "yes")
	rr := httptest.NewRecorder()
	h(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, got.Get("Keep-Alive"), "Keep-Alive must be stripped")
	assert.Equal(t, "yes", got.Get("X-Pass-Through"))
}

package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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

// Same boundary check, but with the `wget` discriminator. wget's flag
// surface is smaller than curl's but still has things like -O, -q, and
// --user-agent that must reach RunE intact.
func TestProxyCmd_InterspersedFalse_PreservesWgetArgs(t *testing.T) {
	var ran []string
	cmd := cmdProxy()
	cmd.RunE = func(c *cobra.Command, args []string) error { ran = args; return nil }

	cmd.SetArgs([]string{"--connection", "cxn_x", "wget", "-O", "out.bin", "https://api.example.com/v1/big", "--user-agent", "ap"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{"wget", "-O", "out.bin", "https://api.example.com/v1/big", "--user-agent", "ap"}, ran)
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

// TestRawProxyHandler_DerivesUpstreamFromBase covers the common path —
// caller hits /v1/foo, --upstream-base is https://upstream/, handler
// sets X-AuthProxy-Upstream-URL to https://upstream/v1/foo and signs
// the outbound request before forwarding to the AuthProxy server.
func TestRawProxyHandler_DerivesUpstreamFromBase(t *testing.T) {
	// Fake AuthProxy /_proxyRaw server. The handler's outbound request
	// lands here; we assert the envelope and echo a small body back.
	authproxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/connections/cxn_test/_proxyRaw", r.URL.Path)
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

// TestRawProxyHandler_ResponseBeforeUploadCompletes checks the CLI's HTTP/1 hop
// forwards early evidence headers without draining or closing a pending upload.
// Keeping the 64 KiB upload below net/http's automatic drain limit makes a
// missing EnableFullDuplex call block on the deliberately withheld suffix.
func TestRawProxyHandler_ResponseBeforeUploadCompletes(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		name := "known length"
		if chunked {
			name = "chunked"
		}
		t.Run(name, func(t *testing.T) {
			assertRawProxyResponseBeforeUploadCompletes(t, chunked)
		})
	}
}

// assertRawProxyResponseBeforeUploadCompletes gates the caller's upload until
// response headers arrive, then checks that both complete bodies survive.
func assertRawProxyResponseBeforeUploadCompletes(t *testing.T, chunked bool) {
	t.Helper()
	prefix := bytes.Repeat([]byte("p"), 16<<10)
	suffix := bytes.Repeat([]byte("s"), 48<<10)
	wantUpload := append(append([]byte(nil), prefix...), suffix...)
	const responseBody = "complete response through the CLI proxy"
	type upstreamResult struct {
		body          []byte
		contentLength int64
		err           error
	}
	upstreamDone := make(chan upstreamResult, 1)
	upstreamHeaders := make(chan struct{}, 1)
	authproxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Model the AuthProxy server returning evidence while the upload is open.
		result := upstreamResult{contentLength: r.ContentLength}
		defer func() { upstreamDone <- result }()
		controller := http.NewResponseController(w)
		if result.err = controller.EnableFullDuplex(); result.err != nil {
			return
		}
		result.body = make([]byte, len(prefix))
		if _, result.err = io.ReadFull(r.Body, result.body); result.err != nil {
			return
		}
		w.Header().Set("X-AuthProxy-Send-Evidence", "sent")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		if result.err = controller.Flush(); result.err != nil {
			return
		}
		upstreamHeaders <- struct{}{}
		var tail []byte
		tail, result.err = io.ReadAll(r.Body)
		result.body = append(result.body, tail...)
		if result.err == nil {
			_, result.err = io.WriteString(w, responseBody)
		}
	}))
	defer authproxy.Close()
	base, err := url.Parse("https://upstream.example.com")
	require.NoError(t, err)
	srv := httptest.NewServer(rawProxyHandler(authproxy.URL, "cxn_duplex", base, stubSigner("t")))
	defer srv.Close()

	// Send a prefix through a real client, withholding the rest until headers
	// return. Cancellation and pipe closure bound cleanup on assertion failures.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/upload", reader)
	require.NoError(t, err)
	req.ContentLength = int64(len(wantUpload))
	if chunked {
		req.ContentLength = -1
	}
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	uploadDone := make(chan error, 1)
	go func() {
		_, err := writer.Write(prefix)
		if err == nil {
			select {
			case <-release:
				_, err = writer.Write(suffix)
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		_ = writer.CloseWithError(err)
		uploadDone <- err
	}()
	type responseResult struct {
		response *http.Response
		err      error
	}
	headerDone := make(chan responseResult, 1)
	go func() {
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		headerDone <- responseResult{resp, err}
	}()
	select {
	case <-upstreamHeaders:
	case failed := <-upstreamDone:
		t.Fatalf("AuthProxy fixture returned before flushing headers: %v", failed.err)
	case <-time.After(3 * time.Second):
		t.Fatal("AuthProxy fixture did not receive the prefix and flush headers")
	}

	// The remaining upload is still blocked, so these headers must not depend
	// on a first response body byte or on draining the inbound request body.
	var resp *http.Response
	select {
	case result := <-headerDone:
		require.NoError(t, result.err)
		resp = result.response
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, []string{"sent"}, resp.Header.Values("X-AuthProxy-Send-Evidence"))
	case <-time.After(3 * time.Second):
		t.Fatal("CLI response headers waited for the unfinished request upload")
	}
	unblock()

	// Once released, both framing modes must preserve every request and
	// response byte; the known-length/chunked choice also reaches AuthProxy.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, responseBody, string(body))
	require.NoError(t, <-uploadDone)
	completed := <-upstreamDone
	require.NoError(t, completed.err)
	assert.Equal(t, req.ContentLength, completed.contentLength)
	assert.True(t, bytes.Equal(wantUpload, completed.body), "AuthProxy must receive the exact complete 64 KiB upload")
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

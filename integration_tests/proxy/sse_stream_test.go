//go:build integration

package proxy

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseTokenCount and sseTokenGap define the upstream's emit cadence.
// The maxArrival assertion below is slack enough to absorb scheduler
// jitter and the proxy hop, but tight enough that buffering the whole
// stream would clearly fail it (full buffering would land all tokens
// at the same time, ~sseTokenCount*sseTokenGap later).
const (
	sseTokenCount   = 20
	sseTokenGap     = 50 * time.Millisecond
	sseMaxArrival   = 150 * time.Millisecond
	sseTotalTimeout = 5 * time.Second
)

// sseUpstream is an httptest.Server that emits sseTokenCount SSE
// tokens with sseTokenGap between them, recording each token's emit
// time so the test can correlate against client arrival times.
type sseUpstream struct {
	server *httptest.Server
	mu     sync.Mutex
	emitAt []time.Time
}

func newSSEUpstream(t *testing.T) *sseUpstream {
	t.Helper()
	u := &sseUpstream{}
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		fl, ok := w.(http.Flusher)
		require.Truef(t, ok, "httptest.Server ResponseWriter must implement http.Flusher")
		fl.Flush()
		for i := 0; i < sseTokenCount; i++ {
			if i > 0 {
				time.Sleep(sseTokenGap)
			}
			u.mu.Lock()
			u.emitAt = append(u.emitAt, time.Now())
			u.mu.Unlock()
			fmt.Fprintf(w, "data: token-%d\n\n", i)
			fl.Flush()
		}
	}))
	t.Cleanup(u.server.Close)
	return u
}

func (u *sseUpstream) emitSnapshot() []time.Time {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]time.Time(nil), u.emitAt...)
}

// TestProxyRaw_SSEStreamingResponse proves the response side of
// /_proxy_raw streams an SSE-shaped response in real time rather
// than buffering the whole thing: each token must arrive at the
// client within sseMaxArrival of the upstream emitting it, and the
// proxied response must carry through the Content-Type:
// text/event-stream header so EventSource clients recognise the
// stream.
//
// Without the response-side streaming-skip (Content-Length<0 →
// no tee in the request_log roundtripper) and the flush-after-write
// wrapper inside the raw-proxy orchestrator, the test would fail
// either because all tokens arrive bunched at the end or because
// the response timing exceeds the per-token slack.
func TestProxyRaw_SSEStreamingResponse(t *testing.T) {
	connectorID := apid.MustParse("cxr_test0000000000331")

	upstream := newSSEUpstream(t)

	env := helpers.Setup(t, helpers.SetupOptions{
		Connectors: []sconfig.Connector{
			helpers.NewNoAuthConnector(connectorID, "sse-stream", nil),
		},
		StartHTTPServer: true,
	})
	defer env.Cleanup()
	conn := env.CreateConnection(t, connectorID, 1)

	resp := env.DoProxyRawStreamingRequest(t, conn, upstream.server.URL+"/stream", http.MethodGet, nil, 0, nil)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"),
		"SSE Content-Type must survive the proxy hop")

	// Read each event ("data: token-N\n\n" = two lines) and timestamp
	// arrival. Compare against the upstream's recorded emit time.
	reader := bufio.NewReader(resp.Body)
	arrivals := make([]time.Time, 0, sseTokenCount)
	overallDeadline := time.Now().Add(sseTotalTimeout)

	for i := 0; i < sseTokenCount; i++ {
		line, err := readLineWithDeadline(reader, time.Until(overallDeadline))
		require.NoErrorf(t, err, "token %d data line", i)
		arrivals = append(arrivals, time.Now())
		assert.Truef(t, strings.HasPrefix(line, "data: token-"), "token %d: got %q", i, line)
		// Consume the trailing blank line.
		_, err = readLineWithDeadline(reader, time.Until(overallDeadline))
		require.NoErrorf(t, err, "token %d blank line", i)
	}

	emits := upstream.emitSnapshot()
	require.Lenf(t, emits, sseTokenCount, "upstream must have emitted %d tokens by now", sseTokenCount)

	for i := 0; i < sseTokenCount; i++ {
		gap := arrivals[i].Sub(emits[i])
		assert.GreaterOrEqualf(t, gap, time.Duration(0), "token %d arrived before upstream emitted it (clock skew?)", i)
		assert.LessOrEqualf(t, gap, sseMaxArrival,
			"token %d arrived %s after upstream emit (max %s) — proxy is buffering",
			i, gap, sseMaxArrival)
	}
}

// readLineWithDeadline reads up to '\n' from r, returning an error
// if the read takes longer than budget. We can't set a Read deadline
// on an http.Response body directly, so the goroutine + select is
// the workable approximation.
func readLineWithDeadline(r *bufio.Reader, budget time.Duration) (string, error) {
	if budget <= 0 {
		return "", fmt.Errorf("deadline already passed")
	}
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		l, err := r.ReadString('\n')
		done <- result{strings.TrimRight(l, "\n"), err}
	}()
	select {
	case res := <-done:
		return res.line, res.err
	case <-time.After(budget):
		return "", fmt.Errorf("read timeout after %s", budget)
	}
}

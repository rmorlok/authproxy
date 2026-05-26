//go:build integration

package proxy

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upstreamEcho captures the bytes received and the framing the client
// used (chunked vs content-length) so the test can assert end-to-end
// pass-through regardless of which capture path was taken.
type upstreamEcho struct {
	server   *httptest.Server
	received atomic.Int64
	last     []byte
	mu       chan struct{}
}

func newUpstreamEcho(t *testing.T) *upstreamEcho {
	t.Helper()
	e := &upstreamEcho{mu: make(chan struct{}, 1)}
	e.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		e.received.Store(int64(len(body)))
		e.last = body
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(e.server.Close)
	return e
}

// waitForLog polls app metrics for a request event matching the
// connection id, up to deadline. Necessary because the round-tripper
// persists the event in a detached goroutine.
func waitForLog(t *testing.T, env *helpers.IntegrationTestEnv, connectionID string, deadline time.Duration) *app_metrics.LogRecord {
	t.Helper()
	connID, err := apid.Parse(connectionID)
	require.NoError(t, err)

	store := env.DM.GetAppMetricsService()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		page := store.NewListRequestsBuilder().
			WithConnectionId(connID).
			Limit(5).
			FetchPage(context.Background())
		if page.Error == nil && len(page.Results) > 0 {
			return page.Results[0]
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no log record persisted for connection %s within %s", connectionID, deadline)
	return nil
}

// TestProxyRawBodyCapture_TeeDecision is the end-to-end version of the
// unit-test invariants in app_metrics/roundtripper_test.go: the raw
// /_proxy_raw path drives the same RoundTripper, so the
// streaming/too_large/captured choice must show up on the LogRecord
// without any per-path special-casing.
//
// Per-subtest cleanup builds a fresh env so each one gets its own
// in-process state (app metrics accumulate across subtests
// otherwise).
func TestProxyRawBodyCapture_TeeDecision(t *testing.T) {
	connectorID := apid.MustParse("cxr_test0000000000302")

	t.Run("small known-length request: body captured", func(t *testing.T) {
		upstream := newUpstreamEcho(t)
		env := helpers.Setup(t, helpers.SetupOptions{
			Connectors: []sconfig.Connector{
				helpers.NewNoAuthConnector(connectorID, "raw-capture", nil),
			},
		})
		defer env.Cleanup()
		conn := env.CreateConnection(t, connectorID, 1)

		body := []byte("hello world")
		w := env.DoProxyRawRequest(t, conn, upstream.server.URL+"/echo", http.MethodPost, body, false, nil)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		assert.Equal(t, int64(len(body)), upstream.received.Load(), "upstream must receive the full body")

		record := waitForLog(t, env, conn, 5*time.Second)
		assert.Empty(t, record.RequestBodySkipped, "small known-length body must be captured (no skip reason)")
	})

	t.Run("oversized known-length request: skipped too_large, body still forwarded", func(t *testing.T) {
		upstream := newUpstreamEcho(t)
		env := helpers.Setup(t, helpers.SetupOptions{
			Connectors: []sconfig.Connector{
				helpers.NewNoAuthConnector(connectorID, "raw-capture", nil),
			},
		})
		defer env.Cleanup()
		conn := env.CreateConnection(t, connectorID, 1)

		// Default max_request_size is 250 KiB — send 300 KiB. The
		// load-bearing assertion is that the upstream still sees all
		// 300 KiB even though the log skipped capture; the previous
		// implementation truncated the *forwarded* body to the cap.
		body := make([]byte, 300*1024)
		_, _ = rand.Read(body)
		w := env.DoProxyRawRequest(t, conn, upstream.server.URL+"/echo", http.MethodPost, body, false, nil)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		assert.Equal(t, int64(len(body)), upstream.received.Load(), "upstream must receive the full body, not the truncated cap")

		record := waitForLog(t, env, conn, 5*time.Second)
		assert.Equal(t, app_metrics.BodySkippedTooLarge, record.RequestBodySkipped,
			"oversized body must record too_large skip reason")
	})

	t.Run("chunked request: skipped streaming, body still forwarded", func(t *testing.T) {
		upstream := newUpstreamEcho(t)
		env := helpers.Setup(t, helpers.SetupOptions{
			Connectors: []sconfig.Connector{
				helpers.NewNoAuthConnector(connectorID, "raw-capture", nil),
			},
		})
		defer env.Cleanup()
		conn := env.CreateConnection(t, connectorID, 1)

		body := []byte("chunked-stream-body")
		w := env.DoProxyRawRequest(t, conn, upstream.server.URL+"/echo", http.MethodPost, body, true, nil)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		assert.Equal(t, int64(len(body)), upstream.received.Load(), "upstream must receive the full body even when chunked")

		record := waitForLog(t, env, conn, 5*time.Second)
		assert.Equal(t, app_metrics.BodySkippedStreaming, record.RequestBodySkipped,
			"chunked body must record streaming skip reason")
	})
}

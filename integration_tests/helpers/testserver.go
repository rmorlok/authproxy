package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// RateLimitTestServer is an in-process HTTP server that can be configured to return 429 responses.
type RateLimitTestServer struct {
	Server       *http.Server
	Port         int
	BaseURL      string
	requestCount atomic.Int64

	return429       atomic.Bool
	retryAfterValue atomic.Value // stores string
}

// NewRateLimitTestServer starts a test HTTP server on a random port.
func NewRateLimitTestServer(t *testing.T) *RateLimitTestServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	port := listener.Addr().(*net.TCPAddr).Port

	ts := &RateLimitTestServer{
		Port:    port,
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
	}
	ts.retryAfterValue.Store("")

	mux := http.NewServeMux()
	mux.HandleFunc("/", ts.handleRequest)

	ts.Server = &http.Server{Handler: mux}

	go func() {
		if err := ts.Server.Serve(listener); err != nil && err != http.ErrServerClosed {
			// Server stopped
		}
	}()

	t.Cleanup(func() {
		ts.Server.Close()
	})

	return ts
}

// SetReturn429 configures the server to return 429 with the given Retry-After value.
func (ts *RateLimitTestServer) SetReturn429(retryAfter string) {
	ts.return429.Store(true)
	ts.retryAfterValue.Store(retryAfter)
}

// SetReturn200 configures the server to return 200 OK.
func (ts *RateLimitTestServer) SetReturn200() {
	ts.return429.Store(false)
	ts.retryAfterValue.Store("")
}

// GetRequestCount returns the total number of requests that reached the server.
func (ts *RateLimitTestServer) GetRequestCount() int64 {
	return ts.requestCount.Load()
}

// ResetRequestCount resets the request counter to zero.
func (ts *RateLimitTestServer) ResetRequestCount() {
	ts.requestCount.Store(0)
}

func (ts *RateLimitTestServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	ts.requestCount.Add(1)

	if ts.return429.Load() {
		retryAfter := ts.retryAfterValue.Load().(string)
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "rate limited"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"path":   r.URL.Path,
		"method": r.Method,
	})
}

// ConfigureTestServer sends a configuration request to a standalone test server.
// Used when the test server is run as a separate process.
func ConfigureTestServer(baseURL string, return429 bool, retryAfter string) error {
	body, _ := json.Marshal(map[string]interface{}{
		"return_429":        return429,
		"retry_after_value": retryAfter,
	})

	resp, err := http.Post(baseURL+"/configure", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("configure returned status %d", resp.StatusCode)
	}

	return nil
}

package helpers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

// ControlledResponse describes one upstream path's response. Configuration is
// snapshotted when a request arrives, so a blocked request keeps its original
// behavior even if the path is reconfigured while it is in flight.
type ControlledResponse struct {
	Status  int
	Headers http.Header
	Body    string
	// DropBeforeHeaders records the request and closes its socket without a response.
	DropBeforeHeaders bool
	// TruncateAfterHeaders advertises more body bytes than the server sends.
	TruncateAfterHeaders bool
	// Arrived is notified after recording the complete request body. Tests can
	// use a buffered channel to coordinate multiple requests without polling.
	Arrived chan<- struct{}
	// WaitFor gates the response after recording and notification. Cancellation
	// also releases the handler, preventing cleanup from waiting indefinitely.
	WaitFor <-chan struct{}
	// FlushHeaders commits response headers before writing any body bytes.
	FlushHeaders bool
	// WaitForBody gates the first body write after headers are committed. Use
	// with FlushHeaders to test header visibility on an open SSE response.
	WaitForBody <-chan struct{}
	// Trailers are declared before the response and supplied after its body.
	Trailers http.Header
	// LateTrailers use net/http's TrailerPrefix to send undeclared trailers.
	// FlushHeaders must be true so they cannot become ordinary response headers.
	LateTrailers http.Header
}

// ControlledRequest records independently observed upstream traffic. Body and
// headers are copied so callers cannot mutate the fixture's retained evidence.
type ControlledRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   string
}

// ControlledUpstream is a real HTTP listener with per-path behavior and request
// recordings. It is safe to configure and inspect during concurrent requests.
type ControlledUpstream struct {
	server   *httptest.Server
	mu       sync.Mutex
	actions  map[string]ControlledResponse
	requests []ControlledRequest
}

// NewControlledUpstream starts an isolated upstream and closes it during test
// cleanup. Unconfigured paths return 200 with an empty JSON object.
func NewControlledUpstream(t *testing.T) *ControlledUpstream {
	t.Helper()
	u := &ControlledUpstream{actions: make(map[string]ControlledResponse)}
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read controlled upstream request: %v", err)
			return
		}
		u.mu.Lock()
		action, configured := u.actions[r.URL.Path]
		u.requests = append(u.requests, ControlledRequest{
			Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: string(body),
		})
		u.mu.Unlock()
		if !configured {
			action = ControlledResponse{Body: "{}", Headers: http.Header{"Content-Type": {"application/json"}}}
		}
		if action.Arrived != nil {
			select {
			case action.Arrived <- struct{}{}:
			case <-r.Context().Done():
				return
			}
		}
		if action.WaitFor != nil {
			select {
			case <-action.WaitFor:
			case <-r.Context().Done():
				return
			}
		}
		if action.DropBeforeHeaders {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack controlled upstream connection: %v", err)
				return
			}
			_ = conn.Close()
			return
		}
		for name, values := range action.Headers {
			w.Header()[name] = append([]string(nil), values...)
		}
		if action.TruncateAfterHeaders {
			w.Header().Set("Content-Length", strconv.Itoa(len(action.Body)+100))
		}
		for name := range action.Trailers {
			w.Header().Add("Trailer", name)
		}
		status := action.Status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		if action.FlushHeaders {
			w.(http.Flusher).Flush()
		}
		if action.WaitForBody != nil {
			select {
			case <-action.WaitForBody:
			case <-r.Context().Done():
				return
			}
		}
		_, _ = io.WriteString(w, action.Body)
		for name, values := range action.Trailers {
			w.Header()[name] = append([]string(nil), values...)
		}
		for name, values := range action.LateTrailers {
			w.Header()[http.TrailerPrefix+name] = append([]string(nil), values...)
		}
	}))
	t.Cleanup(u.server.Close)
	return u
}

// Configure replaces the behavior for path without changing prior recordings.
func (u *ControlledUpstream) Configure(path string, action ControlledResponse) {
	u.mu.Lock()
	defer u.mu.Unlock()
	action.Headers = action.Headers.Clone()
	action.Trailers = action.Trailers.Clone()
	action.LateTrailers = action.LateTrailers.Clone()
	u.actions[path] = action
}

// URL returns the upstream's absolute URL for a slash-prefixed path.
func (u *ControlledUpstream) URL(path string) string { return u.server.URL + path }

// Requests returns a snapshot for path, or all requests when path is empty.
func (u *ControlledUpstream) Requests(path string) []ControlledRequest {
	u.mu.Lock()
	defer u.mu.Unlock()
	var result []ControlledRequest
	for _, req := range u.requests {
		if path == "" || req.Path == path {
			req.Header = req.Header.Clone()
			result = append(result, req)
		}
	}
	return result
}

// Count reports how many complete requests reached path, or all paths if empty.
func (u *ControlledUpstream) Count(path string) int { return len(u.Requests(path)) }

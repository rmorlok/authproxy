package helpers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

// ApiKeyStubUpstream is an in-process HTTP server that validates an incoming
// API-key credential per the configured placement (bearer / header / query /
// basic) and returns 200 on match, 401 otherwise. The accepted credential can
// be rotated at runtime to simulate provider-side key rotation.
//
// Recorded request bodies and headers are kept verbatim so tests can assert
// that the prior credential bytes never appear after a rotation (the
// no-replay invariant).
type ApiKeyStubUpstream struct {
	server  *http.Server
	BaseURL string

	placement    connectors.ApiKeyPlacementType
	headerName   string // when placement == "header"
	headerPrefix string
	paramName    string // when placement == "query"

	mu                sync.RWMutex
	acceptedKey       string
	acceptedUser      string // basic placement only
	requests          []ApiKeyStubRequest
	requestCount      atomic.Int64
	successCount      atomic.Int64
	unauthorizedCount atomic.Int64
}

// ApiKeyStubRequest captures one inbound request to the stub upstream for
// later inspection. Headers and the raw query are recorded so tests can prove
// that a particular byte string never traversed the upstream after a rotation.
type ApiKeyStubRequest struct {
	Method   string
	Path     string
	RawQuery string
	Headers  http.Header
	Status   int
}

// ApiKeyStubOptions configures NewApiKeyStubUpstream.
type ApiKeyStubOptions struct {
	// Placement selects which credential placement the upstream expects.
	Placement connectors.ApiKeyPlacementType

	// HeaderName is required when Placement == "header".
	HeaderName string

	// HeaderPrefix is an optional literal prepended to the key value when
	// Placement == "header" (e.g. "Token "). Matches the connector's
	// placement.prefix.
	HeaderPrefix string

	// ParamName is required when Placement == "query".
	ParamName string

	// AcceptedKey is the credential the upstream initially accepts. Can be
	// rotated at runtime via RotateAcceptedKey / RotateAcceptedBasic.
	AcceptedKey string

	// AcceptedUsername is only used for basic placement.
	AcceptedUsername string
}

// NewApiKeyStubUpstream starts a stub upstream on a random localhost port.
// The server is automatically shut down at the end of the test.
func NewApiKeyStubUpstream(t *testing.T, opts ApiKeyStubOptions) *ApiKeyStubUpstream {
	t.Helper()

	require.NotEmptyf(t, string(opts.Placement), "Placement is required")
	require.NotEmptyf(t, opts.AcceptedKey, "AcceptedKey is required")
	switch opts.Placement {
	case connectors.ApiKeyPlacementHeader:
		require.NotEmptyf(t, opts.HeaderName, "HeaderName is required for header placement")
	case connectors.ApiKeyPlacementQuery:
		require.NotEmptyf(t, opts.ParamName, "ParamName is required for query placement")
	case connectors.ApiKeyPlacementBasic:
		require.NotEmptyf(t, opts.AcceptedUsername, "AcceptedUsername is required for basic placement")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port

	s := &ApiKeyStubUpstream{
		BaseURL:      fmt.Sprintf("http://127.0.0.1:%d", port),
		placement:    opts.Placement,
		headerName:   opts.HeaderName,
		headerPrefix: opts.HeaderPrefix,
		paramName:    opts.ParamName,
		acceptedKey:  opts.AcceptedKey,
		acceptedUser: opts.AcceptedUsername,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	s.server = &http.Server{Handler: mux}

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			// Server stopped — nothing to report from the goroutine.
		}
	}()

	t.Cleanup(func() {
		_ = s.server.Close()
	})

	return s
}

// RotateAcceptedKey replaces the credential the stub upstream accepts. Used
// to simulate provider-side rotation: pre-rotation calls with the old key
// will now 401, while post-rotation calls with the new key will 200.
func (s *ApiKeyStubUpstream) RotateAcceptedKey(newKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acceptedKey = newKey
}

// RotateAcceptedBasic rotates both the username and key for a basic-auth
// stub upstream — usually only the key rotates in practice, but this lets
// tests force both to change.
func (s *ApiKeyStubUpstream) RotateAcceptedBasic(newUsername, newKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acceptedUser = newUsername
	s.acceptedKey = newKey
}

// RequestCount returns the total number of requests that reached the stub.
func (s *ApiKeyStubUpstream) RequestCount() int64 {
	return s.requestCount.Load()
}

// SuccessCount returns the number of requests that returned 200.
func (s *ApiKeyStubUpstream) SuccessCount() int64 {
	return s.successCount.Load()
}

// UnauthorizedCount returns the number of requests that returned 401.
func (s *ApiKeyStubUpstream) UnauthorizedCount() int64 {
	return s.unauthorizedCount.Load()
}

// Requests returns a snapshot of every request recorded by the stub.
func (s *ApiKeyStubUpstream) Requests() []ApiKeyStubRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ApiKeyStubRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

func (s *ApiKeyStubUpstream) handle(w http.ResponseWriter, r *http.Request) {
	s.requestCount.Add(1)

	status := http.StatusOK
	if !s.credentialMatches(r) {
		status = http.StatusUnauthorized
	}

	// Capture the request before responding so a panic in response writing
	// doesn't lose the observation.
	hdr := make(http.Header, len(r.Header))
	for k, vs := range r.Header {
		hdr[k] = append(hdr[k], vs...)
	}
	s.mu.Lock()
	s.requests = append(s.requests, ApiKeyStubRequest{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Headers:  hdr,
		Status:   status,
	})
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status == http.StatusOK {
		s.successCount.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "path": r.URL.Path})
	} else {
		s.unauthorizedCount.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized"})
	}
}

func (s *ApiKeyStubUpstream) credentialMatches(r *http.Request) bool {
	s.mu.RLock()
	expectedKey := s.acceptedKey
	expectedUser := s.acceptedUser
	s.mu.RUnlock()

	switch s.placement {
	case connectors.ApiKeyPlacementBearer:
		got := r.Header.Get("Authorization")
		return got == "Bearer "+expectedKey
	case connectors.ApiKeyPlacementHeader:
		got := r.Header.Get(s.headerName)
		return got == s.headerPrefix+expectedKey
	case connectors.ApiKeyPlacementQuery:
		got := r.URL.Query().Get(s.paramName)
		return got == expectedKey
	case connectors.ApiKeyPlacementBasic:
		got := r.Header.Get("Authorization")
		if !strings.HasPrefix(got, "Basic ") {
			return false
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got, "Basic "))
		if err != nil {
			return false
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return false
		}
		return parts[0] == expectedUser && parts[1] == expectedKey
	}
	return false
}

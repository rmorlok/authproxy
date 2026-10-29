package httpf

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSendObservation requires explicit evidence and retains any possible send.
func TestSendObservation(t *testing.T) {
	// Unreported observations, including their zero value, are conservative.
	var zero SendObservation
	var absent *SendObservation
	require.True(t, zero.MayHaveSent())
	require.True(t, absent.MayHaveSent())
	ctx, observation := ObserveSends(context.Background())
	require.True(t, observation.MayHaveSent())

	// Known-unsent reports establish a useful result without consuming it.
	ReportSendEvidence(ctx, false)
	ReportSendEvidence(ctx, false)
	require.False(t, observation.MayHaveSent())

	// A later possible send permanently overrides earlier or later rejections.
	ReportSendEvidence(ctx, true)
	ReportSendEvidence(ctx, false)
	require.True(t, observation.MayHaveSent())

	// Independent observations do not inherit or overwrite an outer result.
	inner, independent := ObserveSends(ctx)
	ReportSendEvidence(inner, false)
	require.False(t, independent.MayHaveSent())
	require.True(t, observation.MayHaveSent())
	ReportSendEvidence(context.Background(), true)
	ReportSendEvidence(nil, false)
}

// TestSendObservationConcurrentReports prevents false reports from overwriting
// a possible send while reports and readers race on one observation.
func TestSendObservationConcurrentReports(t *testing.T) {
	ctx, observation := ObserveSends(context.Background())
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for range 100 {
				ReportSendEvidence(ctx, i == 0)
				_ = observation.MayHaveSent()
			}
		}()
	}

	// Start all reporters together and inspect the permanent outcome afterward.
	close(start)
	workers.Wait()
	require.True(t, observation.MayHaveSent())
	ReportSendEvidence(ctx, false)
	require.True(t, observation.MayHaveSent())
}

// sendEvidenceRoundTripper adapts test behavior without real network access.
type sendEvidenceRoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip invokes the supplied test behavior.
func (f sendEvidenceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// sendEvidenceMiddleware rejects matching paths before delegating to its
// parent, modeling a local rate limiter inside the real factory stack.
type sendEvidenceMiddleware struct {
	// rejectPath is the URL path answered locally; empty rejects nothing.
	rejectPath string
}

// NewRoundTripper constructs a path-selective local rejection wrapper.
func (m sendEvidenceMiddleware) NewRoundTripper(_ RequestInfo, parent http.RoundTripper) http.RoundTripper {
	return sendEvidenceRoundTripper(func(req *http.Request) (*http.Response, error) {
		if m.rejectPath != "" && req.URL.Path == m.rejectPath {
			if req.Body != nil {
				_ = req.Body.Close()
			}
			return &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: http.NoBody}, nil
		}
		return parent.RoundTrip(req)
	})
}

// TestDoWithSendEvidenceLocalRejection proves that a full factory stack can
// report a synthetic status without claiming any underlying transport handoff.
func TestDoWithSendEvidenceLocalRejection(t *testing.T) {
	// No server is needed: the middleware must reject without contacting it.
	client := CreateFactory(nil, nil, sendEvidenceMiddleware{rejectPath: "/blocked"}, nil).NewHTTPClient()
	req, err := http.NewRequest(http.MethodPost, "https://example.invalid/blocked", strings.NewReader("upload"))
	require.NoError(t, err)
	resp, mayHaveSent, err := DoWithSendEvidence(client, req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.False(t, mayHaveSent)
}

// TestDoWithSendEvidenceUpstreamAndRedirect preserves a possible send when a
// redirect is eventually answered by a local middleware rejection.
func TestDoWithSendEvidenceUpstreamAndRedirect(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if req.URL.Path == "/redirect" {
			http.Redirect(w, req, "/blocked", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client := CreateFactory(nil, nil, sendEvidenceMiddleware{rejectPath: "/blocked"}, nil).NewHTTPClient()
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/direct", http.StatusNoContent},
		{"/redirect", http.StatusTooManyRequests},
	} {
		t.Run(tc.path, func(t *testing.T) {
			// Both outcomes contact the target once, despite different final sources.
			before := calls.Load()
			req, err := http.NewRequest(http.MethodGet, server.URL+tc.path, nil)
			require.NoError(t, err)
			resp, mayHaveSent, err := DoWithSendEvidence(client, req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			require.Equal(t, tc.status, resp.StatusCode)
			require.Equal(t, before+1, calls.Load())
			require.True(t, mayHaveSent)
		})
	}

	// Reusing the middleware client does not leak an earlier call's evidence.
	req, err := http.NewRequest(http.MethodGet, server.URL+"/blocked", nil)
	require.NoError(t, err)
	resp, mayHaveSent, err := DoWithSendEvidence(client, req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.False(t, mayHaveSent)
}

// TestDoWithSendEvidenceTransportFailure records the handoff even when the
// underlying transport fails before it could write any target request bytes.
func TestDoWithSendEvidenceTransportFailure(t *testing.T) {
	// A canceled request exercises the default transport's early-error path.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := CreateFactory(nil, nil, sendEvidenceMiddleware{}, nil).NewHTTPClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid/", nil)
	require.NoError(t, err)
	resp, mayHaveSent, err := DoWithSendEvidence(client, req)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, resp)
	require.True(t, mayHaveSent)
}

// TestDoWithSendEvidenceUnknownTransport treats uninstrumented results and
// pre-RoundTrip rejection as uncertain instead of trusting an absent marker.
func TestDoWithSendEvidenceUnknownTransport(t *testing.T) {
	privateErr := errors.New("transport failed")
	knownClient := CreateFactory(nil, nil, sendEvidenceMiddleware{rejectPath: "/"}, nil).NewHTTPClient()
	for _, tc := range []struct {
		name      string
		transport http.RoundTripper
		wantErr   error
	}{
		{"synthetic response", sendEvidenceRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: http.NoBody}, nil
		}), nil},
		{"transport failure", sendEvidenceRoundTripper(func(*http.Request) (*http.Response, error) {
			return nil, privateErr
		}), privateErr},
		{"handoff without coverage", &sendHandoffTransport{transport: sendEvidenceRoundTripper(func(*http.Request) (*http.Response, error) {
			return nil, privateErr
		})}, privateErr},
		{"unknown wrapper around known stack", sendEvidenceRoundTripper(func(req *http.Request) (*http.Response, error) {
			// Coverage from a nested stack cannot account for I/O this unknown
			// wrapper may perform before returning the stack's local rejection.
			return knownClient.Transport.RoundTrip(req)
		}), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The client is outside the complete factory contract, regardless of status.
			req, err := http.NewRequest(http.MethodGet, "https://example.invalid/", nil)
			require.NoError(t, err)
			resp, mayHaveSent, err := DoWithSendEvidence(&http.Client{Transport: tc.transport}, req)
			require.ErrorIs(t, err, tc.wantErr)
			if resp != nil {
				t.Cleanup(func() { _ = resp.Body.Close() })
			}
			require.True(t, mayHaveSent)
		})
	}

	// Even a known client cannot prove coverage when net/http rejects first.
	client := CreateFactory(nil, nil, sendEvidenceMiddleware{}, nil).NewHTTPClient()
	resp, mayHaveSent, err := DoWithSendEvidence(client, &http.Request{})
	require.Error(t, err)
	require.Nil(t, resp)
	require.True(t, mayHaveSent)
}

// TestSendEvidenceObservationIsolation ensures unrelated HTTP activity cannot
// write caller observations, even when it explicitly tracks transport evidence.
func TestSendEvidenceObservationIsolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client := CreateFactory(nil, nil, sendEvidenceMiddleware{}, nil).NewHTTPClient()
	ctx, observation := ObserveSends(context.Background())
	ReportSendEvidence(ctx, false)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	// Ordinary credential-like requests inherit the observation but cannot report.
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.False(t, observation.MayHaveSent())

	// Target tracking is returned separately and leaves the original context intact.
	resp, mayHaveSent, err := DoWithSendEvidence(client, req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.True(t, mayHaveSent)
	require.False(t, observation.MayHaveSent())
	require.Nil(t, req.Context().Value(targetSendEvidenceKey{}))
	ReportSendEvidence(ctx, mayHaveSent)
	require.True(t, observation.MayHaveSent())
}

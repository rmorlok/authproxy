package httpf

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gentleman.v2"
	gentlemancontext "gopkg.in/h2non/gentleman.v2/context"
	"gopkg.in/h2non/gentleman.v2/plugins/transport"
)

// TestSendObservationState verifies cumulative three-state evidence and the
// conservative interpretation of absent, invalid, and legacy reports.
func TestSendObservationState(t *testing.T) {
	// Absence is uncertainty without being a report that blocks later notSent.
	var absent *SendObservation
	var zero SendObservation
	require.Equal(t, SendStateMayHaveSent, absent.State())
	require.Equal(t, SendStateMayHaveSent, zero.State())
	ctx, observation := ObserveSends(context.Background())
	require.Equal(t, SendStateMayHaveSent, observation.State())
	ReportSendState(ctx, SendStateNotSent)
	require.Equal(t, SendStateNotSent, observation.State())
	require.False(t, observation.MayHaveSent())

	// Explicit uncertainty cannot be erased by any subsequent local rejection.
	ReportSendState(ctx, SendState("invalid"))
	ReportSendState(ctx, SendStateNotSent)
	require.Equal(t, SendStateMayHaveSent, observation.State())
	require.True(t, observation.MayHaveSent())

	// Confirmed responses dominate uncertainty, including old boolean reports.
	ReportSendState(ctx, SendStateSent)
	ReportSendEvidence(ctx, true)
	ReportSendEvidence(ctx, false)
	ReportSendState(ctx, SendState(""))
	require.Equal(t, SendStateSent, observation.State())
	require.True(t, observation.MayHaveSent())

	// A nested observation starts fresh and legacy true never establishes sent.
	inner, independent := ObserveSends(ctx)
	ReportSendEvidence(inner, true)
	require.Equal(t, SendStateMayHaveSent, independent.State())
	require.Equal(t, SendStateSent, observation.State())
	ReportSendState(nil, SendStateSent)
	ReportSendState(context.Background(), SendStateSent)
}

// TestSendObservationStateConcurrent preserves sent while every evidence kind
// races with concurrent readers of both the state and legacy boolean APIs.
func TestSendObservationStateConcurrent(t *testing.T) {
	ctx, observation := ObserveSends(context.Background())
	states := []SendState{SendStateNotSent, SendStateMayHaveSent, SendStateSent, "invalid"}
	start := make(chan struct{})
	var workers sync.WaitGroup
	for worker := range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for range 100 {
				ReportSendState(ctx, states[worker%len(states)])
				_ = observation.State()
				_ = observation.MayHaveSent()
			}
		}()
	}

	// All updates settle before checking that weaker reports cannot win.
	close(start)
	workers.Wait()
	require.Equal(t, SendStateSent, observation.State())
}

// sendStateThroughFactory exercises the real factory's two client builders and
// returns the raw response without consuming its body. POST avoids automatic
// replay when the fixture deliberately drops a connection before responding.
func sendStateThroughFactory(t *testing.T, kind string, factory F, ctx context.Context, target string) (*http.Response, SendState, error) {
	t.Helper()
	if kind == "http" {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, nil)
		require.NoError(t, err)
		return DoWithSendState(factory.NewHTTPClient(), req)
	}
	response, state, err := DoGentlemanWithSendState(factory.New().UseContext(ctx).Request().Method(http.MethodPost).URL(target))
	if response == nil {
		return nil, state, err
	}
	return response.RawResponse, state, err
}

// TestSendStateFactoryResponses distinguishes local responses from target
// replies and retains confirmed replies across redirects and body failures.
func TestSendStateFactoryResponses(t *testing.T) {
	var targetCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		targetCalls.Add(1)
		switch req.URL.Path {
		case "/redirect-blocked":
			http.Redirect(w, req, "/blocked", http.StatusFound)
		case "/redirect-drop":
			// The redirected GET starts on a fresh connection, preventing an
			// automatic retry of its deliberately dropped response.
			w.Header().Set("Connection", "close")
			http.Redirect(w, req, "/drop", http.StatusFound)
		case "/drop":
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack fixture connection: %v", err)
				return
			}
			_ = conn.Close()
		case "/unauthorized":
			w.WriteHeader(http.StatusUnauthorized)
		case "/limited":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/partial":
			w.Header().Set("Content-Length", "10")
			_, _ = w.Write([]byte("short"))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(server.Close)

	for _, kind := range []string{"http", "gentleman"} {
		t.Run(kind, func(t *testing.T) {
			factory := CreateFactory(nil, nil, sendEvidenceMiddleware{rejectPath: "/blocked"}, nil)
			for _, scenario := range []struct {
				path      string
				wantState SendState
				wantCalls int32
				wantError bool
			}{
				{"/direct", SendStateSent, 1, false},
				{"/unauthorized", SendStateSent, 1, false},
				{"/limited", SendStateSent, 1, false},
				{"/blocked", SendStateNotSent, 0, false},
				{"/drop", SendStateMayHaveSent, 1, true},
				{"/redirect-blocked", SendStateSent, 1, false},
				{"/redirect-drop", SendStateSent, 2, true},
				{"/partial", SendStateSent, 1, false},
				{"/blocked", SendStateNotSent, 0, false},
			} {
				t.Run(scenario.path, func(t *testing.T) {
					// Each call has its own observation even when its factory is reused.
					ctx, observation := ObserveSends(context.Background())
					before := targetCalls.Load()
					resp, state, err := sendStateThroughFactory(t, kind, factory, ctx, server.URL+scenario.path)
					require.Equal(t, scenario.wantState, state)
					require.Equal(t, before+scenario.wantCalls, targetCalls.Load())
					require.Equal(t, SendStateMayHaveSent, observation.State(), "helpers must not report")
					if scenario.wantError {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
					if resp != nil && resp.Body != nil {
						defer resp.Body.Close()
					}

					// A body read failure occurs after headers already proved a send.
					ReportSendState(ctx, state)
					if scenario.path == "/partial" {
						_, bodyErr := io.ReadAll(resp.Body)
						require.ErrorIs(t, bodyErr, io.ErrUnexpectedEOF)
						require.Equal(t, SendStateSent, observation.State())
					}
				})
			}
		})
	}
}

// TestSendStateTLSFailure remains uncertain when the transport attempts TLS but
// rejects the certificate before the target can receive an HTTP request.
func TestSendStateTLSFailure(t *testing.T) {
	var targetCalls atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	// The expected handshake rejection should not clutter the test output.
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)

	for _, kind := range []string{"http", "gentleman"} {
		t.Run(kind, func(t *testing.T) {
			// Use the factory's normal trust store, not the fixture's trusted client.
			factory := CreateFactory(nil, nil, sendEvidenceMiddleware{}, nil)
			_, state, err := sendStateThroughFactory(t, kind, factory, context.Background(), server.URL)
			require.Error(t, err)
			require.Equal(t, SendStateMayHaveSent, state)
			require.Zero(t, targetCalls.Load())
		})
	}
}

// sendStateWithTransport runs either helper with a caller-supplied transport so
// wrappers, missing instrumentation, and late Gentleman overrides are tested.
func sendStateWithTransport(t *testing.T, kind string, roundTripper http.RoundTripper, target string) (*http.Response, SendState, error) {
	t.Helper()
	if kind == "http" {
		req, err := http.NewRequest(http.MethodGet, target, nil)
		require.NoError(t, err)
		return DoWithSendState(&http.Client{Transport: roundTripper}, req)
	}
	// Request middleware overrides the factory's parent transport before dial.
	factory := CreateFactory(nil, nil, sendEvidenceMiddleware{}, nil)
	req := factory.New().Request().URL(target).Use(transport.Set(roundTripper))
	resp, state, err := DoGentlemanWithSendState(req)
	if resp == nil {
		return nil, state, err
	}
	return resp.RawResponse, state, err
}

// TestSendStateUnknownWrappers prevents nested coverage from proving notSent,
// while retaining a real target response observed beneath an unknown wrapper.
func TestSendStateUnknownWrappers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	known := CreateFactory(nil, nil, sendEvidenceMiddleware{rejectPath: "/blocked"}, nil).NewHTTPClient()
	postResponseError := errors.New("outer wrapper failed after receiving target response")

	for _, kind := range []string{"http", "gentleman"} {
		for _, scenario := range []struct {
			name      string
			transport http.RoundTripper
			path      string
			wantState SendState
			wantError error
		}{
			{
				"uninstrumented synthetic response",
				sendEvidenceRoundTripper(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
				}),
				"/direct", SendStateMayHaveSent, nil,
			},
			{
				"nested local rejection",
				sendEvidenceRoundTripper(known.Transport.RoundTrip),
				"/blocked", SendStateMayHaveSent, nil,
			},
			{
				"nested target response",
				sendEvidenceRoundTripper(known.Transport.RoundTrip),
				"/direct", SendStateSent, nil,
			},
			{
				"error after target response",
				sendEvidenceRoundTripper(func(req *http.Request) (*http.Response, error) {
					resp, err := known.Transport.RoundTrip(req)
					if err != nil {
						return nil, err
					}
					_ = resp.Body.Close()
					return nil, postResponseError
				}),
				"/direct", SendStateSent, postResponseError,
			},
		} {
			t.Run(kind+"/"+scenario.name, func(t *testing.T) {
				// Final response or error alone cannot substitute for target evidence.
				resp, state, err := sendStateWithTransport(t, kind, scenario.transport, server.URL+scenario.path)
				require.Equal(t, scenario.wantState, state)
				require.ErrorIs(t, err, scenario.wantError)
				if resp != nil && resp.Body != nil {
					require.NoError(t, resp.Body.Close())
				}
			})
		}
	}
}

// TestSendStateInvalidTransportResponses treats broken RoundTripper contracts
// as uncertainty instead of claiming an HTTP response was reliably received.
func TestSendStateInvalidTransportResponses(t *testing.T) {
	transportError := errors.New("transport failed")
	for _, scenario := range []struct {
		name   string
		status int
		err    error
	}{
		{"zero status", 0, nil},
		{"invalid status", 1000, nil},
		{"response alongside error", http.StatusOK, transportError},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Exercise the marker directly so net/http cannot normalize the fixture.
			evidence := &targetSendEvidence{}
			ctx := context.WithValue(context.Background(), targetSendEvidenceKey{}, evidence)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid/", nil)
			require.NoError(t, err)
			marker := &sendHandoffTransport{transport: sendEvidenceRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: scenario.status, Body: http.NoBody}, scenario.err
			})}
			_, err = marker.RoundTrip(req)
			require.ErrorIs(t, err, scenario.err)
			require.Equal(t, SendStateMayHaveSent, evidence.state(true))
		})
	}
}

// TestGentlemanSendStateEarlyInterception keeps plugin-provided responses and
// errors before the effective transport is known conservatively uncertain.
func TestGentlemanSendStateEarlyInterception(t *testing.T) {
	for _, status := range []int{0, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			// A plugin may perform arbitrary work, so bypassing dial proves nothing.
			req := gentleman.New().Request().URL("https://example.invalid/")
			req.UseRequest(func(ctx *gentlemancontext.Context, handler gentlemancontext.Handler) {
				if status == 0 {
					handler.Error(ctx, errors.New("request plugin failed"))
					return
				}
				ctx.Response = &http.Response{StatusCode: status, Header: make(http.Header), Body: http.NoBody}
				handler.Next(ctx)
			})
			_, state, err := DoGentlemanWithSendState(req)
			require.Equal(t, SendStateMayHaveSent, state)
			if status == 0 {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestGentlemanSendStateLateOverride rejects known-unsent conclusions when a
// before-dial plugin changes the effective transport after invoking its child.
func TestGentlemanSendStateLateOverride(t *testing.T) {
	for _, replacement := range []string{"transport", "client"} {
		t.Run(replacement, func(t *testing.T) {
			factory := CreateFactory(nil, nil, sendEvidenceMiddleware{rejectPath: "/blocked"}, nil)
			req := factory.New().Request().URL("https://example.invalid/blocked")
			req.UseHandler("before dial", func(ctx *gentlemancontext.Context, handler gentlemancontext.Handler) {
				known := ctx.Client.Transport
				handler.Next(ctx)
				// This wrapper can do untracked work before the known stack rejects.
				wrapped := sendEvidenceRoundTripper(known.RoundTrip)
				if replacement == "client" {
					ctx.Client = &http.Client{Transport: wrapped}
				} else {
					ctx.Client.Transport = wrapped
				}
			})

			// Recheck the final client, including replacement of its original pointer.
			resp, state, err := DoGentlemanWithSendState(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
			require.NoError(t, resp.RawResponse.Body.Close())
			require.Equal(t, SendStateMayHaveSent, state)
		})
	}
}

// TestSendStateRedirectTransportOverride preserves uncertainty when a redirect
// callback adds an unknown wrapper after a local synthetic redirect response.
func TestSendStateRedirectTransportOverride(t *testing.T) {
	known := &sendCoverageTransport{transport: sendEvidenceRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/redirect" {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"/blocked"}},
				Body:       http.NoBody,
			}, nil
		}
		return &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: http.NoBody}, nil
	})}
	client := &http.Client{Transport: known}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		// Coverage from the first hop cannot account for this wrapper's work.
		client.Transport = sendEvidenceRoundTripper(known.RoundTrip)
		return nil
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.invalid/redirect", nil)
	require.NoError(t, err)

	// Neither response came from a target, but the new wrapper is uninstrumented.
	resp, state, err := DoWithSendState(client, req)
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, SendStateMayHaveSent, state)
}

// TestSendStateCredentialIsolation keeps credential-like ordinary HTTP calls
// separate from target tracking and preserves the caller's original context.
func TestSendStateCredentialIsolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	factory := CreateFactory(nil, nil, sendEvidenceMiddleware{rejectPath: "/blocked"}, nil)
	for _, kind := range []string{"http", "gentleman"} {
		t.Run(kind, func(t *testing.T) {
			ctx, observation := ObserveSends(context.Background())
			ReportSendState(ctx, SendStateNotSent)

			// Credential traffic inherits the logical context without a target tracker.
			credentialReq, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/credential", nil)
			require.NoError(t, err)
			credentialResp, err := factory.NewHTTPClient().Do(credentialReq)
			require.NoError(t, err)
			require.NoError(t, credentialResp.Body.Close())

			// A later locally rejected target remains notSent despite credential I/O.
			resp, state, err := sendStateThroughFactory(t, kind, factory, ctx, server.URL+"/blocked")
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, SendStateNotSent, state)
			require.Equal(t, SendStateNotSent, observation.State())
			require.Nil(t, ctx.Value(targetSendEvidenceKey{}))
		})
	}
}

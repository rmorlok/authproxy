package request_log

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"log/slog"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/util"
	"github.com/stretchr/testify/require"
)

func TestRedisLogger_RoundTrip(t *testing.T) {
	mockTransport := &mockRoundTripper{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name                string
		recordFullRequest   bool
		maxFullRequestSize  uint64
		maxFullResponseSize uint64
		roundTripErr        error
		request             *http.Request
		response            *http.Response
		expectedEntry       *Entry
	}{
		{
			name:                "successful request, full logging enabled",
			recordFullRequest:   true,
			maxFullRequestSize:  1024,
			maxFullResponseSize: 1024,
			request: &http.Request{
				Method:     "GET",
				URL:        util.Must(url.Parse("http://example.com/path?q=1")),
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"Some":         []string{"header"},
				},
				Body:          io.NopCloser(bytes.NewBufferString(`{"some": "json"}`)),
				ContentLength: int64(len([]byte(`{"some": "json"}`))),
			},
			response: &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Proto:      "HTTP/2",
				ProtoMajor: 2,
				ProtoMinor: 0,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"Other":        []string{"header"},
				},
				Body:          io.NopCloser(bytes.NewBufferString(`{"other": "json"}`)),
				ContentLength: int64(len([]byte(`{"other": "json"}`))),
			},
			expectedEntry: &Entry{
				ID:            uuid.New(),
				CorrelationID: "some-value",
				Timestamp:     time.Now(),
				Request: EntryRequest{
					URL:         "http://example.com/path?q=1",
					HttpVersion: "HTTP/1.1",
					Method:      "GET",
					Headers: http.Header{
						"Content-Type": []string{"application/json"},
						"Some":         []string{"header"},
					},
					ContentLength: int64(len([]byte(`{"some": "json"}`))),
					Body:          []byte(`{"some": "json"}`),
				},
				Response: EntryResponse{
					HttpVersion: "HTTP/2",
					StatusCode:  http.StatusOK,
					Headers: http.Header{
						"Content-Type": []string{"application/json"},
						"Other":        []string{"header"},
					},
					ContentLength: int64(len([]byte(`{"other": "json"}`))),
					Body:          []byte(`{"other": "json"}`),
				},
			},
		},
		{
			name:                "successful request, full logging enabled, request and response truncated",
			recordFullRequest:   true,
			maxFullRequestSize:  4,
			maxFullResponseSize: 5,
			request: &http.Request{
				Method:     "GET",
				URL:        util.Must(url.Parse("http://example.com/path?q=1")),
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"Some":         []string{"header"},
				},
				Body:          io.NopCloser(bytes.NewBufferString(`{"some": "json"}`)),
				ContentLength: int64(len([]byte(`{"some": "json"}`))),
			},
			response: &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Proto:      "HTTP/2",
				ProtoMajor: 2,
				ProtoMinor: 0,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"Other":        []string{"header"},
				},
				Body:          io.NopCloser(bytes.NewBufferString(`{"other": "json"}`)),
				ContentLength: int64(len([]byte(`{"other": "json"}`))),
			},
			expectedEntry: &Entry{
				ID:            uuid.New(),
				CorrelationID: "some-value",
				Timestamp:     time.Now(),
				Request: EntryRequest{
					URL:         "http://example.com/path?q=1",
					HttpVersion: "HTTP/1.1",
					Method:      "GET",
					Headers: http.Header{
						"Content-Type": []string{"application/json"},
						"Some":         []string{"header"},
					},
					ContentLength: int64(len([]byte(`{"some": "json"}`))),
					Body:          []byte(`{"so`),
				},
				Response: EntryResponse{
					HttpVersion: "HTTP/2",
					StatusCode:  http.StatusOK,
					Headers: http.Header{
						"Content-Type": []string{"application/json"},
						"Other":        []string{"header"},
					},
					ContentLength: int64(len([]byte(`{"other": "json"}`))),
					Body:          []byte(`{"oth`),
				},
			},
		},
		{
			name:                "successful request, no full request logging",
			recordFullRequest:   false,
			maxFullRequestSize:  1024,
			maxFullResponseSize: 1024,
			request: &http.Request{
				Method:     "GET",
				URL:        util.Must(url.Parse("http://example.com/path?q=1")),
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"Some":         []string{"header"},
				},
				Body:          io.NopCloser(bytes.NewBufferString(`{"some": "json"}`)),
				ContentLength: int64(len([]byte(`{"some": "json"}`))),
			},
			response: &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Proto:      "HTTP/2",
				ProtoMajor: 2,
				ProtoMinor: 0,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"Other":        []string{"header"},
				},
				Body:          io.NopCloser(bytes.NewBufferString(`{"other": "json"}`)),
				ContentLength: int64(len([]byte(`{"other": "json"}`))),
			},
			expectedEntry: &Entry{
				ID:            uuid.New(),
				CorrelationID: "some-value",
				Timestamp:     time.Now(),
				Request: EntryRequest{
					URL:         "http://example.com/path?q=1",
					HttpVersion: "HTTP/1.1",
					Method:      "GET",
					Headers: http.Header{
						"Content-Type": []string{"application/json"},
						"Some":         []string{"header"},
					},
					ContentLength: int64(len([]byte(`{"some": "json"}`))),
					Body:          nil,
				},
				Response: EntryResponse{
					HttpVersion: "HTTP/2",
					StatusCode:  http.StatusOK,
					Headers: http.Header{
						"Content-Type": []string{"application/json"},
						"Other":        []string{"header"},
					},
					ContentLength: int64(len([]byte(`{"other": "json"}`))),
					Body:          nil,
				},
			},
		},
		{
			name:                "error making request",
			recordFullRequest:   true,
			maxFullRequestSize:  1024,
			maxFullResponseSize: 1024,
			request: &http.Request{
				Method:     "GET",
				URL:        util.Must(url.Parse("http://example.com/path?q=1")),
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"Some":         []string{"header"},
				},
				Body:          io.NopCloser(bytes.NewBufferString(`{"some": "json"}`)),
				ContentLength: int64(len([]byte(`{"some": "json"}`))),
			},
			roundTripErr: errors.New("network issue"),
			expectedEntry: &Entry{
				ID:            uuid.New(),
				CorrelationID: "some-value",
				Timestamp:     time.Now(),
				Request: EntryRequest{
					URL:         "http://example.com/path?q=1",
					HttpVersion: "HTTP/1.1",
					Method:      "GET",
					Headers: http.Header{
						"Content-Type": []string{"application/json"},
						"Some":         []string{"header"},
					},
					ContentLength: int64(len([]byte(`{"some": "json"}`))),
					Body:          []byte(`{"some": "json"}`),
				},
				Response: EntryResponse{
					StatusCode: http.StatusInternalServerError,
					Err:        "network issue",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockTransport.reset()
			mockTransport.response = test.response
			mockTransport.err = test.roundTripErr
			var result *Entry
			wg := sync.WaitGroup{}
			wg.Add(1)

			rLogger := &redisLogger{
				r:                     nil, // We are overriding the persistEntry method, so we don't need a redis client
				logger:                logger,
				recordFullRequest:     test.recordFullRequest,
				maxFullRequestSize:    test.maxFullRequestSize,
				maxFullResponseSize:   test.maxFullResponseSize,
				transport:             mockTransport,
				expiration:            time.Minute,
				fullRequestExpiration: time.Minute,
				maxResponseWait:       60 * time.Second,
				persistEntry: func(e *Entry, req *bytes.Buffer, resp *io.PipeReader) error {
					defer wg.Done()

					result = e

					if req != nil {
						result.Request.Body = req.Bytes()
					}

					if resp != nil {
						result.Response.Body, _ = io.ReadAll(resp)
					}

					return nil
				},
			}

			ctx := apctx.
				NewBuilderBackground().
				WithFixedUuidGenerator(test.expectedEntry.ID).
				WithCorrelationID(test.expectedEntry.CorrelationID).
				WithFixedClock(test.expectedEntry.Timestamp).
				Build()

			req := test.request.WithContext(ctx)
			resp, err := rLogger.RoundTrip(req)

			if resp != nil && resp.Body != nil {
				io.ReadAll(resp.Body) // Simulate response being consumed
				resp.Body.Close()
			}

			start := time.Now()
			wg.Wait()

			if test.roundTripErr != nil {
				if err == nil || err.Error() != test.roundTripErr.Error() {
					t.Fatalf("expected error %v, got %v", test.roundTripErr, err)
				}
			}

			if result == nil {
				t.Fatal("persistEntry not invoked")
			}

			if time.Since(start) > time.Second {
				t.Fatalf("persistEntry took too long: %v; this likely implies a deadlock that was broken by the max request timeout", time.Since(start))
			}

			// We can't control duration
			result.MillisecondDuration = test.expectedEntry.MillisecondDuration

			require.Equal(t, test.response, resp)
			require.Equal(t, test.expectedEntry, result)
		})
	}
}

type mockRoundTripper struct {
	response *http.Response
	err      error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	io.ReadAll(req.Body) // Simulate request being consumed
	return m.response, m.err
}

func (m *mockRoundTripper) reset() {
	m.response = nil
	m.err = nil
}

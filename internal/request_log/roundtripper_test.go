package request_log

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"log/slog"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// mockRecordStore captures StoreRecord calls for test assertions.
type mockRecordStore struct {
	mu      sync.Mutex
	records []*LogRecord
	err     error
}

func (m *mockRecordStore) StoreRecord(_ context.Context, record *LogRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, record)
	return m.err
}

func (m *mockRecordStore) StoreRecords(_ context.Context, records []*LogRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, records...)
	return m.err
}

func (m *mockRecordStore) getRecords() []*LogRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*LogRecord, len(m.records))
	copy(result, m.records)
	return result
}

// mockFullStore captures Store calls for test assertions.
type mockFullStore struct {
	mu   sync.Mutex
	logs []*FullLog
	err  error
	done chan struct{}
}

func newMockFullStore() *mockFullStore {
	return &mockFullStore{done: make(chan struct{}, 10)}
}

func (m *mockFullStore) Store(_ context.Context, log *FullLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, log)
	m.done <- struct{}{}
	return m.err
}

func (m *mockFullStore) GetFullLog(_ context.Context, _ string, _ apid.ID) (*FullLog, error) {
	return nil, nil
}

func (m *mockFullStore) getLogs() []*FullLog {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*FullLog, len(m.logs))
	copy(result, m.logs)
	return result
}

func (m *mockFullStore) waitForStore(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-m.done:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for store call")
	}
}

func TestRoundTripper_RoundTrip(t *testing.T) {
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
		expectedFullLog     *FullLog
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
			expectedFullLog: &FullLog{
				Id:            apid.New(apid.PrefixRequestLog),
				CorrelationID: "some-value",
				Timestamp:     time.Now(),
				Full:          true,
				Request: FullLogRequest{
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
				Response: FullLogResponse{
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
			expectedFullLog: &FullLog{
				Id:            apid.New(apid.PrefixRequestLog),
				CorrelationID: "some-value",
				Timestamp:     time.Now(),
				Full:          true,
				Request: FullLogRequest{
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
				Response: FullLogResponse{
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
			expectedFullLog: &FullLog{
				Id:            apid.New(apid.PrefixRequestLog),
				CorrelationID: "some-value",
				Timestamp:     time.Now(),
				Full:          false,
				Request: FullLogRequest{
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
				Response: FullLogResponse{
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
			expectedFullLog: &FullLog{
				Id:            apid.New(apid.PrefixRequestLog),
				CorrelationID: "some-value",
				Timestamp:     time.Now(),
				Full:          true,
				Request: FullLogRequest{
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
				Response: FullLogResponse{
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
			store := &mockRecordStore{}
			fullStore := newMockFullStore()

			rt := &RoundTripper{
				store:     store,
				fullStore: fullStore,
				logger:    logger,
				captureConfig: captureConfig{
					recordFullRequest:   test.recordFullRequest,
					maxFullRequestSize:  test.maxFullRequestSize,
					maxFullResponseSize: test.maxFullResponseSize,
					expiration:          time.Minute,
					fullRequestExpiration: time.Minute,
					maxResponseWait:     5 * 60 * time.Second,
				},
				requestInfo: httpf.RequestInfo{},
				transport:   mockTransport,
			}

			ctx := apctx.
				NewBuilderBackground().
				WithFixedIdGenerator(test.expectedFullLog.Id).
				WithCorrelationID(test.expectedFullLog.CorrelationID).
				WithFixedClock(test.expectedFullLog.Timestamp).
				Build()

			req := test.request.WithContext(ctx)
			resp, err := rt.RoundTrip(req)

			if resp != nil && resp.Body != nil {
				io.ReadAll(resp.Body) // Simulate response being consumed
				resp.Body.Close()
			}

			// Wait for the async goroutine to store the full log
			fullStore.waitForStore(t, 5*time.Second)

			if test.roundTripErr != nil {
				if err == nil || err.Error() != test.roundTripErr.Error() {
					t.Fatalf("expected error %v, got %v", test.roundTripErr, err)
				}
			}

			logs := fullStore.getLogs()
			require.Len(t, logs, 1, "expected exactly one full log stored")
			result := logs[0]

			// We freeze time so duration is zero
			test.expectedFullLog.MillisecondDuration = 0

			require.Equal(t, test.response, resp)
			require.Equal(t, test.expectedFullLog, result)

			// Verify that a record was also stored
			records := store.getRecords()
			require.Len(t, records, 1, "expected exactly one record stored")
			require.Equal(t, test.expectedFullLog.Id, records[0].RequestId)
		})
	}
}

func TestRoundTripper_RoundTrip_TimesOutAtConfiguredValue(t *testing.T) {
	request := &http.Request{
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
	}
	response := &http.Response{
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
		ContentLength: 0, // Unset directly, must be inferred from the response size
	}
	expectedFullLog := &FullLog{
		Id:               apid.New(apid.PrefixRequestLog),
		CorrelationID:    "some-value",
		Timestamp:        time.Now(),
		InternalTimeout:  true,
		RequestCancelled: false,
		Request: FullLogRequest{
			URL:         "http://example.com/path?q=1",
			HttpVersion: "HTTP/1.1",
			Method:      "GET",
			Headers: http.Header{
				"Content-Type": []string{"application/json"},
				"Some":         []string{"header"},
			},
			ContentLength: int64(len([]byte(`{"some": "json"}`))),
			Body:          nil, // Not configured for full request logging
		},
		Response: FullLogResponse{
			HttpVersion: "HTTP/2",
			StatusCode:  http.StatusOK,
			Headers: http.Header{
				"Content-Type": []string{"application/json"},
				"Other":        []string{"header"},
			},
			ContentLength: 0,   // Does not get set because body not consumed
			Body:          nil, // Not configured for full request logging
		},
	}

	mockTransport := &mockRoundTripper{}
	mockTransport.response = response
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &mockRecordStore{}
	fullStore := newMockFullStore()

	rt := &RoundTripper{
		store:     store,
		fullStore: fullStore,
		logger:    logger,
		captureConfig: captureConfig{
			recordFullRequest:     false,
			maxFullRequestSize:    0,
			maxFullResponseSize:   0,
			expiration:            time.Minute,
			fullRequestExpiration: time.Minute,
			maxResponseWait:       250 * time.Millisecond,
		},
		requestInfo: httpf.RequestInfo{},
		transport:   mockTransport,
	}

	ctx := apctx.
		NewBuilderBackground().
		WithFixedIdGenerator(expectedFullLog.Id).
		WithCorrelationID(expectedFullLog.CorrelationID).
		WithFixedClock(expectedFullLog.Timestamp).
		Build()

	req := request.WithContext(ctx)
	resp, _ := rt.RoundTrip(req)

	if resp != nil && resp.Body != nil {
		// Do not consume the body to force a timeout
	}

	// Wait for the async goroutine to store
	fullStore.waitForStore(t, 5*time.Second)

	logs := fullStore.getLogs()
	require.Len(t, logs, 1)
	result := logs[0]

	// We freeze time so duration is zero
	expectedFullLog.MillisecondDuration = 0

	require.Equal(t, response, resp)
	require.Equal(t, expectedFullLog, result)
}

func TestRoundTripper_RoundTrip_TimesOutAtContextCancel(t *testing.T) {
	request := &http.Request{
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
	}
	response := &http.Response{
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
		ContentLength: 0, // Unset directly, must be inferred from the response size
	}
	expectedFullLog := &FullLog{
		Id:               apid.New(apid.PrefixRequestLog),
		CorrelationID:    "some-value",
		Timestamp:        time.Now(),
		InternalTimeout:  false,
		RequestCancelled: true,
		Request: FullLogRequest{
			URL:         "http://example.com/path?q=1",
			HttpVersion: "HTTP/1.1",
			Method:      "GET",
			Headers: http.Header{
				"Content-Type": []string{"application/json"},
				"Some":         []string{"header"},
			},
			ContentLength: int64(len([]byte(`{"some": "json"}`))),
			Body:          nil, // Not configured for full request logging
		},
		Response: FullLogResponse{
			HttpVersion: "HTTP/2",
			StatusCode:  http.StatusOK,
			Headers: http.Header{
				"Content-Type": []string{"application/json"},
				"Other":        []string{"header"},
			},
			ContentLength: 0,   // Does not get set because body not consumed
			Body:          nil, // Not configured for full request logging
		},
	}

	mockTransport := &mockRoundTripper{}
	mockTransport.response = response
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &mockRecordStore{}
	fullStore := newMockFullStore()

	rt := &RoundTripper{
		store:     store,
		fullStore: fullStore,
		logger:    logger,
		captureConfig: captureConfig{
			recordFullRequest:     false,
			maxFullRequestSize:    0,
			maxFullResponseSize:   0,
			expiration:            time.Minute,
			fullRequestExpiration: time.Minute,
			maxResponseWait:       2 * time.Hour, // Really long so it will wait
		},
		requestInfo: httpf.RequestInfo{},
		transport:   mockTransport,
	}

	ctx := apctx.
		NewBuilderBackground().
		WithFixedIdGenerator(expectedFullLog.Id).
		WithCorrelationID(expectedFullLog.CorrelationID).
		WithFixedClock(expectedFullLog.Timestamp).
		Build()
	ctx, cancel := context.WithCancel(ctx)

	req := request.WithContext(ctx)
	resp, _ := rt.RoundTrip(req)

	if resp != nil && resp.Body != nil {
		// Do not consume the body to force a timeout
	}

	cancel()

	// Wait for the async goroutine to store
	fullStore.waitForStore(t, 5*time.Second)

	logs := fullStore.getLogs()
	require.Len(t, logs, 1)
	result := logs[0]

	// We freeze time so duration is zero
	expectedFullLog.MillisecondDuration = 0

	require.Equal(t, response, resp)
	require.Equal(t, expectedFullLog, result)
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

package app_metrics

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apblob"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type recordRetrieverStub struct {
	record *LogRecord
	err    error
}

func (r *recordRetrieverStub) GetRecord(_ context.Context, _ apid.ID) (*LogRecord, error) {
	return r.record, r.err
}

func (r *recordRetrieverStub) NewListRequestsBuilder() ListRequestBuilder {
	panic("not implemented")
}

func (r *recordRetrieverStub) ListRequestsFromCursor(context.Context, string) (ListRequestExecutor, error) {
	panic("not implemented")
}

func (r *recordRetrieverStub) QueryRequestEventMetrics(context.Context, []RequestEventMetricsQuery) ([]RequestEventMetricSeries, error) {
	panic("not implemented")
}

func (r *recordRetrieverStub) QueryResourceMetrics(context.Context, []ResourceMetricsQuery) ([]ResourceMetricSeries, error) {
	panic("not implemented")
}

type errorEncryptor struct {
	err error
}

func (e errorEncryptor) EncryptForNamespace(context.Context, string, []byte) (encfield.EncryptedField, error) {
	return encfield.EncryptedField{}, e.err
}

func (e errorEncryptor) Decrypt(context.Context, encfield.EncryptedField) ([]byte, error) {
	return nil, e.err
}

type errorBlobClient struct {
	err error
}

func (c errorBlobClient) Put(context.Context, apblob.PutInput) error {
	return c.err
}

func (c errorBlobClient) Get(context.Context, string) ([]byte, error) {
	return nil, c.err
}

func (c errorBlobClient) Delete(context.Context, string) error {
	return c.err
}

// noopEncryptor implements Encryptor using base64 encoding (no real encryption).
type noopEncryptor struct{}

func (noopEncryptor) EncryptForNamespace(_ context.Context, _ string, data []byte) (encfield.EncryptedField, error) {
	return encfield.EncryptedField{
		ID:   apid.ID("dek_noop"),
		Data: base64.StdEncoding.EncodeToString(data),
	}, nil
}

func (noopEncryptor) Decrypt(_ context.Context, ef encfield.EncryptedField) ([]byte, error) {
	return base64.StdEncoding.DecodeString(ef.Data)
}

type fakeTransport struct {
	status      int
	respBody    string
	readReqBody bool
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.readReqBody && req.Body != nil {
		io.ReadAll(req.Body)
	}
	resp := &http.Response{
		StatusCode:    f.status,
		Proto:         "HTTP/1.1",
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(f.respBody)),
		ContentLength: int64(len(f.respBody)),
	}
	resp.Header.Set("Content-Type", "text/plain")
	return resp, nil
}

func newNoopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestStorageService(recordFullRequest bool) (*StorageService, *mockRecordStore, *mockFullStore) {
	store := &mockRecordStore{}
	fullStore := newMockFullStore()
	logger := newNoopLogger()

	ss := &StorageService{
		logger:    logger,
		store:     store,
		fullStore: fullStore,
		captureConfig: captureConfig{
			expiration:            10 * time.Minute,
			recordFullRequest:     recordFullRequest,
			fullRequestExpiration: 5 * time.Minute,
			maxFullRequestSize:    1024,
			maxFullResponseSize:   1024,
			maxResponseWait:       60 * time.Second,
		},
	}

	return ss, store, fullStore
}

func TestStoreSummaryOnly_NoFullLog(t *testing.T) {
	ss, store, fullStore := newTestStorageService(false)

	ft := &fakeTransport{status: 200, respBody: "ok", readReqBody: true}
	ri := httpf.RequestInfo{Type: httpf.RequestTypeProxy}
	rt := ss.NewRoundTripper(ri, ft)

	req, _ := http.NewRequest("GET", "http://example.com/path?q=1", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := rt.RoundTrip(req)
	assert.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Wait for async storage
	fullStore.waitForStore(t, 2*time.Second)

	// Record should be stored
	records := store.getRecords()
	require.Equal(t, 1, len(records))
	require.Equal(t, httpf.RequestTypeProxy, records[0].Type)
	require.Equal(t, "GET", records[0].Method)
	require.Equal(t, "example.com", records[0].Host)
	require.Equal(t, "http", records[0].Scheme)
	require.Equal(t, "/path", records[0].Path)

	// Full log should still be stored (with Full=false since recordFullRequest is false)
	logs := fullStore.getLogs()
	require.Equal(t, 1, len(logs))
	require.False(t, logs[0].Full)
	require.Nil(t, logs[0].Request.Body)
	require.Nil(t, logs[0].Response.Body)
}

func TestStoreFullRequestAndResponse(t *testing.T) {
	ss, store, fullStore := newTestStorageService(true)

	ft := &fakeTransport{status: 201, respBody: "respdata", readReqBody: true}
	ri := httpf.RequestInfo{Type: httpf.RequestTypeGlobal}
	rt := ss.NewRoundTripper(ri, ft)

	reqBody := bytes.NewBufferString("reqdata")
	req, _ := http.NewRequest("POST", "https://service.local/api/v1", io.NopCloser(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	// Wait for async storage
	fullStore.waitForStore(t, 2*time.Second)

	// Record should be stored
	records := store.getRecords()
	require.Equal(t, 1, len(records))
	require.Equal(t, httpf.RequestTypeGlobal, records[0].Type)
	require.Equal(t, "POST", records[0].Method)
	require.Equal(t, 201, records[0].ResponseStatusCode)

	// Full log should be stored with bodies
	logs := fullStore.getLogs()
	require.Equal(t, 1, len(logs))
	require.True(t, logs[0].Full)
	require.Equal(t, "POST", logs[0].Request.Method)
	require.Equal(t, "https://service.local/api/v1", logs[0].Request.URL)
	require.Equal(t, []byte("reqdata"), logs[0].Request.Body)
	require.Equal(t, []byte("respdata"), logs[0].Response.Body)
	require.Equal(t, 201, logs[0].Response.StatusCode)
}

func TestErrorRoundTrip_StoresError(t *testing.T) {
	ss, store, fullStore := newTestStorageService(true)

	errTransport := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, assert.AnError
	})

	ri := httpf.RequestInfo{Type: httpf.RequestTypeOAuth}
	rt := ss.NewRoundTripper(ri, errTransport)

	req, _ := http.NewRequest("GET", "http://err.example.com", nil)
	_, _ = rt.RoundTrip(req)

	// Wait for async storage
	fullStore.waitForStore(t, 2*time.Second)

	// Record should be stored with error
	records := store.getRecords()
	require.Equal(t, 1, len(records))
	require.NotEmpty(t, records[0].ResponseError)
	require.Equal(t, httpf.RequestTypeOAuth, records[0].Type)

	// Full log should also have the error
	logs := fullStore.getLogs()
	require.Equal(t, 1, len(logs))
	require.NotEmpty(t, logs[0].Response.Err)
}

func TestNamespacePopulated(t *testing.T) {
	ss, store, fullStore := newTestStorageService(true)

	ft := &fakeTransport{status: 200, respBody: "ok", readReqBody: true}
	ri := httpf.RequestInfo{
		Type:      httpf.RequestTypeProxy,
		Namespace: "root.myns",
	}
	rt := ss.NewRoundTripper(ri, ft)

	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Wait for async storage
	fullStore.waitForStore(t, 2*time.Second)

	// Namespace should be populated in the record
	records := store.getRecords()
	require.Equal(t, 1, len(records))
	require.Equal(t, "root.myns", records[0].Namespace)

	// Namespace should be populated in the full log
	logs := fullStore.getLogs()
	require.Equal(t, 1, len(logs))
	require.Equal(t, "root.myns", logs[0].Namespace)
}

func TestLabelsPopulatedFromRequestInfo(t *testing.T) {
	ss, store, fullStore := newTestStorageService(true)

	ft := &fakeTransport{status: 200, respBody: "ok", readReqBody: true}
	ri := httpf.RequestInfo{
		Type:      httpf.RequestTypeProxy,
		Namespace: "root.myns",
		Labels:    map[string]string{"env": "prod", "team": "api"},
	}
	rt := ss.NewRoundTripper(ri, ft)

	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Wait for async storage
	fullStore.waitForStore(t, 2*time.Second)

	// Labels should be populated in the record
	records := store.getRecords()
	require.Equal(t, 1, len(records))
	require.Equal(t, map[string]string{"env": "prod", "team": "api"}, map[string]string(records[0].Labels))
}

func TestNilLabelsNotPopulated(t *testing.T) {
	ss, store, fullStore := newTestStorageService(true)

	ft := &fakeTransport{status: 200, respBody: "ok", readReqBody: true}
	ri := httpf.RequestInfo{
		Type:      httpf.RequestTypeProxy,
		Namespace: "root.myns",
	}
	rt := ss.NewRoundTripper(ri, ft)

	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Wait for async storage
	fullStore.waitForStore(t, 2*time.Second)

	// Labels should be nil/empty
	records := store.getRecords()
	require.Equal(t, 1, len(records))
	require.True(t, len(records[0].Labels) == 0)
}

func TestGetFullLog_BlobStore(t *testing.T) {
	logger := newNoopLogger()
	blob := apblob.NewMemoryClient()
	fullStore := NewBlobStore(blob, noopEncryptor{}, logger)

	testId := apid.New(apid.PrefixRequestEvents)
	ns := "root.test"

	original := &FullLog{
		Id:        testId,
		Namespace: ns,
		Timestamp: time.Now().UTC().Truncate(time.Millisecond),
		Full:      true,
		Request: FullLogRequest{
			URL:    "http://example.com/test",
			Method: "GET",
		},
		Response: FullLogResponse{
			StatusCode: 200,
		},
	}

	// Store the full log
	err := fullStore.Store(context.Background(), original)
	require.NoError(t, err)

	// Retrieve it
	result, err := fullStore.GetFullLog(context.Background(), ns, testId)
	require.NoError(t, err)
	require.Equal(t, original.Id, result.Id)
	require.Equal(t, original.Namespace, result.Namespace)
	require.Equal(t, original.Request.URL, result.Request.URL)
	require.Equal(t, original.Request.Method, result.Request.Method)
	require.Equal(t, original.Response.StatusCode, result.Response.StatusCode)
	require.Nil(t, result.Record, "the encrypted blob does not duplicate the searchable record")

	record := original.ToRecord()
	serviceResult, err := (&StorageService{
		retriever: &recordRetrieverStub{record: record},
		fullStore: fullStore,
	}).GetFullLog(context.Background(), testId)
	require.NoError(t, err)
	require.Same(t, record, serviceResult.Record)
}

func TestBlobStoreStore_ReturnsEncryptError(t *testing.T) {
	fullStore := NewBlobStore(apblob.NewMemoryClient(), errorEncryptor{err: errors.New("encrypt failed")}, newNoopLogger())

	err := fullStore.Store(context.Background(), &FullLog{
		Id:        apid.New(apid.PrefixRequestEvents),
		Namespace: "root",
	})

	require.ErrorContains(t, err, "encrypt full HTTP log entry")
	require.ErrorContains(t, err, "encrypt failed")
}

func TestBlobStoreStore_ReturnsBlobPutError(t *testing.T) {
	fullStore := NewBlobStore(errorBlobClient{err: errors.New("disk full")}, noopEncryptor{}, newNoopLogger())

	err := fullStore.Store(context.Background(), &FullLog{
		Id:        apid.New(apid.PrefixRequestEvents),
		Namespace: "root",
	})

	require.ErrorContains(t, err, "store full HTTP log entry in blob storage")
	require.ErrorContains(t, err, "disk full")
}

// newCaptureLoadTestRecord returns a searchable record with representative
// summary fields so degraded GetFullLog results can be checked against it.
func newCaptureLoadTestRecord(testId apid.ID) *LogRecord {
	return &LogRecord{
		RequestId:           testId,
		Namespace:           "root",
		CorrelationId:       "corr-test",
		Timestamp:           time.Now().UTC().Truncate(time.Millisecond),
		MillisecondDuration: MillisecondDuration(250 * time.Millisecond),
		Method:              "POST",
		Scheme:              "https",
		Host:                "api.example.com",
		Path:                "/v1/oauth/tokens",
		RequestHttpVersion:  "HTTP/1.1",
		RequestSizeBytes:    123,
		RequestMimeType:     "application/json",
		ResponseHttpVersion: "HTTP/2.0",
		ResponseStatusCode:  http.StatusOK,
		ResponseSizeBytes:   456,
		ResponseMimeType:    "application/json",
		FullRequestRecorded: true,
		RequestBodySkipped:  BodySkippedStreaming,
		ResponseBodySkipped: BodySkippedTooLarge,
		InternalTimeout:     true,
		RequestCancelled:    true,
	}
}

// captureLoadObservers wires a StorageService with an in-memory log sink and a
// recording tracer so tests can assert that blob failures stay observable.
type captureLoadObservers struct {
	logs  *bytes.Buffer
	spans *tracetest.SpanRecorder
	ctx   context.Context
	end   func()
}

func newCaptureLoadObservers(t *testing.T) *captureLoadObservers {
	t.Helper()
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "GET /metrics/request-events/:id")
	return &captureLoadObservers{
		logs:  &bytes.Buffer{},
		spans: spans,
		ctx:   ctx,
		end:   func() { span.End() },
	}
}

func TestGetFullLog_MissingBlobReturnsRecordSummary(t *testing.T) {
	testId := apid.New(apid.PrefixRequestEvents)
	record := newCaptureLoadTestRecord(testId)
	obs := newCaptureLoadObservers(t)

	// The record exists but blob storage has no blob for it.
	fullStore := newMockFullStore()
	fullStore.getErr = apblob.ErrBlobNotFound
	ss := &StorageService{
		logger:    slog.New(slog.NewJSONHandler(obs.logs, nil)),
		retriever: &recordRetrieverStub{record: record},
		fullStore: fullStore,
	}

	result, err := ss.GetFullLog(obs.ctx, testId)
	obs.end()

	// The read succeeds with the record's summary and a "missing" state.
	require.NoError(t, err)
	require.Same(t, record, result.Record)
	require.Equal(t, CaptureLoadStateMissing, result.CaptureLoadState)
	require.False(t, result.Full, "no captured details are available to render")
	require.Equal(t, testId, result.Id)
	require.Equal(t, "POST", result.Request.Method)
	require.Equal(t, "https://api.example.com/v1/oauth/tokens", result.Request.URL)
	require.Equal(t, http.StatusOK, result.Response.StatusCode)

	// A missing blob is logged as a warning, not an error.
	require.Contains(t, obs.logs.String(), `"level":"WARN"`)
	require.Contains(t, obs.logs.String(), `"capture_state":"missing"`)
	require.Contains(t, obs.logs.String(), testId.String())

	// The active span carries the error event and the resulting state.
	ended := obs.spans.Ended()
	require.Len(t, ended, 1)
	requireSpanCaptureState(t, ended[0], "missing")
}

func TestGetFullLog_BlobErrorReturnsRecordSummaryAndIsObservable(t *testing.T) {
	testId := apid.New(apid.PrefixRequestEvents)
	record := newCaptureLoadTestRecord(testId)
	obs := newCaptureLoadObservers(t)

	// Blob storage fails the read, as MinIO did with 503 SlowDownRead.
	fullStore := newMockFullStore()
	fullStore.getErr = errors.New("operation error S3: GetObject, https response error StatusCode: 503, api error SlowDownRead")
	ss := &StorageService{
		logger:    slog.New(slog.NewJSONHandler(obs.logs, nil)),
		retriever: &recordRetrieverStub{record: record},
		fullStore: fullStore,
	}

	result, err := ss.GetFullLog(obs.ctx, testId)
	obs.end()

	// The read still succeeds with the summary, flagged as "unavailable".
	require.NoError(t, err)
	require.Same(t, record, result.Record)
	require.Equal(t, CaptureLoadStateUnavailable, result.CaptureLoadState)
	require.False(t, result.Full)
	require.Equal(t, "POST", result.Request.Method)

	// The underlying storage error is logged at ERROR with identifying fields.
	logs := obs.logs.String()
	require.Contains(t, logs, `"level":"ERROR"`)
	require.Contains(t, logs, "SlowDownRead")
	require.Contains(t, logs, `"capture_state":"unavailable"`)
	require.Contains(t, logs, `"namespace":"root"`)

	// The span records the exception so it is visible in tracing.
	ended := obs.spans.Ended()
	require.Len(t, ended, 1)
	requireSpanCaptureState(t, ended[0], "unavailable")
	require.Contains(t, spanExceptionMessages(ended[0]), "SlowDownRead")
}

func TestGetFullLog_LoadedBlobSetsLoadedState(t *testing.T) {
	testId := apid.New(apid.PrefixRequestEvents)
	record := newCaptureLoadTestRecord(testId)

	// A stored blob loads normally and is marked as loaded.
	fullStore := newMockFullStore()
	fullStore.getLog = &FullLog{Id: testId, Namespace: "root", Full: true}
	ss := &StorageService{
		logger:    newNoopLogger(),
		retriever: &recordRetrieverStub{record: record},
		fullStore: fullStore,
	}

	result, err := ss.GetFullLog(context.Background(), testId)
	require.NoError(t, err)
	require.Equal(t, CaptureLoadStateLoaded, result.CaptureLoadState)
	require.True(t, result.Full)
	require.Same(t, record, result.Record)
}

func TestGetFullLog_MissingRecordStillReturnsNotFound(t *testing.T) {
	// Without a searchable record there is nothing to degrade to.
	ss := &StorageService{
		logger:    newNoopLogger(),
		retriever: &recordRetrieverStub{err: ErrNotFound},
		fullStore: newMockFullStore(),
	}

	_, err := ss.GetFullLog(context.Background(), apid.New(apid.PrefixRequestEvents))
	require.ErrorIs(t, err, ErrNotFound)
}

// requireSpanCaptureState asserts the capture-state attribute recorded on span.
func requireSpanCaptureState(t *testing.T, span sdktrace.ReadOnlySpan, want string) {
	t.Helper()
	for _, kv := range span.Attributes() {
		if string(kv.Key) == "authproxy.request_event.capture_state" {
			require.Equal(t, want, kv.Value.AsString())
			return
		}
	}
	t.Fatalf("span %q has no capture_state attribute", span.Name())
}

// spanExceptionMessages joins the exception.message attributes of the span's
// recorded error events.
func spanExceptionMessages(span sdktrace.ReadOnlySpan) string {
	var msgs []string
	for _, ev := range span.Events() {
		for _, kv := range ev.Attributes {
			if string(kv.Key) == "exception.message" {
				msgs = append(msgs, kv.Value.AsString())
			}
		}
	}
	return strings.Join(msgs, "\n")
}

// RoundTripperFunc is an adapter to allow the use of ordinary functions as http.RoundTripper.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

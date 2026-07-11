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

func TestGetFullLog_MissingBlobReturnsNotFound(t *testing.T) {
	testId := apid.New(apid.PrefixRequestEvents)
	ts := time.Now().UTC().Truncate(time.Millisecond)
	record := &LogRecord{
		RequestId:           testId,
		Namespace:           "root",
		CorrelationId:       "corr-test",
		Timestamp:           ts,
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
		FullRequestRecorded: false,
		RequestBodySkipped:  BodySkippedStreaming,
		ResponseBodySkipped: BodySkippedTooLarge,
		InternalTimeout:     true,
		RequestCancelled:    true,
	}
	fullStore := newMockFullStore()
	fullStore.getErr = apblob.ErrBlobNotFound
	ss := &StorageService{
		logger:    newNoopLogger(),
		retriever: &recordRetrieverStub{record: record},
		fullStore: fullStore,
	}

	_, err := ss.GetFullLog(context.Background(), testId)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestGetFullLog_ReturnsNonMissingBlobError(t *testing.T) {
	testId := apid.New(apid.PrefixRequestEvents)
	record := &LogRecord{
		RequestId: testId,
		Namespace: "root",
	}
	fullStore := newMockFullStore()
	fullStore.getErr = errors.New("decrypt full log")
	ss := &StorageService{
		logger:    newNoopLogger(),
		retriever: &recordRetrieverStub{record: record},
		fullStore: fullStore,
	}

	_, err := ss.GetFullLog(context.Background(), testId)
	require.ErrorContains(t, err, "decrypt full log")
}

// RoundTripperFunc is an adapter to allow the use of ordinary functions as http.RoundTripper.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

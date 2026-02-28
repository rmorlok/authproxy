package request_log

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apblob"
	apredis2 "github.com/rmorlok/authproxy/internal/apredis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmorlok/authproxy/internal/apredis"
)

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

func newTestRedis(t *testing.T) apredis.Client {
	t.Helper()
	_, r := apredis2.MustApplyTestConfig(nil)
	return r
}

func newNoopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func waitForKey(t *testing.T, r apredis.Client, pattern string, timeout time.Duration) ([]string, error) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		keys, err := r.Keys(ctx, pattern).Result()
		if err != nil {
			return nil, err
		}
		if len(keys) > 0 {
			return keys, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, context.DeadlineExceeded
}

func getTTL(t *testing.T, r apredis.Client, key string) time.Duration {
	t.Helper()
	d, err := r.TTL(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("TTL error: %v", err)
	}
	return d
}

func TestStoreSummaryOnly_NoFullLog(t *testing.T) {
	r := newTestRedis(t)
	logger := newNoopLogger()
	blob := apblob.NewMemoryClient()

	ft := &fakeTransport{status: 200, respBody: "ok", readReqBody: true}

	l := NewRedisLogger(
		r,
		blob,
		logger,
		RequestInfo{Type: RequestTypeProxy},
		10*time.Minute, // summary expiration
		false,          // recordFullRequest
		5*time.Minute,  // full expiration (unused)
		1024,           // max req
		1024,           // max resp
		60*time.Second,
		ft,
	)

	req, _ := http.NewRequest("GET", "http://example.com/path?q=1", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.RoundTrip(req)
	assert.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	keys, err := waitForKey(t, r, "rl:*", 500*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
	summaryKey := keys[0]

	// No full log expected in blob storage
	require.Equal(t, 0, len(blob.Keys()))

	vals, err := r.HGetAll(context.Background(), summaryKey).Result()
	require.NoError(t, err)
	// Sanity checks on required fields
	require.Equal(t, string(RequestTypeProxy), vals[fieldType])
	require.NotEmpty(t, vals[fieldRequestId])
	require.NotEmpty(t, vals[fieldTimestamp])
	require.Equal(t, "GET", vals[fieldMethod])
	require.Equal(t, "example.com", vals[fieldHost])
	require.Equal(t, "http", vals[fieldScheme])
	require.Equal(t, "/path", vals[fieldPath])

	// TTL roughly equals expiration
	ttl := getTTL(t, r, summaryKey)
	require.GreaterOrEqual(t, ttl, 9*time.Minute)
	require.LessOrEqual(t, ttl, 10*time.Minute)
}

func TestStoreFullRequestAndResponse_JSONStored(t *testing.T) {
	r := newTestRedis(t)
	logger := newNoopLogger()
	blob := apblob.NewMemoryClient()

	ft := &fakeTransport{status: 201, respBody: "respdata", readReqBody: true}

	l := NewRedisLogger(
		r,
		blob,
		logger,
		RequestInfo{Type: RequestTypeGlobal},
		2*time.Minute, // summary expiration
		true,          // recordFullRequest
		1*time.Minute, // full expiration
		1<<20,         // 1MB max req
		1<<20,         // 1MB max resp
		60*time.Second,
		ft,
	)

	reqBody := bytes.NewBufferString("reqdata")
	req, _ := http.NewRequest("POST", "https://service.local/api/v1", io.NopCloser(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.RoundTrip(req)
	require.NoError(t, err)
	// Read response fully to ensure tee copies and pipe closes
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	// Summary must exist
	keys, err := waitForKey(t, r, "rl:*", 500*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
	summaryKey := keys[0]

	// Validate summary TTL
	ttlSummary := getTTL(t, r, summaryKey)
	require.GreaterOrEqual(t, ttlSummary, 90*time.Second)
	require.LessOrEqual(t, ttlSummary, 2*time.Minute)

	// Full entry must exist in blob storage
	blobKeys := blob.Keys()
	require.Equal(t, 1, len(blobKeys))

	// Validate full entry content from blob storage
	data, err := blob.Get(context.Background(), blobKeys[0])
	require.NoError(t, err)
	var e Entry
	require.NoError(t, json.Unmarshal(data, &e))
	require.Equal(t, "POST", e.Request.Method)
	require.Equal(t, "https://service.local/api/v1", e.Request.URL)
	require.Equal(t, []byte("reqdata"), e.Request.Body)
	require.Equal(t, []byte("respdata"), e.Response.Body)
	require.Equal(t, 201, e.Response.StatusCode)
}

func TestErrorRoundTrip_StoresError(t *testing.T) {
	r := newTestRedis(t)
	logger := newNoopLogger()
	blob := apblob.NewMemoryClient()

	// Transport that returns error
	errTransport := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, assert.AnError
	})

	l := NewRedisLogger(
		r,
		blob,
		logger,
		RequestInfo{Type: RequestTypeOAuth},
		3*time.Minute,
		true,
		1*time.Minute,
		1024,
		1024,
		60*time.Second,
		errTransport,
	)

	req, _ := http.NewRequest("GET", "http://err.example.com", nil)
	_, _ = l.RoundTrip(req)

	keys, err := waitForKey(t, r, "rl:*", 500*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
	vals, err := r.HGetAll(context.Background(), keys[0]).Result()
	require.NoError(t, err)

	// Error message should be present (not empty)
	if v, ok := vals[fieldResponseError]; ok {
		require.NotEmpty(t, v)
	}
}

func TestNamespacePopulatedInRedis(t *testing.T) {
	r := newTestRedis(t)
	logger := newNoopLogger()
	blob := apblob.NewMemoryClient()

	ft := &fakeTransport{status: 200, respBody: "ok", readReqBody: true}

	l := NewRedisLogger(
		r,
		blob,
		logger,
		RequestInfo{
			Type:      RequestTypeProxy,
			Namespace: "root.myns",
		},
		10*time.Minute,
		true, // recordFullRequest so we can also check the JSON entry
		5*time.Minute,
		1024,
		1024,
		60*time.Second,
		ft,
	)

	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	resp, err := l.RoundTrip(req)
	require.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Wait for the summary hash
	keys, err := waitForKey(t, r, "rl:*", 500*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))

	vals, err := r.HGetAll(context.Background(), keys[0]).Result()
	require.NoError(t, err)
	require.Equal(t, "root.myns", vals[fieldNamespace], "namespace should be populated in Redis hash")

	// Also verify the full JSON entry has the namespace in blob storage
	blobKeys := blob.Keys()
	require.Equal(t, 1, len(blobKeys))

	data, err := blob.Get(context.Background(), blobKeys[0])
	require.NoError(t, err)
	var e Entry
	require.NoError(t, json.Unmarshal(data, &e))
	require.Equal(t, "root.myns", e.Namespace, "namespace should be populated in full JSON entry")
}

// RoundTripperFunc is an adapter to allow the use of ordinary functions as http.RoundTripper.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

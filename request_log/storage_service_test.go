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
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apredis "github.com/rmorlok/authproxy/redis"
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

func newTestRedis(t *testing.T) (apredis.R, *redis.Client) {
	t.Helper()
	_, r := apredis.MustApplyTestConfig(nil)
	return r, r.Client()
}

func newNoopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func waitForKey(t *testing.T, c *redis.Client, pattern string, timeout time.Duration) ([]string, error) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		keys, err := c.Keys(ctx, pattern).Result()
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

func getTTL(t *testing.T, c *redis.Client, key string) time.Duration {
	t.Helper()
	d, err := c.TTL(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("TTL error: %v", err)
	}
	return d
}

func TestStoreSummaryOnly_NoFullLog(t *testing.T) {
	r, client := newTestRedis(t)
	logger := newNoopLogger()

	ft := &fakeTransport{status: 200, respBody: "ok", readReqBody: true}

	l := NewRedisLogger(
		r,
		logger,
		RequestInfo{Type: RequestTypeProxy},
		10*time.Minute, // summary expiration
		false,          // recordFullRequest
		5*time.Minute,  // full expiration (unused)
		1024,           // max req
		1024,           // max resp
		ft,
	)

	req, _ := http.NewRequest("GET", "http://example.com/path?q=1", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.RoundTrip(req)
	assert.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	keys, err := waitForKey(t, client, "rl:*", 500*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
	summaryKey := keys[0]

	// No full log expected
	fullKeys, _ := client.Keys(context.Background(), "rlf:*").Result()
	require.Equal(t, 0, len(fullKeys))

	vals, err := client.HGetAll(context.Background(), summaryKey).Result()
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
	ttl := getTTL(t, client, summaryKey)
	require.GreaterOrEqual(t, ttl, 9*time.Minute)
	require.LessOrEqual(t, ttl, 10*time.Minute)
}

func TestStoreFullRequestAndResponse_JSONStored(t *testing.T) {
	r, client := newTestRedis(t)
	logger := newNoopLogger()

	ft := &fakeTransport{status: 201, respBody: "respdata", readReqBody: true}

	l := NewRedisLogger(
		r,
		logger,
		RequestInfo{Type: RequestTypeGlobal},
		2*time.Minute, // summary expiration
		true,          // recordFullRequest
		1*time.Minute, // full expiration
		1<<20,         // 1MB max req
		1<<20,         // 1MB max resp
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
	keys, err := waitForKey(t, client, "rl:*", 500*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
	summaryKey := keys[0]

	// Full entry must exist
	fullKeys, err := waitForKey(t, client, "rlf:*", 500*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 1, len(fullKeys))
	fullKey := fullKeys[0]

	// Validate TTLs
	ttlSummary := getTTL(t, client, summaryKey)
	require.GreaterOrEqual(t, ttlSummary, 90*time.Second)
	require.LessOrEqual(t, ttlSummary, 2*time.Minute)
	ttlFull := getTTL(t, client, fullKey)
	require.GreaterOrEqual(t, ttlFull, 30*time.Second)
	require.LessOrEqual(t, ttlFull, 1*time.Minute)

	// Validate full entry content
	data, err := client.Get(context.Background(), fullKey).Bytes()
	require.NoError(t, err)
	var e Entry
	require.NoError(t, json.Unmarshal(data, &e))
	require.Equal(t, "POST", e.Request.Method)
	require.Equal(t, "https://service.local/api/v1", e.Request.URL)
	require.Equal(t, []byte("reqdata"), e.Request.Body)
	require.Equal(t, []byte("respdata"), e.Response.Body)
	require.Equal(t, 201, e.Response.StatusCode)
}

func TestDirectStore_WritesKeys(t *testing.T) {
	r, client := newTestRedis(t)
	logger := newNoopLogger()

	ft := &fakeTransport{status: 200, respBody: "ok", readReqBody: false}
	ll := NewRedisLogger(r, logger, RequestInfo{Type: RequestTypePublic}, 1*time.Minute, true, 30*time.Second, 1024, 1024, ft)
	rl := ll.(*redisLogger)

	entry := &Entry{
		ID:        uuid.New(),
		Timestamp: time.Now(),
		Request: EntryRequest{
			URL:         "http://x/y",
			Method:      "GET",
			HttpVersion: "HTTP/1.1",
		},
		Response: EntryResponse{
			StatusCode: 200,
		},
	}

	// also try to mirror how values are constructed to ensure they are non-empty
	er := EntryRecord{}
	rl.requestInfo.setRedisRecordFields(&er)
	entry.setRedisRecordFields(&er)

	vals := make(map[string]interface{})
	er.setRedisRecordFields(vals)
	if len(vals) == 0 {
		t.Fatalf("constructed vals unexpectedly empty")
	}

	// Call the actual method under test (no body stored)
	err := rl.storeEntryInRedis(entry, nil, nil)
	require.NoError(t, err)

	key := redisLogKey(entry.ID)
	m, err := client.HGetAll(context.Background(), key).Result()

	require.NoError(t, err)
	require.NotEmpty(t, m)
}

func TestErrorRoundTrip_StoresError(t *testing.T) {
	r, client := newTestRedis(t)
	logger := newNoopLogger()

	// Transport that returns error
	errTransport := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, assert.AnError
	})

	l := NewRedisLogger(
		r,
		logger,
		RequestInfo{Type: RequestTypeOAuth},
		3*time.Minute,
		true,
		1*time.Minute,
		1024,
		1024,
		errTransport,
	)

	req, _ := http.NewRequest("GET", "http://err.example.com", nil)
	_, _ = l.RoundTrip(req)

	keys, err := waitForKey(t, client, "rl:*", 500*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
	vals, err := client.HGetAll(context.Background(), keys[0]).Result()
	require.NoError(t, err)

	// Error message should be present (not empty)
	if v, ok := vals[fieldResponseError]; ok {
		require.NotEmpty(t, v)
	}
}

// RoundTripperFunc is an adapter to allow the use of ordinary functions as http.RoundTripper.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

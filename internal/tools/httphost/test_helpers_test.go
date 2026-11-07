package httphost_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/stretchr/testify/require"
)

// sessionTestClient supplies scripted configured-client behavior to adapter tests.
type sessionTestClient struct {
	do func(*http.Request) (*http.Response, error)
}

// Do delegates to the scenario without applying any extra middleware.
func (c *sessionTestClient) Do(req *http.Request) (*http.Response, error) { return c.do(req) }

// sessionTestLimits bounds adapter tests without constraining ordinary responses.
func sessionTestLimits() aphttp.Limits {
	return aphttp.Limits{Timeout: 30 * time.Second, MaxRequests: 8, MaxResponseBytes: 8192, MaxStreamBytes: 8192, MaxHeaderBytes: 4096}
}

// sessionTestNew constructs an invocation and releases its timer after the test.
func sessionTestNew(t *testing.T, ctx context.Context, client iface.HTTPClient, limits aphttp.Limits) *httphost.Session {
	t.Helper()
	session, err := httphost.New(ctx, client, limits)
	require.NoError(t, err)
	t.Cleanup(session.Close)
	return session
}

// sessionTestBody records ownership and detects eager response consumption.
type sessionTestBody struct {
	reader io.Reader
	reads  atomic.Int32
	closes atomic.Int32
}

// Read records consumer access before forwarding to the finite source.
func (b *sessionTestBody) Read(p []byte) (int, error) {
	b.reads.Add(1)
	return b.reader.Read(p)
}

// Close records when the response or upload owner releases the body.
func (b *sessionTestBody) Close() error { b.closes.Add(1); return nil }

// sessionTestRequest gives each attempted call its own tracked upload body.
func sessionTestRequest(ctx context.Context) (*http.Request, *sessionTestBody) {
	body := &sessionTestBody{reader: strings.NewReader("request payload")}
	request := &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Scheme: "https", Host: "provider.example", Path: "/items"},
		Header: make(http.Header), Body: body,
	}
	return request.WithContext(ctx), body
}

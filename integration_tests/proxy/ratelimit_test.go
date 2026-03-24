//go:build integration

package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proxyResponse mirrors the JSON structure returned by the proxy endpoint.
type proxyResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	BodyRaw    []byte            `json:"body_raw"`
	BodyJson   interface{}       `json:"body_json"`
}

func parseProxyResponse(t *testing.T, w *httptest.ResponseRecorder) *proxyResponse {
	t.Helper()
	var resp proxyResponse
	require.Equal(t, http.StatusOK, w.Code, "proxy endpoint should return 200; body: %s", w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return &resp
}

func TestRateLimiting429(t *testing.T) {
	connectorID := apid.MustParse("cxr_test0000000000001")

	t.Run("upstream 429 triggers rate limiting and blocks subsequent requests", func(t *testing.T) {
		ts := helpers.NewRateLimitTestServer(t)

		noJitter := float64(0)
		env := helpers.Setup(t, helpers.SetupOptions{
			Connectors: []sconfig.Connector{
				helpers.NewNoAuthConnector(connectorID, "rate-limit-test", &connectors.RateLimiting{
					DefaultRetryAfter: &common.HumanDuration{Duration: 2 * time.Second},
					MaxRetryAfter:     &common.HumanDuration{Duration: 10 * time.Second},
					ExponentialBackoff: &connectors.ExponentialBackoff{
						JitterFraction: &noJitter,
					},
				}),
			},
		})
		defer env.Cleanup()

		connectionID := env.CreateConnection(t, connectorID, 1)

		// Step 1: Normal requests should succeed and reach the upstream
		w := env.DoProxyRequest(t, connectionID, ts.BaseURL+"/test", "GET")
		resp := parseProxyResponse(t, w)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "first request should succeed")
		assert.Equal(t, int64(1), ts.GetRequestCount(), "upstream should have received 1 request")

		// Step 2: Configure the upstream to return 429 with Retry-After
		ts.SetReturn429("2")

		// This request should reach the upstream and get a 429 back
		w = env.DoProxyRequest(t, connectionID, ts.BaseURL+"/test", "GET")
		resp = parseProxyResponse(t, w)
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "should get 429 from upstream")
		assert.Equal(t, int64(2), ts.GetRequestCount(), "upstream should have received 2 requests")

		// The response should NOT have X-Authproxy-Ratelimited since it came from the upstream
		assert.Empty(t, resp.Headers["X-Authproxy-Ratelimited"],
			"first 429 should come from upstream, not authproxy")

		// Step 3: Immediately send another request - authproxy should block it
		w = env.DoProxyRequest(t, connectionID, ts.BaseURL+"/test", "GET")
		resp = parseProxyResponse(t, w)
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "should get synthetic 429")
		assert.Equal(t, int64(2), ts.GetRequestCount(),
			"upstream should still have 2 requests - authproxy blocked the 3rd")
		assert.Equal(t, "true", resp.Headers["X-Authproxy-Ratelimited"],
			"should have authproxy rate limit header")

		// Step 4: Configure upstream back to 200 and wait for the backoff to expire
		ts.SetReturn200()
		time.Sleep(3 * time.Second)

		// This request should now go through to the upstream
		w = env.DoProxyRequest(t, connectionID, ts.BaseURL+"/test", "GET")
		resp = parseProxyResponse(t, w)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "request should succeed after backoff expires")
		assert.Equal(t, int64(3), ts.GetRequestCount(),
			"upstream should have received 3rd request after backoff")
	})

	t.Run("rate limiting is per-connection", func(t *testing.T) {
		ts := helpers.NewRateLimitTestServer(t)

		noJitter := float64(0)
		env := helpers.Setup(t, helpers.SetupOptions{
			Connectors: []sconfig.Connector{
				helpers.NewNoAuthConnector(connectorID, "rate-limit-test", &connectors.RateLimiting{
					DefaultRetryAfter: &common.HumanDuration{Duration: 5 * time.Second},
					MaxRetryAfter:     &common.HumanDuration{Duration: 10 * time.Second},
					ExponentialBackoff: &connectors.ExponentialBackoff{
						JitterFraction: &noJitter,
					},
				}),
			},
		})
		defer env.Cleanup()

		conn1 := env.CreateConnection(t, connectorID, 1)
		conn2 := env.CreateConnection(t, connectorID, 1)

		// Trigger 429 on connection 1
		ts.SetReturn429("5")
		w := env.DoProxyRequest(t, conn1, ts.BaseURL+"/test", "GET")
		resp := parseProxyResponse(t, w)
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

		// Connection 1 should be blocked
		ts.SetReturn200()
		w = env.DoProxyRequest(t, conn1, ts.BaseURL+"/test", "GET")
		resp = parseProxyResponse(t, w)
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		assert.Equal(t, "true", resp.Headers["X-Authproxy-Ratelimited"],
			"connection 1 should be rate limited")

		// Connection 2 should still work
		requestsBefore := ts.GetRequestCount()
		w = env.DoProxyRequest(t, conn2, ts.BaseURL+"/test", "GET")
		resp = parseProxyResponse(t, w)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "connection 2 should not be affected")
		assert.Equal(t, requestsBefore+1, ts.GetRequestCount(),
			"connection 2 request should reach upstream")
	})

	t.Run("retry-after header from upstream is respected", func(t *testing.T) {
		ts := helpers.NewRateLimitTestServer(t)

		noJitter := float64(0)
		env := helpers.Setup(t, helpers.SetupOptions{
			Connectors: []sconfig.Connector{
				helpers.NewNoAuthConnector(connectorID, "rate-limit-test", &connectors.RateLimiting{
					DefaultRetryAfter: &common.HumanDuration{Duration: 30 * time.Second},
					MaxRetryAfter:     &common.HumanDuration{Duration: 60 * time.Second},
					ExponentialBackoff: &connectors.ExponentialBackoff{
						JitterFraction: &noJitter,
					},
				}),
			},
		})
		defer env.Cleanup()

		connectionID := env.CreateConnection(t, connectorID, 1)

		// Set upstream to return 429 with short Retry-After (1 second)
		ts.SetReturn429("1")

		w := env.DoProxyRequest(t, connectionID, ts.BaseURL+"/test", "GET")
		resp := parseProxyResponse(t, w)
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

		// Should be blocked immediately
		ts.SetReturn200()
		w = env.DoProxyRequest(t, connectionID, ts.BaseURL+"/test", "GET")
		resp = parseProxyResponse(t, w)
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		assert.Equal(t, "true", resp.Headers["X-Authproxy-Ratelimited"])

		// Wait for the 1-second retry-after to expire
		time.Sleep(2 * time.Second)

		// Should be unblocked now
		w = env.DoProxyRequest(t, connectionID, ts.BaseURL+"/test", "GET")
		resp = parseProxyResponse(t, w)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "should succeed after retry-after expires")
	})

	t.Run("rate limiting disabled passes 429 through", func(t *testing.T) {
		ts := helpers.NewRateLimitTestServer(t)

		env := helpers.Setup(t, helpers.SetupOptions{
			Connectors: []sconfig.Connector{
				helpers.NewNoAuthConnector(connectorID, "rate-limit-test", &connectors.RateLimiting{
					Disabled: true,
				}),
			},
		})
		defer env.Cleanup()

		connectionID := env.CreateConnection(t, connectorID, 1)

		// Set upstream to return 429
		ts.SetReturn429("2")

		// Request should go through and return the upstream 429
		w := env.DoProxyRequest(t, connectionID, ts.BaseURL+"/test", "GET")
		resp := parseProxyResponse(t, w)
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		assert.Equal(t, int64(1), ts.GetRequestCount())

		// Next request should ALSO go through (not blocked by authproxy)
		w = env.DoProxyRequest(t, connectionID, ts.BaseURL+"/test", "GET")
		resp = parseProxyResponse(t, w)
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		assert.Equal(t, int64(2), ts.GetRequestCount(),
			"with rate limiting disabled, all requests should reach upstream")
		assert.Empty(t, resp.Headers["X-Authproxy-Ratelimited"],
			"should not have authproxy rate limit header when disabled")
	})
}

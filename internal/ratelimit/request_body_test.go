package ratelimit

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rejectedRequestBody detects reads and records ownership without consuming an
// upload. Its close error must not replace the rate-limit response.
type rejectedRequestBody struct {
	reads  int
	closes int
}

func (b *rejectedRequestBody) Read([]byte) (int, error) {
	b.reads++
	return 0, errors.New("rejected request body must not be read")
}

func (b *rejectedRequestBody) Close() error {
	b.closes++
	return errors.New("request body close failed")
}

func TestRateLimitRejectionClosesRequestBodyWithoutReading(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source app_metrics.ResponseSource
		ruleID apid.ID
		build  func(*testing.T, *enforcerEnv, *mockTransport) http.RoundTripper
	}{
		{
			name:   "connector cooldown",
			source: app_metrics.ResponseSourceConnectorRateLimiter,
			build: func(t *testing.T, env *enforcerEnv, upstream *mockTransport) http.RoundTripper {
				store := NewStore(env.rds)
				connectionID := testConnectionID()
				require.NoError(t, store.SetRateLimited(env.ctx(), connectionID, 30*time.Second))
				return &RoundTripper{
					connectionId: connectionID,
					store:        store,
					transport:    upstream,
					logger:       testLogger(),
				}
			},
		},
		{
			name:   "rate limit resource",
			source: app_metrics.ResponseSourceRateLimit,
			ruleID: apid.ID("rl_body"),
			build: func(t *testing.T, env *enforcerEnv, upstream *mockTransport) http.RoundTripper {
				env.loadRules(mkEnfRule("rl_body", minimalTokenBucketDef(1, rlschema.ModeEnforce)))
				rt := newEnforcer(t, env, proxyRI("cxn_body", "act_body"), upstream)
				// Consume the single token before testing the rejected upload.
				resp, err := rt.RoundTrip(mkProxyReq(t, env.ctx(), "https://upstream.example.com/upload"))
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, resp.StatusCode)
				require.NoError(t, resp.Body.Close())
				upstream.called = false
				return rt
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnforcerEnv(t)
			t.Cleanup(env.server.Close)
			upstream := &mockTransport{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       http.NoBody,
			}}
			rt := tc.build(t, env, upstream)
			ctx, attr := env.ctxWithAttr()
			body := &rejectedRequestBody{}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://upstream.example.com/upload", body)
			require.NoError(t, err)

			resp, err := (&http.Client{Transport: rt}).Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
			assert.Equal(t, "true", resp.Header.Get("X-Authproxy-Ratelimited"))
			assert.NotEmpty(t, resp.Header.Get("Retry-After"))
			assert.False(t, upstream.called, "rejected upload must not reach the upstream transport")
			assert.Zero(t, body.reads, "rejected upload must not be drained")
			assert.Equal(t, 1, body.closes, "rejecting transport must close its request body")
			assert.Equal(t, tc.source, attr.Source)
			if tc.ruleID != apid.Nil {
				assert.Equal(t, tc.ruleID, attr.RateLimitId)
				assert.Equal(t, string(rlschema.ModeEnforce), attr.RateLimitMode)
				assert.Equal(t, tc.ruleID.String(), resp.Header.Get("X-Authproxy-Ratelimit"))
			}
		})
	}
}

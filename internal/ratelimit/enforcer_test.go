package ratelimit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/request_log"
	"github.com/rmorlok/authproxy/internal/schema/common"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

// enforcerEnv is a thin wrapper around miniredis + an in-memory cache so
// integration tests can wire up the whole enforcer stack in a few lines.
type enforcerEnv struct {
	rds    apredis.Client
	server *miniredis.Miniredis
	cache  MutableCache
	clock  *clock.FakeClock
}

func newEnforcerEnv(t *testing.T) *enforcerEnv {
	t.Helper()
	_, r, server := apredis.MustApplyTestConfigWithServer(nil)
	return &enforcerEnv{
		rds:    r,
		server: server,
		cache:  NewCache(),
		clock:  clock.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	}
}

func (e *enforcerEnv) ctx() context.Context {
	return apctx.NewBuilderBackground().WithClock(e.clock).Build()
}

// withAttribution returns a context pre-stamped with an Attribution and
// the attribution itself so tests can inspect it after the round trip.
func (e *enforcerEnv) ctxWithAttr() (context.Context, *request_log.Attribution) {
	attr := &request_log.Attribution{}
	return request_log.ContextWithAttribution(e.ctx(), attr), attr
}

func (e *enforcerEnv) step(d time.Duration) {
	e.clock.Step(d)
	e.server.FastForward(d)
}

// loadRules atomically swaps the cache with the supplied rules; mimics
// what the Refresher (#219) does on each tick.
func (e *enforcerEnv) loadRules(rules ...*database.RateLimit) {
	e.cache.Replace(rules, e.clock.Now())
}

// fakeTransport records the last request seen and returns a canned 200.
// The enforcer's tests don't need to exercise real upstream behaviour —
// we just need to know whether the inner transport was invoked.
type fakeTransport struct {
	called  bool
	lastReq *http.Request
	resp    *http.Response
	err     error
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.called = true
	f.lastReq = req
	if f.resp != nil {
		return f.resp, f.err
	}
	return &http.Response{
		StatusCode:    200,
		Header:        http.Header{},
		Body:          io.NopCloser(nil),
		ContentLength: 0,
	}, nil
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{}
}

func mkProxyReq(t *testing.T, ctx context.Context, rawurl string) *http.Request {
	t.Helper()
	u, err := url.Parse(rawurl)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	require.NoError(t, err)
	return req
}

func mkEnfRule(id string, def rlschema.RateLimit) *database.RateLimit {
	return &database.RateLimit{Id: apid.ID(id), Namespace: "root", Definition: def}
}

func minimalTokenBucketDef(capacity int, mode rlschema.Mode) rlschema.RateLimit {
	return rlschema.RateLimit{
		Mode:     mode,
		Selector: rlschema.Selector{},
		Bucket:   rlschema.Bucket{Dimensions: []string{rlschema.DimensionActor}},
		Algorithm: rlschema.Algorithm{
			TokenBucket: &rlschema.TokenBucket{Capacity: capacity, RefillRate: 0.1},
		},
	}
}

func proxyRI(connID, actorID string) httpf.RequestInfo {
	labels := map[string]string{}
	if actorID != "" {
		labels[actorIDLabelKey] = actorID
	}
	return httpf.RequestInfo{
		Type:         httpf.RequestTypeProxy,
		Namespace:    "root",
		ConnectionId: apid.ID(connID),
		Labels:       labels,
	}
}

func newEnforcer(t *testing.T, env *enforcerEnv, ri httpf.RequestInfo, transport http.RoundTripper) http.RoundTripper {
	t.Helper()
	f := NewEnforcerFactory(env.cache, env.rds, aplog.NewNoopLogger())
	rt := f.NewRoundTripper(ri, transport)
	require.NotNil(t, rt)
	return rt
}

// --- happy paths ---

func TestEnforcer_NoRules_PassThrough(t *testing.T) {
	env := newEnforcerEnv(t)
	ft := newFakeTransport()
	rt := newEnforcer(t, env, proxyRI("cxn_a", "act_a"), ft)

	resp, err := rt.RoundTrip(mkProxyReq(t, env.ctx(), "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.True(t, ft.called)
}

func TestEnforcer_RuleMatchesButUnderLimit_PassThrough(t *testing.T) {
	env := newEnforcerEnv(t)
	env.loadRules(mkEnfRule("rl_a", minimalTokenBucketDef(5, rlschema.ModeEnforce)))

	ft := newFakeTransport()
	rt := newEnforcer(t, env, proxyRI("cxn_a", "act_a"), ft)

	ctx, attr := env.ctxWithAttr()
	resp, err := rt.RoundTrip(mkProxyReq(t, ctx, "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.True(t, ft.called)

	// Match recorded but no rejection.
	require.Len(t, attr.RateLimitMatched, 1)
	require.Equal(t, apid.ID("rl_a"), attr.RateLimitMatched[0].Id)
	require.Equal(t, request_log.ResponseSource(""), attr.Source, "no source stamped when no rule fired")
}

func TestEnforcer_RuleDoesNotMatch_PassThrough(t *testing.T) {
	env := newEnforcerEnv(t)
	def := minimalTokenBucketDef(1, rlschema.ModeEnforce)
	def.Selector.Methods = []string{"POST"} // request is GET; won't match
	env.loadRules(mkEnfRule("rl_a", def))

	ft := newFakeTransport()
	rt := newEnforcer(t, env, proxyRI("cxn_a", "act_a"), ft)

	ctx, attr := env.ctxWithAttr()
	resp, err := rt.RoundTrip(mkProxyReq(t, ctx, "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.True(t, ft.called)
	require.Empty(t, attr.RateLimitMatched)
}

// --- rejection ---

func TestEnforcer_SingleRuleRejects_SyntheticRetryAfterAndAttribution(t *testing.T) {
	env := newEnforcerEnv(t)
	env.loadRules(mkEnfRule("rl_a", minimalTokenBucketDef(1, rlschema.ModeEnforce)))

	ft := newFakeTransport()
	rt := newEnforcer(t, env, proxyRI("cxn_a", "act_a"), ft)

	ctx, attr := env.ctxWithAttr()
	// Drain the single token.
	_, _ = rt.RoundTrip(mkProxyReq(t, ctx, "https://upstream.example.com/x"))

	// Next request should be rejected.
	ctx2, attr2 := env.ctxWithAttr()
	resp, err := rt.RoundTrip(mkProxyReq(t, ctx2, "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get("Retry-After"))
	require.Equal(t, "true", resp.Header.Get("X-Authproxy-Ratelimited"))
	require.Equal(t, "rl_a", resp.Header.Get("X-Authproxy-Ratelimit"))

	// Body identifies the firing rule.
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	require.Equal(t, "rl_a", parsed["rate_limit_id"])

	// Attribution stamped on the rejected request.
	require.Equal(t, request_log.ResponseSourceRateLimit, attr2.Source)
	require.Equal(t, apid.ID("rl_a"), attr2.RateLimitId)
	require.Equal(t, "enforce", attr2.RateLimitMode)

	// First request didn't get rejected so its Source stayed default.
	require.NotEqual(t, request_log.ResponseSourceRateLimit, attr.Source)
}

// --- multi-rule all-apply ---

func TestEnforcer_MultiRuleAllApply_MostRestrictiveWins(t *testing.T) {
	env := newEnforcerEnv(t)
	// Two enforce rules, both saturated. rule "long" has a longer
	// retry-after; we expect the firing rule to be that one.
	env.loadRules(
		mkEnfRule("rl_short", rlschema.RateLimit{
			Mode:     rlschema.ModeEnforce,
			Selector: rlschema.Selector{},
			Bucket:   rlschema.Bucket{Dimensions: []string{rlschema.DimensionActor}},
			Algorithm: rlschema.Algorithm{
				FixedWindow: &rlschema.FixedWindow{
					Window: common.HumanDuration{Duration: time.Minute}, Limit: 0,
				},
			},
		}),
		mkEnfRule("rl_long", rlschema.RateLimit{
			Mode:     rlschema.ModeEnforce,
			Selector: rlschema.Selector{},
			Bucket:   rlschema.Bucket{Dimensions: []string{rlschema.DimensionActor}},
			Algorithm: rlschema.Algorithm{
				FixedWindow: &rlschema.FixedWindow{
					Window: common.HumanDuration{Duration: time.Hour}, Limit: 0,
				},
			},
		}),
	)

	ft := newFakeTransport()
	rt := newEnforcer(t, env, proxyRI("cxn_a", "act_a"), ft)

	ctx, attr := env.ctxWithAttr()
	resp, err := rt.RoundTrip(mkProxyReq(t, ctx, "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.Equal(t, apid.ID("rl_long"), attr.RateLimitId,
		"the rule with the longer Retry-After should win")
	require.Len(t, attr.RateLimitMatched, 2, "all matched rules recorded, not just the firing one")
}

// --- observe mode ---

func TestEnforcer_ObserveModeOnly_PassThroughButLogged(t *testing.T) {
	env := newEnforcerEnv(t)
	env.loadRules(mkEnfRule("rl_obs", minimalTokenBucketDef(0, rlschema.ModeObserve)))

	ft := newFakeTransport()
	rt := newEnforcer(t, env, proxyRI("cxn_a", "act_a"), ft)

	ctx, attr := env.ctxWithAttr()
	resp, err := rt.RoundTrip(mkProxyReq(t, ctx, "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode, "observe mode never rejects")
	require.True(t, ft.called)

	// Observe-only matches still surface on the request log.
	require.Len(t, attr.RateLimitMatched, 1)
	require.Equal(t, "observe", attr.RateLimitMatched[0].Mode)
	require.Equal(t, apid.ID("rl_obs"), attr.RateLimitId)
	require.Equal(t, "observe", attr.RateLimitMode)
	require.Equal(t, request_log.ResponseSource(""), attr.Source,
		"observe-only must NOT stamp Source = rate_limit — the response really came from upstream")
}

func TestEnforcer_ObservePlusEnforce_EnforceWins(t *testing.T) {
	env := newEnforcerEnv(t)
	env.loadRules(
		mkEnfRule("rl_obs", minimalTokenBucketDef(0, rlschema.ModeObserve)),
		mkEnfRule("rl_enf", minimalTokenBucketDef(0, rlschema.ModeEnforce)),
	)

	ft := newFakeTransport()
	rt := newEnforcer(t, env, proxyRI("cxn_a", "act_a"), ft)

	ctx, attr := env.ctxWithAttr()
	resp, err := rt.RoundTrip(mkProxyReq(t, ctx, "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.Equal(t, request_log.ResponseSourceRateLimit, attr.Source)
	require.Equal(t, apid.ID("rl_enf"), attr.RateLimitId, "enforce rule must be the firing one")
	require.Len(t, attr.RateLimitMatched, 2)
}

// --- per-bucket isolation ---

func TestEnforcer_PerBucketIsolation(t *testing.T) {
	env := newEnforcerEnv(t)
	env.loadRules(mkEnfRule("rl_a", minimalTokenBucketDef(1, rlschema.ModeEnforce)))

	// Actor A drains the bucket; Actor B is unaffected.
	rtA := newEnforcer(t, env, proxyRI("cxn_a", "act_a"), newFakeTransport())
	_, _ = rtA.RoundTrip(mkProxyReq(t, env.ctx(), "https://upstream.example.com/x"))
	resp, err := rtA.RoundTrip(mkProxyReq(t, env.ctx(), "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

	rtB := newEnforcer(t, env, proxyRI("cxn_b", "act_b"), newFakeTransport())
	resp, err = rtB.RoundTrip(mkProxyReq(t, env.ctx(), "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
}

// --- fail-open on Redis outage ---

func TestEnforcer_FailOpenOnRedisDown(t *testing.T) {
	env := newEnforcerEnv(t)
	env.loadRules(mkEnfRule("rl_a", minimalTokenBucketDef(1, rlschema.ModeEnforce)))
	env.server.Close() // kill Redis after rules are loaded

	ft := newFakeTransport()
	rt := newEnforcer(t, env, proxyRI("cxn_a", "act_a"), ft)
	resp, err := rt.RoundTrip(mkProxyReq(t, env.ctx(), "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode, "fail open: Redis down must not produce a customer-visible outage")
	require.True(t, ft.called)
}

// --- request type ---

func TestEnforcer_DefaultRequestTypesProxyAndProbe(t *testing.T) {
	env := newEnforcerEnv(t)
	env.loadRules(mkEnfRule("rl_a", minimalTokenBucketDef(0, rlschema.ModeEnforce)))

	// Proxy traffic: rejected.
	rtProxy := newEnforcer(t, env, proxyRI("cxn_a", "act_a"), newFakeTransport())
	resp, err := rtProxy.RoundTrip(mkProxyReq(t, env.ctx(), "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

	// Probe traffic: also rejected (default request types = [proxy, probe]).
	probeRI := proxyRI("cxn_b", "act_b")
	probeRI.Type = httpf.RequestTypeProbe
	rtProbe := newEnforcer(t, env, probeRI, newFakeTransport())
	resp, err = rtProbe.RoundTrip(mkProxyReq(t, env.ctx(), "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

func TestEnforcer_OAuthTrafficSkippedByDefault(t *testing.T) {
	env := newEnforcerEnv(t)
	// Rule with default request types ([proxy, probe]).
	env.loadRules(mkEnfRule("rl_a", minimalTokenBucketDef(0, rlschema.ModeEnforce)))

	oauthRI := proxyRI("cxn_a", "act_a")
	oauthRI.Type = httpf.RequestTypeOAuth
	ft := newFakeTransport()
	rt := newEnforcer(t, env, oauthRI, ft)
	resp, err := rt.RoundTrip(mkProxyReq(t, env.ctx(), "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode, "OAuth traffic is not in default request types")
	require.True(t, ft.called)
}

func TestEnforcer_OAuthTrafficGovernedWhenOptedIn(t *testing.T) {
	env := newEnforcerEnv(t)
	def := minimalTokenBucketDef(0, rlschema.ModeEnforce)
	def.Selector.RequestTypes = []common.RequestType{common.RequestTypeOAuth}
	env.loadRules(mkEnfRule("rl_a", def))

	oauthRI := proxyRI("cxn_a", "act_a")
	oauthRI.Type = httpf.RequestTypeOAuth
	ft := newFakeTransport()
	rt := newEnforcer(t, env, oauthRI, ft)
	resp, err := rt.RoundTrip(mkProxyReq(t, env.ctx(), "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.False(t, ft.called)
}

// --- factory short-circuits ---

func TestEnforcerFactory_NilCacheSkips(t *testing.T) {
	f := NewEnforcerFactory(nil, nil, aplog.NewNoopLogger())
	require.Nil(t, f.NewRoundTripper(httpf.RequestInfo{}, http.DefaultTransport))
}

// --- attribution wired correctly without pre-installed attribution ---

func TestEnforcer_NoAttributionInContext_DoesNotPanic(t *testing.T) {
	// In production an Attribution is installed by the request-log
	// round-tripper before the enforcer runs; tests that don't go
	// through that path shouldn't crash.
	env := newEnforcerEnv(t)
	env.loadRules(mkEnfRule("rl_a", minimalTokenBucketDef(0, rlschema.ModeEnforce)))

	ft := newFakeTransport()
	rt := newEnforcer(t, env, proxyRI("cxn_a", "act_a"), ft)
	resp, err := rt.RoundTrip(mkProxyReq(t, env.ctx(), "https://upstream.example.com/x"))
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

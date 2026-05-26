package ratelimit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/common"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

// EnforcerFactory builds round-trippers that evaluate cached RateLimit
// resources on each governed request. Separate from the existing Factory
// (which handles reactive 429 backoff per connection); both can run in
// the same middleware chain and both can fire on the same request — see
// the docs on EnforcerRoundTripper for the coexistence story.
type EnforcerFactory struct {
	cache  Cache
	redis  apredis.Client
	logger *slog.Logger
}

// NewEnforcerFactory wires the in-memory rule cache populated by the
// Refresher (#219) together with the Redis client used by Limiter
// (#221). The factory is per-process, the round-trippers it produces are
// per-request-info, and the Limiters those round-trippers build are
// per-rule-per-Decide-call.
func NewEnforcerFactory(cache Cache, redis apredis.Client, logger *slog.Logger) *EnforcerFactory {
	if logger == nil {
		logger = slog.Default()
	}
	return &EnforcerFactory{cache: cache, redis: redis, logger: logger}
}

// NewRoundTripper satisfies httpf.RoundTripperFactory. Returns a wrapper
// for every request — the wrapper itself decides per-call whether any
// rule matches. (We don't filter by request type here because rules can
// opt-in to non-default request types via Selector.RequestTypes; the
// Match() function applies the per-rule filter.)
func (f *EnforcerFactory) NewRoundTripper(ri httpf.RequestInfo, transport http.RoundTripper) http.RoundTripper {
	if f.cache == nil {
		// No cache wired — nothing to enforce against. Cheap pass-through.
		return nil
	}
	return &EnforcerRoundTripper{
		ri:        ri,
		cache:     f.cache,
		redis:     f.redis,
		logger:    f.logger,
		transport: transport,
	}
}

// EnforcerRoundTripper runs proxy-side rate-limit evaluation in front of
// the upstream call. For each cached rule that matches the request:
//
//  1. The rule's algorithm is checked / incremented via Limiter.Decide.
//  2. Enforce-mode rejections are collected; the one with the longest
//     Retry-After wins (most-restrictive). On any enforce rejection the
//     round-tripper short-circuits with a synthetic 429.
//  3. Observe-mode matches also call Decide() so observe counters stay
//     hot — flipping a rule from observe to enforce shouldn't reset its
//     bucket. Their decisions don't reject anything.
//
// Every match (enforce or observe) is recorded on the request-event
// Attribution so the firing rule, all matched rules, and the resolved
// bucket are visible to operators downstream.
//
// Coexistence with the connector-level reactive limiter (Factory in this
// package): both run in the middleware chain. The enforcer runs before
// the connector limiter — so if a proxy-side rate-limit resource
// rejects, the request never reaches the reactive limiter. A real
// upstream 429 still flows back through both.
type EnforcerRoundTripper struct {
	ri        httpf.RequestInfo
	cache     Cache
	redis     apredis.Client
	logger    *slog.Logger
	transport http.RoundTripper
}

func (rt *EnforcerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	rules := rt.cache.All()
	if len(rules) == 0 {
		return rt.transport.RoundTrip(req)
	}

	reqCtx := rt.buildRequestContext(req)
	matched := rt.findMatches(rules, reqCtx)
	if len(matched) == 0 {
		return rt.transport.RoundTrip(req)
	}

	// Execute Decide for each match. Failures fail open inside the
	// Limiter; here we just collect what each one returned and look for
	// rejections.
	var firing *matchedRule
	matchedSet := make([]app_metrics.RateLimitMatch, 0, len(matched))

	for i := range matched {
		m := &matched[i]
		limiter, err := NewLimiter(m.rule, rt.redis, rt.logger)
		if err != nil {
			rt.logger.WarnContext(ctx, "rate-limit limiter construction failed; skipping rule",
				slog.String("rule_id", string(m.rule.Id)),
				slog.String("error", err.Error()),
			)
			continue
		}

		decision, decideErr := limiter.Decide(ctx, m.bucket)
		// Limiter.Decide already fail-opens internally on Redis errors,
		// so a non-nil error is informational. We still continue
		// processing other rules.
		_ = decideErr

		m.decision = decision
		matchedSet = append(matchedSet, app_metrics.RateLimitMatch{
			Id:     m.rule.Id,
			Mode:   string(m.effectiveMode),
			Bucket: m.bucket.AsMap(),
		})

		// Only enforce-mode rejections can fire — observe-mode rules
		// keep their counters hot but never reject.
		if !decision.Allowed && m.effectiveMode == rlschema.ModeEnforce {
			if firing == nil || decision.RetryAfter > firing.decision.RetryAfter {
				firing = m
			}
		}
	}

	// Stamp the request-event Attribution with the firing rule (if any)
	// plus the full match set. When only observe rules matched, the
	// top-level RateLimit* fields are still populated (with the first
	// observe match) so a single log filter on rate_limit_id catches
	// both enforce-rejected and observe-only entries.
	if attr := app_metrics.AttributionFromContext(ctx); attr != nil {
		attr.RateLimitMatched = matchedSet
		switch {
		case firing != nil:
			attr.Source = app_metrics.ResponseSourceRateLimit
			attr.RateLimitId = firing.rule.Id
			attr.RateLimitMode = string(rlschema.ModeEnforce)
			attr.RateLimitBucket = firing.bucket.AsMap()
		default:
			for i := range matched {
				if matched[i].effectiveMode == rlschema.ModeObserve {
					attr.RateLimitId = matched[i].rule.Id
					attr.RateLimitMode = string(rlschema.ModeObserve)
					attr.RateLimitBucket = matched[i].bucket.AsMap()
					break
				}
			}
		}
	}

	if firing != nil {
		rt.logger.InfoContext(ctx, "request rejected by rate-limit resource",
			slog.String("rule_id", string(firing.rule.Id)),
			slog.Duration("retry_after", firing.decision.RetryAfter),
			slog.Int("matched_rules", len(matchedSet)),
		)
		return rt.syntheticTooManyRequests(firing.rule.Id, firing.decision.RetryAfter), nil
	}

	return rt.transport.RoundTrip(req)
}

// matchedRule pairs a rule with its match-time outputs so the per-rule
// loop carries everything subsequent steps need without re-running
// Match().
type matchedRule struct {
	rule          *database.RateLimit
	effectiveMode rlschema.Mode
	bucket        BucketKey
	decision      Decision
}

// findMatches runs Match() over every cached rule and returns the
// subset (rule + resolved bucket) that applies to this request.
func (rt *EnforcerRoundTripper) findMatches(rules []*database.RateLimit, reqCtx *RequestContext) []matchedRule {
	out := make([]matchedRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		ok, bucket, err := Match(rule.Definition, reqCtx)
		if err != nil {
			rt.logger.WarnContext(reqCtx.contextOrBackground(), "rate-limit rule match failed; skipping",
				slog.String("rule_id", string(rule.Id)),
				slog.String("error", err.Error()),
			)
			continue
		}
		if !ok {
			continue
		}
		out = append(out, matchedRule{
			rule:          rule,
			effectiveMode: rule.Definition.EffectiveMode(),
			bucket:        bucket,
		})
	}
	return out
}

// buildRequestContext extracts the per-request fields the matcher
// consumes. The ActorID is recovered from the request's per-request
// label snapshot, where ForActor() installs apxy/act/-/id; using the
// label keeps the in-flight RequestInfo struct narrow and avoids
// circular plumbing through httpf.
func (rt *EnforcerRoundTripper) buildRequestContext(req *http.Request) *RequestContext {
	rc := &RequestContext{
		Type:             common.RequestType(rt.ri.Type),
		Method:           req.Method,
		UpstreamURL:      req.URL,
		Namespace:        rt.ri.Namespace,
		ConnectionID:     rt.ri.ConnectionId,
		ConnectorID:      rt.ri.ConnectorId,
		ConnectorVersion: rt.ri.ConnectorVersion,
		Labels:           rt.ri.Labels,
	}
	if rt.ri.Labels != nil {
		if v, ok := rt.ri.Labels[actorIDLabelKey]; ok {
			rc.ActorID = apid.ID(v)
		}
	}
	return rc
}

// actorIDLabelKey is the per-request label httpf.ForActor() stamps with
// the initiating actor's id. Kept as a constant here rather than
// reaching into the database package to assemble it on every request.
const actorIDLabelKey = "apxy/act/-/id"

// syntheticTooManyRequests builds the 429 returned when an enforce-mode
// rule rejects. Modeled on the connector-level reactive limiter's synth
// 429 so callers see a consistent shape; the body identifies the rule
// id so consumers can correlate with the rate-limit list endpoint.
func (rt *EnforcerRoundTripper) syntheticTooManyRequests(ruleID apid.ID, retryAfter time.Duration) *http.Response {
	retryAfterSeconds := int(math.Ceil(retryAfter.Seconds()))
	if retryAfterSeconds < 1 {
		retryAfterSeconds = 1
	}
	body := fmt.Sprintf(
		`{"error":"rate limited","rate_limit_id":%q,"retry_after_seconds":%d}`,
		string(ruleID), retryAfterSeconds,
	)
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"Content-Type":            {"application/json"},
			"Retry-After":             {strconv.Itoa(retryAfterSeconds)},
			"X-Authproxy-Ratelimited": {"true"},
			"X-Authproxy-Ratelimit":   {string(ruleID)},
		},
		Body:          io.NopCloser(bytes.NewBufferString(body)),
		ContentLength: int64(len(body)),
	}
}

// contextOrBackground gives findMatches a context to log against even when
// the RequestContext predates the upstream call's context. Background is
// fine here — match logs are advisory.
func (rc *RequestContext) contextOrBackground() context.Context {
	return context.Background()
}

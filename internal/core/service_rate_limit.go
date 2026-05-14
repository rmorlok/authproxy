package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/ratelimit"
	"github.com/rmorlok/authproxy/internal/schema/common"
	rlschema "github.com/rmorlok/authproxy/internal/schema/rate_limit"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

func (s *service) GetRateLimit(ctx context.Context, id apid.ID) (iface.RateLimit, error) {
	rl, err := s.db.GetRateLimit(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapRateLimit(*rl, s), nil
}

func (s *service) CreateRateLimit(ctx context.Context, namespace string, def rlschema.RateLimit, labels, annotations map[string]string) (iface.RateLimit, error) {
	rl := &database.RateLimit{
		Id:          apid.New(apid.PrefixRateLimit),
		Namespace:   namespace,
		Definition:  def,
		Labels:      database.Labels(labels),
		Annotations: database.Annotations(annotations),
	}

	if err := s.db.CreateRateLimit(ctx, rl); err != nil {
		return nil, err
	}
	return wrapRateLimit(*rl, s), nil
}

func (s *service) UpdateRateLimitDefinition(ctx context.Context, id apid.ID, def rlschema.RateLimit) (iface.RateLimit, error) {
	rl, err := s.db.UpdateRateLimitDefinition(ctx, id, def)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapRateLimit(*rl, s), nil
}

func (s *service) DeleteRateLimit(ctx context.Context, id apid.ID) error {
	if err := s.db.DeleteRateLimit(ctx, id); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *service) UpdateRateLimitLabels(ctx context.Context, id apid.ID, labels map[string]string) (iface.RateLimit, error) {
	rl, err := s.db.UpdateRateLimitLabels(ctx, id, labels)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapRateLimit(*rl, s), nil
}

func (s *service) PutRateLimitLabels(ctx context.Context, id apid.ID, labels map[string]string) (iface.RateLimit, error) {
	rl, err := s.db.PutRateLimitLabels(ctx, id, labels)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapRateLimit(*rl, s), nil
}

func (s *service) DeleteRateLimitLabels(ctx context.Context, id apid.ID, keys []string) (iface.RateLimit, error) {
	rl, err := s.db.DeleteRateLimitLabels(ctx, id, keys)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapRateLimit(*rl, s), nil
}

func (s *service) UpdateRateLimitAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (iface.RateLimit, error) {
	rl, err := s.db.UpdateRateLimitAnnotations(ctx, id, annotations)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapRateLimit(*rl, s), nil
}

func (s *service) PutRateLimitAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (iface.RateLimit, error) {
	rl, err := s.db.PutRateLimitAnnotations(ctx, id, annotations)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapRateLimit(*rl, s), nil
}

func (s *service) DeleteRateLimitAnnotations(ctx context.Context, id apid.ID, keys []string) (iface.RateLimit, error) {
	rl, err := s.db.DeleteRateLimitAnnotations(ctx, id, keys)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapRateLimit(*rl, s), nil
}

type listRateLimitsWrapper struct {
	l database.ListRateLimitsBuilder
	e database.ListRateLimitsExecutor
	s *service
}

func (l *listRateLimitsWrapper) convertPageResult(result pagination.PageResult[database.RateLimit]) pagination.PageResult[iface.RateLimit] {
	if result.Error != nil {
		return pagination.PageResult[iface.RateLimit]{Error: result.Error}
	}

	out := make([]iface.RateLimit, 0, len(result.Results))
	for _, r := range result.Results {
		out = append(out, wrapRateLimit(r, l.s))
	}

	return pagination.PageResult[iface.RateLimit]{
		Results: out,
		Error:   result.Error,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listRateLimitsWrapper) executor() database.ListRateLimitsExecutor {
	if l.e != nil {
		return l.e
	}
	return l.l
}

func (l *listRateLimitsWrapper) FetchPage(ctx context.Context) pagination.PageResult[iface.RateLimit] {
	return l.convertPageResult(l.executor().FetchPage(ctx))
}

func (l *listRateLimitsWrapper) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[iface.RateLimit]) error {
	return l.executor().Enumerate(ctx, func(result pagination.PageResult[database.RateLimit]) (keepGoing pagination.KeepGoing, err error) {
		return callback(l.convertPageResult(result))
	})
}

func (l *listRateLimitsWrapper) Limit(lim int32) iface.ListRateLimitsBuilder {
	return &listRateLimitsWrapper{l: l.l.Limit(lim), s: l.s}
}

func (l *listRateLimitsWrapper) ForNamespaceMatcher(matcher string) iface.ListRateLimitsBuilder {
	return &listRateLimitsWrapper{l: l.l.ForNamespaceMatcher(matcher), s: l.s}
}

func (l *listRateLimitsWrapper) ForNamespaceMatchers(matchers []string) iface.ListRateLimitsBuilder {
	return &listRateLimitsWrapper{l: l.l.ForNamespaceMatchers(matchers), s: l.s}
}

func (l *listRateLimitsWrapper) OrderBy(f database.RateLimitOrderByField, o pagination.OrderBy) iface.ListRateLimitsBuilder {
	return &listRateLimitsWrapper{l: l.l.OrderBy(f, o), s: l.s}
}

func (l *listRateLimitsWrapper) IncludeDeleted() iface.ListRateLimitsBuilder {
	return &listRateLimitsWrapper{l: l.l.IncludeDeleted(), s: l.s}
}

func (l *listRateLimitsWrapper) ForLabelSelector(selector string) iface.ListRateLimitsBuilder {
	return &listRateLimitsWrapper{l: l.l.ForLabelSelector(selector), s: l.s}
}

func (s *service) ListRateLimitsBuilder() iface.ListRateLimitsBuilder {
	return &listRateLimitsWrapper{
		l: s.db.ListRateLimitsBuilder(),
		s: s,
	}
}

func (s *service) ListRateLimitsFromCursor(ctx context.Context, cursor string) (iface.ListRateLimitsExecutor, error) {
	e, err := s.db.ListRateLimitsFromCursor(ctx, cursor)
	if err != nil {
		return nil, err
	}
	return &listRateLimitsWrapper{e: e, s: s}, nil
}

var _ iface.ListRateLimitsBuilder = (*listRateLimitsWrapper)(nil)

// DryRunRateLimit evaluates which cached rules would apply to the
// synthesized request and what Limiter.Peek says about each — without
// writing to any counter. Hydration mirrors the runtime: when
// ConnectionId is given, namespace / connector / labels come from the
// connection (the way httpf.ForConnection populates them); otherwise
// raw fields are used and manual Labels always merge on top.
//
// Rule cascade matches the enforcer: rules at the request's namespace
// or any ancestor are evaluated; rules in unrelated branches are
// filtered out.
func (s *service) DryRunRateLimit(ctx context.Context, req iface.DryRunRateLimitRequest) (iface.DryRunRateLimitResult, error) {
	if req.Request.Method == "" {
		return iface.DryRunRateLimitResult{}, fmt.Errorf("%w: request.method is required", ErrInvalidArgument)
	}
	if req.Request.URL == "" {
		return iface.DryRunRateLimitResult{}, fmt.Errorf("%w: request.url is required", ErrInvalidArgument)
	}
	if req.RequestType == "" {
		return iface.DryRunRateLimitResult{}, fmt.Errorf("%w: request_type is required", ErrInvalidArgument)
	}
	if !common.IsValidRequestType(req.RequestType) {
		return iface.DryRunRateLimitResult{}, fmt.Errorf("%w: invalid request_type %q", ErrInvalidArgument, req.RequestType)
	}
	if req.Context.ConnectionId == nil && req.Context.Namespace == nil {
		return iface.DryRunRateLimitResult{}, fmt.Errorf("%w: connection_id or namespace is required", ErrInvalidArgument)
	}

	reqCtx, err := s.hydrateDryRunContext(ctx, req)
	if err != nil {
		return iface.DryRunRateLimitResult{}, err
	}
	if reqCtx.Namespace == "" {
		return iface.DryRunRateLimitResult{}, fmt.Errorf("%w: could not determine namespace from context", ErrInvalidArgument)
	}

	// Pull from the enforcer's cache so operators see exactly what
	// the runtime would: a rule freshly written but not yet
	// propagated won't appear here either.
	var rules []*database.RateLimit
	if s.rlCache != nil {
		rules = filterRulesInScope(s.rlCache.All(), reqCtx.Namespace)
	}

	matched := make([]iface.DryRunRateLimitMatch, 0)
	notMatched := make([]iface.DryRunRateLimitNotMatched, 0)

	for _, rule := range rules {
		if rule == nil {
			continue
		}
		ok, bucket, reason, mErr := ratelimit.MatchExplain(rule.Definition, reqCtx)
		if mErr != nil {
			// Malformed rule (e.g. uncompilable regex that escaped
			// validation) — surface as a miss so the dry-run doesn't
			// die on a single bad rule.
			notMatched = append(notMatched, iface.DryRunRateLimitNotMatched{
				RateLimitId: rule.Id,
				Namespace:   rule.Namespace,
				Reason:      fmt.Sprintf("malformed rule: %s", mErr.Error()),
			})
			continue
		}
		if !ok {
			notMatched = append(notMatched, iface.DryRunRateLimitNotMatched{
				RateLimitId: rule.Id,
				Namespace:   rule.Namespace,
				Reason:      reason,
			})
			continue
		}

		limiter, lErr := ratelimit.NewLimiter(rule, s.r, s.logger)
		if lErr != nil {
			// Skip; the cache shouldn't hold rules whose algorithm
			// validation failed, so this is a should-not-happen.
			continue
		}

		decision, _ := limiter.Peek(ctx, bucket)
		matched = append(matched, iface.DryRunRateLimitMatch{
			RateLimitId:      rule.Id,
			Namespace:        rule.Namespace,
			EffectiveMode:    string(rule.Definition.EffectiveMode()),
			BucketKey:        bucket.String(),
			AlgorithmSummary: algorithmSummary(rule.Definition),
			WouldAllow:       decision.Allowed,
			Remaining:        decision.Remaining,
			RetryAfterMs:     decision.RetryAfter.Milliseconds(),
			PeekFailed:       decision.FailedOpen,
		})
	}

	return iface.DryRunRateLimitResult{
		Namespace:            reqCtx.Namespace,
		RequestLabelSnapshot: reqCtx.Labels,
		Matched:              matched,
		NotMatched:           notMatched,
	}, nil
}

// hydrateDryRunContext fills a RequestContext from the dry-run input.
// When a connection is supplied, the connection's namespace / connector
// / labels are used (mirroring httpf.ForConnection). Labels on the
// ProxyRequest itself always merge on top, matching the real proxy
// path's "request labels override connection labels" rule.
func (s *service) hydrateDryRunContext(ctx context.Context, req iface.DryRunRateLimitRequest) (*ratelimit.RequestContext, error) {
	u, err := url.Parse(req.Request.URL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid url: %s", ErrInvalidArgument, err.Error())
	}

	rc := &ratelimit.RequestContext{
		Type:        common.RequestType(req.RequestType),
		Method:      strings.ToUpper(req.Request.Method),
		UpstreamURL: u,
	}

	if req.Context.ConnectionId != nil && !req.Context.ConnectionId.IsNil() {
		conn, err := s.GetConnection(ctx, *req.Context.ConnectionId)
		if err != nil {
			return nil, err
		}
		rc.Namespace = conn.GetNamespace()
		rc.ConnectionID = conn.GetId()
		rc.ConnectorID = conn.GetConnectorId()
		rc.ConnectorVersion = conn.GetConnectorVersion()
		// Connection labels already carry forward namespace + connector
		// labels — copy so manual overrides don't mutate the core's map.
		if connLabels := conn.GetLabels(); len(connLabels) > 0 {
			rc.Labels = make(map[string]string, len(connLabels))
			for k, v := range connLabels {
				rc.Labels[k] = v
			}
		}
	} else if req.Context.Namespace != nil {
		rc.Namespace = *req.Context.Namespace
	}

	if req.Context.ActorId != nil && !req.Context.ActorId.IsNil() {
		rc.ActorID = *req.Context.ActorId
	}

	if len(req.Request.Labels) > 0 {
		if rc.Labels == nil {
			rc.Labels = make(map[string]string, len(req.Request.Labels))
		}
		for k, v := range req.Request.Labels {
			rc.Labels[k] = v
		}
	}

	return rc, nil
}

// filterRulesInScope keeps rules whose namespace is the request's
// namespace or any ancestor — matching the runtime cascade where a rule
// at root.foo applies to a request in root.foo.bar.
func filterRulesInScope(rules []*database.RateLimit, requestNamespace string) []*database.RateLimit {
	out := make([]*database.RateLimit, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		if rule.Namespace == requestNamespace || strings.HasPrefix(requestNamespace, rule.Namespace+".") {
			out = append(out, rule)
		}
	}
	return out
}

// algorithmSummary renders the chosen algorithm for the dry-run output.
func algorithmSummary(def rlschema.RateLimit) string {
	a := def.Algorithm
	switch {
	case a.TokenBucket != nil:
		return fmt.Sprintf("token bucket %d @ %g/s", a.TokenBucket.Capacity, a.TokenBucket.RefillRate)
	case a.FixedWindow != nil:
		return fmt.Sprintf("fixed window %d / %s", a.FixedWindow.Limit, a.FixedWindow.Window.Duration)
	case a.SlidingWindow != nil:
		return fmt.Sprintf("sliding window (%s) %d / %s",
			a.SlidingWindow.Mode, a.SlidingWindow.Limit, a.SlidingWindow.Window.Duration,
		)
	}
	return "—"
}

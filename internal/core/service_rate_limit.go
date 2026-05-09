package core

import (
	"context"
	"errors"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
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

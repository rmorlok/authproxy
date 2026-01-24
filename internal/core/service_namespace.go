package core

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

func (s *service) EnsureNamespaceAncestorPath(ctx context.Context, targetNamespace string) (iface.Namespace, error) {
	if err := aschema.ValidateNamespacePath(targetNamespace); err != nil {
		return nil, err
	}

	var err error
	var final *database.Namespace
	for _, ns := range aschema.SplitNamespacePathToPrefixes(targetNamespace) {
		final, err = s.db.GetNamespace(ctx, ns)
		if err != nil {
			if errors.Is(err, database.ErrNotFound) {
				final = &database.Namespace{
					Path:      ns,
					State:     database.NamespaceStateActive,
					CreatedAt: apctx.GetClock(ctx).Now(),
					UpdatedAt: apctx.GetClock(ctx).Now(),
				}
				err := s.db.CreateNamespace(ctx, final)
				if err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		}
	}

	if final == nil {
		return nil, errors.New("failed to create namespace")
	}

	return wrapNamespace(*final, s), nil
}

func (s *service) GetNamespace(ctx context.Context, path string) (iface.Namespace, error) {
	ns, err := s.db.GetNamespace(ctx, path)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return wrapNamespace(*ns, s), nil
}

func (s *service) CreateNamespace(ctx context.Context, path string) (iface.Namespace, error) {
	ns := &database.Namespace{
		Path:  path,
		State: database.NamespaceStateActive,
	}

	err := s.db.CreateNamespace(ctx, ns)
	if err != nil {
		return nil, err
	}

	return wrapNamespace(*ns, s), nil
}

type listNamespaceWrapper struct {
	l database.ListNamespacesBuilder
	e database.ListNamespacesExecutor
	s *service
}

func (l *listNamespaceWrapper) convertPageResult(result pagination.PageResult[database.Namespace]) pagination.PageResult[iface.Namespace] {
	if result.Error != nil {
		return pagination.PageResult[iface.Namespace]{Error: result.Error}
	}

	versions := make([]iface.Namespace, 0, len(result.Results))
	for _, r := range result.Results {
		versions = append(versions, wrapNamespace(r, l.s))
	}

	return pagination.PageResult[iface.Namespace]{
		Results: versions,
		Error:   result.Error,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listNamespaceWrapper) executor() database.ListNamespacesExecutor {
	if l.e != nil {
		return l.e
	} else {
		return l.l
	}
}

func (l *listNamespaceWrapper) FetchPage(ctx context.Context) pagination.PageResult[iface.Namespace] {
	return l.convertPageResult(l.executor().FetchPage(ctx))
}

func (l *listNamespaceWrapper) Enumerate(ctx context.Context, callback func(pagination.PageResult[iface.Namespace]) (keepGoing bool, err error)) error {
	return l.executor().Enumerate(ctx, func(result pagination.PageResult[database.Namespace]) (keepGoing bool, err error) {
		return callback(l.convertPageResult(result))
	})
}

func (l *listNamespaceWrapper) Limit(lim int32) iface.ListNamespacesBuilder {
	return &listNamespaceWrapper{
		l: l.l.Limit(lim),
		s: l.s,
	}
}

func (l *listNamespaceWrapper) ForPathPrefix(prefix string) iface.ListNamespacesBuilder {
	return &listNamespaceWrapper{
		l: l.l.ForPathPrefix(prefix),
		s: l.s,
	}
}

func (l *listNamespaceWrapper) ForDepth(depth uint64) iface.ListNamespacesBuilder {
	return &listNamespaceWrapper{
		l: l.l.ForDepth(depth),
		s: l.s,
	}
}

func (l *listNamespaceWrapper) ForChildrenOf(prefix string) iface.ListNamespacesBuilder {
	return &listNamespaceWrapper{
		l: l.l.ForChildrenOf(prefix),
		s: l.s,
	}
}

func (l *listNamespaceWrapper) ForNamespaceMatcher(matcher string) iface.ListNamespacesBuilder {
	return &listNamespaceWrapper{
		l: l.l.ForNamespaceMatcher(matcher),
		s: l.s,
	}
}

func (l *listNamespaceWrapper) ForNamespaceMatchers(matchers []string) iface.ListNamespacesBuilder {
	return &listNamespaceWrapper{
		l: l.l.ForNamespaceMatchers(matchers),
		s: l.s,
	}
}

func (l *listNamespaceWrapper) ForState(s database.NamespaceState) iface.ListNamespacesBuilder {
	return &listNamespaceWrapper{
		l: l.l.ForState(s),
		s: l.s,
	}
}

func (l *listNamespaceWrapper) OrderBy(f database.NamespaceOrderByField, o pagination.OrderBy) iface.ListNamespacesBuilder {
	return &listNamespaceWrapper{
		l: l.l.OrderBy(f, o),
		s: l.s,
	}
}

func (l *listNamespaceWrapper) IncludeDeleted() iface.ListNamespacesBuilder {
	return &listNamespaceWrapper{
		l: l.l.IncludeDeleted(),
		s: l.s,
	}
}

func (s *service) ListNamespacesBuilder() iface.ListNamespacesBuilder {
	return &listNamespaceWrapper{
		l: s.db.ListNamespacesBuilder(),
		s: s,
	}
}

func (s *service) ListNamespacesFromCursor(ctx context.Context, cursor string) (iface.ListNamespacesExecutor, error) {
	e, err := s.db.ListNamespacesFromCursor(ctx, cursor)
	if err != nil {
		return nil, err
	}

	return &listNamespaceWrapper{
		e: e,
		s: s,
	}, nil
}

var _ iface.ListNamespacesBuilder = (*listNamespaceWrapper)(nil)

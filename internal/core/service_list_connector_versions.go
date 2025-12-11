package core

import (
	"context"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type listConnectorVersionWrapper struct {
	l database.ListConnectorVersionsBuilder
	e database.ListConnectorVersionsExecutor
	s *service
}

func (l *listConnectorVersionWrapper) convertPageResult(result pagination.PageResult[database.ConnectorVersion]) pagination.PageResult[iface.ConnectorVersion] {
	if result.Error != nil {
		return pagination.PageResult[iface.ConnectorVersion]{Error: result.Error}
	}

	versions := make([]iface.ConnectorVersion, 0, len(result.Results))
	for _, r := range result.Results {
		versions = append(versions, wrapConnectorVersion(r, l.s))
	}

	return pagination.PageResult[iface.ConnectorVersion]{
		Results: versions,
		Error:   result.Error,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listConnectorVersionWrapper) executor() database.ListConnectorVersionsExecutor {
	if l.e != nil {
		return l.e
	} else {
		return l.l
	}
}

func (l *listConnectorVersionWrapper) FetchPage(ctx context.Context) pagination.PageResult[iface.ConnectorVersion] {
	return l.convertPageResult(l.executor().FetchPage(ctx))
}

func (l *listConnectorVersionWrapper) Enumerate(ctx context.Context, callback func(pagination.PageResult[iface.ConnectorVersion]) (keepGoing bool, err error)) error {
	return l.executor().Enumerate(ctx, func(result pagination.PageResult[database.ConnectorVersion]) (keepGoing bool, err error) {
		return callback(l.convertPageResult(result))
	})
}

func (l *listConnectorVersionWrapper) Limit(lim int32) iface.ListConnectorVersionsBuilder {
	return &listConnectorVersionWrapper{
		l: l.l.Limit(lim),
		s: l.s,
	}
}

func (l *listConnectorVersionWrapper) ForType(t string) iface.ListConnectorVersionsBuilder {
	return &listConnectorVersionWrapper{
		l: l.l.ForType(t),
		s: l.s,
	}
}

func (l *listConnectorVersionWrapper) ForId(id uuid.UUID) iface.ListConnectorVersionsBuilder {
	return &listConnectorVersionWrapper{
		l: l.l.ForId(id),
		s: l.s,
	}
}

func (l *listConnectorVersionWrapper) ForVersion(version uint64) iface.ListConnectorVersionsBuilder {
	return &listConnectorVersionWrapper{
		l: l.l.ForVersion(version),
		s: l.s,
	}
}

func (l *listConnectorVersionWrapper) ForState(s database.ConnectorVersionState) iface.ListConnectorVersionsBuilder {
	return &listConnectorVersionWrapper{
		l: l.l.ForState(s),
		s: l.s,
	}
}

func (l *listConnectorVersionWrapper) ForStates(states []database.ConnectorVersionState) iface.ListConnectorVersionsBuilder {
	return &listConnectorVersionWrapper{
		l: l.l.ForStates(states),
		s: l.s,
	}
}

func (l *listConnectorVersionWrapper) ForNamespaceMatcher(m string) iface.ListConnectorVersionsBuilder {
	return &listConnectorVersionWrapper{
		l: l.l.ForNamespaceMatcher(m),
		s: l.s,
	}
}

func (l *listConnectorVersionWrapper) OrderBy(f database.ConnectorVersionOrderByField, o pagination.OrderBy) iface.ListConnectorVersionsBuilder {
	return &listConnectorVersionWrapper{
		l: l.l.OrderBy(f, o),
		s: l.s,
	}
}

func (l *listConnectorVersionWrapper) IncludeDeleted() iface.ListConnectorVersionsBuilder {
	return &listConnectorVersionWrapper{
		l: l.l.IncludeDeleted(),
		s: l.s,
	}
}

func (s *service) ListConnectorVersionsBuilder() iface.ListConnectorVersionsBuilder {
	return &listConnectorVersionWrapper{
		l: s.db.ListConnectorVersionsBuilder(),
		s: s,
	}
}

func (s *service) ListConnectorVersionsFromCursor(ctx context.Context, cursor string) (iface.ListConnectorVersionsExecutor, error) {
	e, err := s.db.ListConnectorVersionsFromCursor(ctx, cursor)
	if err != nil {
		return nil, err
	}

	return &listConnectorVersionWrapper{
		e: e,
		s: s,
	}, nil
}

var _ iface.ListConnectorVersionsBuilder = (*listConnectorVersionWrapper)(nil)

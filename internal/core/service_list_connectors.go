package core

import (
	"context"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

func wrapConnector(c database.Connector, s *service) *Connector {
	return &Connector{
		ConnectorVersion: *wrapConnectorVersion(c.ConnectorVersion, s),
		TotalVersions:    c.TotalVersions,
		States:           c.States,
	}
}

type listWrapper struct {
	l database.ListConnectorsBuilder
	e database.ListConnectorsExecutor
	s *service
}

func (l *listWrapper) convertPageResult(result pagination.PageResult[database.Connector]) pagination.PageResult[iface.Connector] {
	if result.Error != nil {
		return pagination.PageResult[iface.Connector]{Error: result.Error}
	}

	versions := make([]iface.Connector, 0, len(result.Results))
	for _, r := range result.Results {
		versions = append(versions, wrapConnector(r, l.s))
	}

	return pagination.PageResult[iface.Connector]{
		Results: versions,
		Error:   result.Error,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listWrapper) executor() database.ListConnectorsExecutor {
	if l.e != nil {
		return l.e
	} else {
		return l.l
	}
}

func (l *listWrapper) FetchPage(ctx context.Context) pagination.PageResult[iface.Connector] {
	return l.convertPageResult(l.executor().FetchPage(ctx))
}

func (l *listWrapper) Enumerate(ctx context.Context, callback func(pagination.PageResult[iface.Connector]) (keepGoing bool, err error)) error {
	return l.executor().Enumerate(ctx, func(result pagination.PageResult[database.Connector]) (keepGoing bool, err error) {
		return callback(l.convertPageResult(result))
	})
}

func (l *listWrapper) Limit(lim int32) iface.ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.Limit(lim),
		s: l.s,
	}
}

func (l *listWrapper) ForType(t string) iface.ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.ForType(t),
		s: l.s,
	}
}

func (l *listWrapper) ForId(id uuid.UUID) iface.ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.ForId(id),
		s: l.s,
	}
}

func (l *listWrapper) ForState(s database.ConnectorVersionState) iface.ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.ForState(s),
		s: l.s,
	}
}

func (l *listWrapper) ForStates(states []database.ConnectorVersionState) iface.ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.ForStates(states),
		s: l.s,
	}
}

func (l *listWrapper) ForNamespaceMatcher(m string) iface.ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.ForNamespaceMatcher(m),
		s: l.s,
	}
}

func (l *listWrapper) OrderBy(f database.ConnectorOrderByField, o pagination.OrderBy) iface.ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.OrderBy(f, o),
		s: l.s,
	}
}

func (l *listWrapper) IncludeDeleted() iface.ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.IncludeDeleted(),
		s: l.s,
	}
}

func (s *service) ListConnectorsBuilder() iface.ListConnectorsBuilder {
	return &listWrapper{
		l: s.db.ListConnectorsBuilder(),
		s: s,
	}
}

func (s *service) ListConnectorsFromCursor(ctx context.Context, cursor string) (iface.ListConnectorsExecutor, error) {
	e, err := s.db.ListConnectorsFromCursor(ctx, cursor)
	if err != nil {
		return nil, err
	}

	return &listWrapper{
		e: e,
		s: s,
	}, nil
}

var _ iface.ListConnectorsBuilder = (*listWrapper)(nil)

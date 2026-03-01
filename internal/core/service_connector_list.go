package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
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

type listConnectorWrapper struct {
	l database.ListConnectorsBuilder
	e database.ListConnectorsExecutor
	s *service
}

func (l *listConnectorWrapper) convertPageResult(result pagination.PageResult[database.Connector]) pagination.PageResult[iface.Connector] {
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

func (l *listConnectorWrapper) executor() database.ListConnectorsExecutor {
	if l.e != nil {
		return l.e
	} else {
		return l.l
	}
}

func (l *listConnectorWrapper) FetchPage(ctx context.Context) pagination.PageResult[iface.Connector] {
	return l.convertPageResult(l.executor().FetchPage(ctx))
}

func (l *listConnectorWrapper) Enumerate(ctx context.Context, callback func(pagination.PageResult[iface.Connector]) (keepGoing bool, err error)) error {
	return l.executor().Enumerate(ctx, func(result pagination.PageResult[database.Connector]) (keepGoing bool, err error) {
		return callback(l.convertPageResult(result))
	})
}

func (l *listConnectorWrapper) Limit(lim int32) iface.ListConnectorsBuilder {
	return &listConnectorWrapper{
		l: l.l.Limit(lim),
		s: l.s,
	}
}

func (l *listConnectorWrapper) ForType(t string) iface.ListConnectorsBuilder {
	return &listConnectorWrapper{
		l: l.l.ForType(t),
		s: l.s,
	}
}

func (l *listConnectorWrapper) ForId(id apid.ID) iface.ListConnectorsBuilder {
	return &listConnectorWrapper{
		l: l.l.ForId(id),
		s: l.s,
	}
}

func (l *listConnectorWrapper) ForState(s database.ConnectorVersionState) iface.ListConnectorsBuilder {
	return &listConnectorWrapper{
		l: l.l.ForState(s),
		s: l.s,
	}
}

func (l *listConnectorWrapper) ForStates(states []database.ConnectorVersionState) iface.ListConnectorsBuilder {
	return &listConnectorWrapper{
		l: l.l.ForStates(states),
		s: l.s,
	}
}

func (l *listConnectorWrapper) ForNamespaceMatcher(m string) iface.ListConnectorsBuilder {
	return &listConnectorWrapper{
		l: l.l.ForNamespaceMatcher(m),
		s: l.s,
	}
}

func (l *listConnectorWrapper) ForNamespaceMatchers(matchers []string) iface.ListConnectorsBuilder {
	return &listConnectorWrapper{
		l: l.l.ForNamespaceMatchers(matchers),
		s: l.s,
	}
}

func (l *listConnectorWrapper) OrderBy(f database.ConnectorOrderByField, o pagination.OrderBy) iface.ListConnectorsBuilder {
	return &listConnectorWrapper{
		l: l.l.OrderBy(f, o),
		s: l.s,
	}
}

func (l *listConnectorWrapper) IncludeDeleted() iface.ListConnectorsBuilder {
	return &listConnectorWrapper{
		l: l.l.IncludeDeleted(),
		s: l.s,
	}
}

func (l *listConnectorWrapper) ForLabelSelector(s string) iface.ListConnectorsBuilder {
	return &listConnectorWrapper{
		l: l.l.ForLabelSelector(s),
		s: l.s,
	}
}

func (s *service) ListConnectorsBuilder() iface.ListConnectorsBuilder {
	return &listConnectorWrapper{
		l: s.db.ListConnectorsBuilder(),
		s: s,
	}
}

func (s *service) ListConnectorsFromCursor(ctx context.Context, cursor string) (iface.ListConnectorsExecutor, error) {
	e, err := s.db.ListConnectorsFromCursor(ctx, cursor)
	if err != nil {
		return nil, err
	}

	return &listConnectorWrapper{
		e: e,
		s: s,
	}, nil
}

var _ iface.ListConnectorsBuilder = (*listConnectorWrapper)(nil)

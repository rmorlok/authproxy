package core

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type listConnectionsWrapper struct {
	l  database.ListConnectionsBuilder
	e  database.ListConnectionsExecutor
	cc map[iface.ConnectorVersionId]*ConnectorVersion
	s  *service
}

func (l *listConnectionsWrapper) cloneWithBuilder(newL database.ListConnectionsBuilder) *listConnectionsWrapper {
	return &listConnectionsWrapper{
		l:  newL,
		e:  l.e,
		cc: l.cc,
		s:  l.s,
	}
}

func (l *listConnectionsWrapper) convertPageResult(ctx context.Context, result pagination.PageResult[database.Connection]) pagination.PageResult[iface.Connection] {
	if result.Error != nil {
		return pagination.PageResult[iface.Connection]{Error: result.Error}
	}

	allNeededConnectorVersionIds := GetConnectorVersionIdsForConnections(result.Results)
	toLoadConnectorVersions := make([]iface.ConnectorVersionId, 0, len(allNeededConnectorVersionIds))
	for _, id := range allNeededConnectorVersionIds {
		// Check if we already have the connector version loaded
		if _, ok := l.cc[id]; !ok {
			toLoadConnectorVersions = append(toLoadConnectorVersions, id)
		}
	}

	versions, err := l.s.getConnectorVersions(ctx, toLoadConnectorVersions)
	if err != nil {
		return pagination.PageResult[iface.Connection]{Error: err}
	}

	for _, v := range versions {
		if l.cc == nil {
			l.cc = make(map[iface.ConnectorVersionId]*ConnectorVersion)
		}

		l.cc[iface.ConnectorVersionId{Id: v.GetID(), Version: v.GetVersion()}] = v
	}

	connections := make([]iface.Connection, 0, len(result.Results))
	for _, r := range result.Results {
		if cv, ok := l.cc[iface.ConnectorVersionId{Id: r.ConnectorId, Version: r.ConnectorVersion}]; ok {
			connections = append(connections, wrapConnection(&r, cv, l.s))
		} else {
			return pagination.PageResult[iface.Connection]{
				Error: errors.Errorf("could not find connector version %s:%d", r.ConnectorId, r.ConnectorVersion),
			}
		}
	}

	return pagination.PageResult[iface.Connection]{
		Results: connections,
		Error:   result.Error,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listConnectionsWrapper) executor() database.ListConnectionsExecutor {
	if l.e != nil {
		return l.e
	} else {
		return l.l
	}
}

func (l *listConnectionsWrapper) FetchPage(ctx context.Context) pagination.PageResult[iface.Connection] {
	return l.convertPageResult(ctx, l.executor().FetchPage(ctx))
}

func (l *listConnectionsWrapper) Enumerate(ctx context.Context, callback func(pagination.PageResult[iface.Connection]) (keepGoing bool, err error)) error {
	return l.executor().Enumerate(ctx, func(result pagination.PageResult[database.Connection]) (keepGoing bool, err error) {
		return callback(l.convertPageResult(ctx, result))
	})
}

func (l *listConnectionsWrapper) Limit(lim int32) iface.ListConnectionsBuilder {
	return l.cloneWithBuilder(l.l.Limit(lim))
}

func (l *listConnectionsWrapper) ForState(s database.ConnectionState) iface.ListConnectionsBuilder {
	return l.cloneWithBuilder(l.l.ForState(s))
}

func (l *listConnectionsWrapper) ForStates(states []database.ConnectionState) iface.ListConnectionsBuilder {
	return l.cloneWithBuilder(l.l.ForStates(states))
}

func (l *listConnectionsWrapper) ForNamespaceMatcher(m string) iface.ListConnectionsBuilder {
	return l.cloneWithBuilder(l.l.ForNamespaceMatcher(m))
}

func (l *listConnectionsWrapper) OrderBy(f database.ConnectionOrderByField, o pagination.OrderBy) iface.ListConnectionsBuilder {
	return l.cloneWithBuilder(l.l.OrderBy(f, o))
}

func (l *listConnectionsWrapper) IncludeDeleted() iface.ListConnectionsBuilder {
	return l.cloneWithBuilder(l.l.IncludeDeleted())
}

func (l *listConnectionsWrapper) WithDeletedHandling(h database.DeletedHandling) iface.ListConnectionsBuilder {
	return l.cloneWithBuilder(l.l.WithDeletedHandling(h))
}

func (s *service) ListConnectionsBuilder() iface.ListConnectionsBuilder {
	return &listConnectionsWrapper{
		l: s.db.ListConnectionsBuilder(),
		s: s,
	}
}

func (s *service) ListConnectionsFromCursor(ctx context.Context, cursor string) (iface.ListConnectionsExecutor, error) {
	e, err := s.db.ListConnectionsFromCursor(ctx, cursor)
	if err != nil {
		return nil, err
	}

	return &listConnectionsWrapper{
		e: e,
		s: s,
	}, nil
}

var _ iface.ListConnectionsBuilder = (*listConnectionsWrapper)(nil)

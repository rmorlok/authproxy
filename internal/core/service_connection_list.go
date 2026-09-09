package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type listConnectionsWrapper struct {
	l  database.ListConnectionsBuilder
	e  database.ListConnectionsExecutor
	cc map[iface.ConnectorGenerationId]*Connector
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

	allNeededConnectorGenerationIds := GetConnectorGenerationIdsForConnections(result.Results)
	toLoadConnectorGenerations := make([]iface.ConnectorGenerationId, 0, len(allNeededConnectorGenerationIds))
	for _, id := range allNeededConnectorGenerationIds {
		// Check if we already have the connector generation loaded
		if _, ok := l.cc[id]; !ok {
			toLoadConnectorGenerations = append(toLoadConnectorGenerations, id)
		}
	}

	generations, err := l.s.getConnectorGenerations(ctx, toLoadConnectorGenerations)
	if err != nil {
		return pagination.PageResult[iface.Connection]{Error: err}
	}

	for _, v := range generations {
		if l.cc == nil {
			l.cc = make(map[iface.ConnectorGenerationId]*Connector)
		}

		l.cc[iface.ConnectorGenerationId{Id: v.GetId(), Generation: v.GetGeneration()}] = v
	}

	connections := make([]iface.Connection, 0, len(result.Results))
	for _, r := range result.Results {
		if c, ok := l.cc[iface.ConnectorGenerationId{Id: r.ConnectorId, Generation: r.ConnectorGeneration}]; ok {
			connections = append(connections, wrapConnection(&r, c, l.s))
		} else {
			return pagination.PageResult[iface.Connection]{
				Error: fmt.Errorf("could not find connector generation %s:%d", r.ConnectorId, r.ConnectorGeneration),
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

func (l *listConnectionsWrapper) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[iface.Connection]) error {
	return l.executor().Enumerate(ctx, func(result pagination.PageResult[database.Connection]) (keepGoing pagination.KeepGoing, err error) {
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

func (l *listConnectionsWrapper) ForConnectorId(id apid.ID) iface.ListConnectionsBuilder {
	return l.cloneWithBuilder(l.l.ForConnectorId(id))
}

func (l *listConnectionsWrapper) ForNamespaceMatcher(m string) iface.ListConnectionsBuilder {
	return l.cloneWithBuilder(l.l.ForNamespaceMatcher(m))
}

func (l *listConnectionsWrapper) ForNamespaceMatchers(matchers []string) iface.ListConnectionsBuilder {
	return l.cloneWithBuilder(l.l.ForNamespaceMatchers(matchers))
}

func (l *listConnectionsWrapper) ForName(name common.ResourceName) iface.ListConnectionsBuilder {
	return l.cloneWithBuilder(l.l.ForName(name))
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

func (l *listConnectionsWrapper) ForLabelSelector(s string) iface.ListConnectionsBuilder {
	return l.cloneWithBuilder(l.l.ForLabelSelector(s))
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

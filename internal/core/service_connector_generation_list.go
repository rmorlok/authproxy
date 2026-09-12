package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type listConnectorGenerationWrapper struct {
	l database.ListConnectorGenerationsBuilder
	e database.ListConnectorGenerationsExecutor
	s *service
}

func (l *listConnectorGenerationWrapper) convertPageResult(result pagination.PageResult[database.ConnectorWithDefinition]) pagination.PageResult[iface.Connector] {
	if result.Error != nil {
		return pagination.PageResult[iface.Connector]{Error: result.Error}
	}

	generations := make([]iface.Connector, 0, len(result.Results))
	for _, r := range result.Results {
		generations = append(generations, wrapConnector(r, l.s))
	}

	return pagination.PageResult[iface.Connector]{
		Results: generations,
		Error:   result.Error,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listConnectorGenerationWrapper) executor() database.ListConnectorGenerationsExecutor {
	if l.e != nil {
		return l.e
	} else {
		return l.l
	}
}

func (l *listConnectorGenerationWrapper) FetchPage(ctx context.Context) pagination.PageResult[iface.Connector] {
	return l.convertPageResult(l.executor().FetchPage(ctx))
}

func (l *listConnectorGenerationWrapper) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[iface.Connector]) error {
	return l.executor().Enumerate(ctx, func(result pagination.PageResult[database.ConnectorWithDefinition]) (keepGoing pagination.KeepGoing, err error) {
		return callback(l.convertPageResult(result))
	})
}

func (l *listConnectorGenerationWrapper) Limit(lim int32) iface.ListConnectorGenerationsBuilder {
	return &listConnectorGenerationWrapper{
		l: l.l.Limit(lim),
		s: l.s,
	}
}

func (l *listConnectorGenerationWrapper) ForId(id apid.ID) iface.ListConnectorGenerationsBuilder {
	return &listConnectorGenerationWrapper{
		l: l.l.ForId(id),
		s: l.s,
	}
}

func (l *listConnectorGenerationWrapper) ForGeneration(generation uint64) iface.ListConnectorGenerationsBuilder {
	return &listConnectorGenerationWrapper{
		l: l.l.ForGeneration(generation),
		s: l.s,
	}
}

func (l *listConnectorGenerationWrapper) ForState(s database.ConnectorGenerationState) iface.ListConnectorGenerationsBuilder {
	return &listConnectorGenerationWrapper{
		l: l.l.ForState(s),
		s: l.s,
	}
}

func (l *listConnectorGenerationWrapper) ForStates(states []database.ConnectorGenerationState) iface.ListConnectorGenerationsBuilder {
	return &listConnectorGenerationWrapper{
		l: l.l.ForStates(states),
		s: l.s,
	}
}

func (l *listConnectorGenerationWrapper) ForNamespaceMatcher(m string) iface.ListConnectorGenerationsBuilder {
	return &listConnectorGenerationWrapper{
		l: l.l.ForNamespaceMatcher(m),
		s: l.s,
	}
}

func (l *listConnectorGenerationWrapper) ForNamespaceMatchers(matchers []string) iface.ListConnectorGenerationsBuilder {
	return &listConnectorGenerationWrapper{
		l: l.l.ForNamespaceMatchers(matchers),
		s: l.s,
	}
}

func (l *listConnectorGenerationWrapper) ForName(name common.ResourceName) iface.ListConnectorGenerationsBuilder {
	return &listConnectorGenerationWrapper{
		l: l.l.ForName(name),
		s: l.s,
	}
}

func (l *listConnectorGenerationWrapper) OrderBy(f database.ConnectorGenerationOrderByField, o pagination.OrderBy) iface.ListConnectorGenerationsBuilder {
	return &listConnectorGenerationWrapper{
		l: l.l.OrderBy(f, o),
		s: l.s,
	}
}

func (l *listConnectorGenerationWrapper) IncludeDeleted() iface.ListConnectorGenerationsBuilder {
	return &listConnectorGenerationWrapper{
		l: l.l.IncludeDeleted(),
		s: l.s,
	}
}

func (l *listConnectorGenerationWrapper) ForLabelSelector(s string) iface.ListConnectorGenerationsBuilder {
	return &listConnectorGenerationWrapper{
		l: l.l.ForLabelSelector(s),
		s: l.s,
	}
}

func (s *service) ListConnectorGenerationsBuilder() iface.ListConnectorGenerationsBuilder {
	return &listConnectorGenerationWrapper{
		l: s.db.ListConnectorGenerationsBuilder(),
		s: s,
	}
}

func (s *service) ListConnectorGenerationsFromCursor(ctx context.Context, cursor string) (iface.ListConnectorGenerationsExecutor, error) {
	e, err := s.db.ListConnectorGenerationsFromCursor(ctx, cursor)
	if err != nil {
		return nil, err
	}

	return &listConnectorGenerationWrapper{
		e: e,
		s: s,
	}, nil
}

var _ iface.ListConnectorGenerationsBuilder = (*listConnectorGenerationWrapper)(nil)

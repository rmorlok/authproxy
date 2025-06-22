package connectors

import (
	"context"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/database"
)

// Connector object is returned from queries for connectors, with one record per id. It aggregates some information
// across all versions for a connector.
type Connector struct {
	ConnectorVersion
	TotalVersions int64
	States        database.ConnectorVersionStates
}

func wrapConnector(c database.Connector, s *service) *Connector {
	return &Connector{
		ConnectorVersion: *wrapConnectorVersion(c.ConnectorVersion, s),
		TotalVersions:    c.TotalVersions,
		States:           c.States,
	}
}

/*
 * These interfaces wrap the database equivalents so this service can provide decryption and potentially caching.
 */

type ListConnectorsExecutor interface {
	FetchPage(context.Context) database.PageResult[*Connector]
	Enumerate(context.Context, func(database.PageResult[*Connector]) (keepGoing bool, err error)) error
}

type ListConnectorsBuilder interface {
	ListConnectorsExecutor
	Limit(int32) ListConnectorsBuilder
	ForType(string) ListConnectorsBuilder
	ForId(uuid.UUID) ListConnectorsBuilder
	ForConnectorVersionState(database.ConnectorVersionState) ListConnectorsBuilder
	OrderBy(database.ConnectorOrderByField, database.OrderBy) ListConnectorsBuilder
	IncludeDeleted() ListConnectorsBuilder
}

type listWrapper struct {
	l database.ListConnectorsBuilder
	e database.ListConnectorsExecutor
	s *service
}

func (l *listWrapper) convertPageResult(result database.PageResult[database.Connector]) database.PageResult[*Connector] {
	if result.Error != nil {
		return database.PageResult[*Connector]{Error: result.Error}
	}

	versions := make([]*Connector, 0, len(result.Results))
	for _, r := range result.Results {
		versions = append(versions, wrapConnector(r, l.s))
	}

	return database.PageResult[*Connector]{
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

func (l *listWrapper) FetchPage(ctx context.Context) database.PageResult[*Connector] {
	return l.convertPageResult(l.executor().FetchPage(ctx))
}

func (l *listWrapper) Enumerate(ctx context.Context, callback func(database.PageResult[*Connector]) (keepGoing bool, err error)) error {
	return l.executor().Enumerate(ctx, func(result database.PageResult[database.Connector]) (keepGoing bool, err error) {
		return callback(l.convertPageResult(result))
	})
}

func (l *listWrapper) Limit(lim int32) ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.Limit(lim),
		s: l.s,
	}
}

func (l *listWrapper) ForType(t string) ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.ForType(t),
		s: l.s,
	}
}

func (l *listWrapper) ForId(id uuid.UUID) ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.ForId(id),
		s: l.s,
	}
}

func (l *listWrapper) ForConnectorVersionState(s database.ConnectorVersionState) ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.ForConnectorVersionState(s),
		s: l.s,
	}
}

func (l *listWrapper) OrderBy(f database.ConnectorOrderByField, o database.OrderBy) ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.OrderBy(f, o),
		s: l.s,
	}
}

func (l *listWrapper) IncludeDeleted() ListConnectorsBuilder {
	return &listWrapper{
		l: l.l.IncludeDeleted(),
		s: l.s,
	}
}

func (s *service) ListConnectorsBuilder() ListConnectorsBuilder {
	return &listWrapper{
		l: s.db.ListConnectorsBuilder(),
		s: s,
	}
}

func (s *service) ListConnectorsFromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error) {
	e, err := s.db.ListConnectorsFromCursor(ctx, cursor)
	if err != nil {
		return nil, err
	}

	return &listWrapper{
		e: e,
		s: s,
	}, nil
}

var _ ListConnectorsBuilder = (*listWrapper)(nil)

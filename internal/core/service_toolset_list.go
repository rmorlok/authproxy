package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// listToolSetsWrapper adapts a database logical ToolSet listing to default
// generation projections. Exactly one of l or e is set: l while building a
// listing, e when resuming a cursor.
type listToolSetsWrapper struct {
	l database.ListToolSetsBuilder
	e database.ListToolSetsExecutor
	s *service
}

// convertPageResult renders each logical ToolSet's default projection. This reads
// each ToolSet's generations separately; pages are bounded. A page can therefore
// hold fewer items than its limit while HasMore is still true.
func (l *listToolSetsWrapper) convertPageResult(
	ctx context.Context,
	result pagination.PageResult[database.ToolSet],
) pagination.PageResult[iface.ToolSet] {
	if result.Error != nil {
		return pagination.PageResult[iface.ToolSet]{Error: mapToolListError(result.Error)}
	}

	items := make([]iface.ToolSet, 0, len(result.Results))
	for i := range result.Results {
		view, err := l.s.defaultToolSetView(ctx, &result.Results[i])
		if errors.Is(err, ErrNotFound) {
			// A ToolSet deleted after the page was read is skipped.
			continue
		}
		if err != nil {
			return pagination.PageResult[iface.ToolSet]{Error: err}
		}
		items = append(items, view)
	}

	return pagination.PageResult[iface.ToolSet]{
		Results: items,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listToolSetsWrapper) executor() database.ListToolSetsExecutor {
	if l.e != nil {
		return l.e
	}
	return l.l
}

func (l *listToolSetsWrapper) FetchPage(ctx context.Context) pagination.PageResult[iface.ToolSet] {
	return l.convertPageResult(ctx, l.executor().FetchPage(ctx))
}

// Enumerate stops on a hydration failure rather than handing the callback a page
// that holds only an error.
func (l *listToolSetsWrapper) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[iface.ToolSet]) error {
	err := l.executor().Enumerate(ctx, func(result pagination.PageResult[database.ToolSet]) (pagination.KeepGoing, error) {
		page := l.convertPageResult(ctx, result)
		if page.Error != nil {
			return pagination.Stop, page.Error
		}
		return callback(page)
	})
	return mapToolListError(err)
}

func (l *listToolSetsWrapper) Limit(limit int32) iface.ListToolSetsBuilder {
	return &listToolSetsWrapper{l: l.l.Limit(limit), s: l.s}
}

func (l *listToolSetsWrapper) ForNamespaceMatcher(matcher string) iface.ListToolSetsBuilder {
	return &listToolSetsWrapper{l: l.l.ForNamespaceMatcher(matcher), s: l.s}
}

func (l *listToolSetsWrapper) ForNamespaceMatchers(matchers []string) iface.ListToolSetsBuilder {
	return &listToolSetsWrapper{l: l.l.ForNamespaceMatchers(matchers), s: l.s}
}

func (l *listToolSetsWrapper) ForName(name common.ResourceName) iface.ListToolSetsBuilder {
	return &listToolSetsWrapper{l: l.l.ForName(name), s: l.s}
}

func (l *listToolSetsWrapper) ForLabelSelector(selector string) iface.ListToolSetsBuilder {
	return &listToolSetsWrapper{l: l.l.ForLabelSelector(selector), s: l.s}
}

func (l *listToolSetsWrapper) OrderBy(field database.ToolSetOrderByField, by pagination.OrderBy) iface.ListToolSetsBuilder {
	return &listToolSetsWrapper{l: l.l.OrderBy(field, by), s: l.s}
}

// ListToolSetsBuilder starts a listing of logical ToolSets.
func (s *service) ListToolSetsBuilder() iface.ListToolSetsBuilder {
	return &listToolSetsWrapper{l: s.db.ListToolSetsBuilder(), s: s}
}

// ListToolSetsFromCursor continues a logical listing with its saved filters.
func (s *service) ListToolSetsFromCursor(ctx context.Context, cursor string) (iface.ListToolSetsExecutor, error) {
	e, err := s.db.ListToolSetsFromCursor(ctx, cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}
	return &listToolSetsWrapper{e: e, s: s}, nil
}

// listToolSetGenerationsWrapper adapts a database generation listing to ToolSet
// views. Exactly one of l or e is set: l while building, e when resuming a cursor.
type listToolSetGenerationsWrapper struct {
	l database.ListToolSetGenerationsBuilder
	e database.ListToolSetGenerationsExecutor
	s *service
}

// convertPageResult decrypts each generation against its logical record, so
// every item projects today's selector.
func (l *listToolSetGenerationsWrapper) convertPageResult(
	ctx context.Context,
	result pagination.PageResult[database.ToolSetGeneration],
) pagination.PageResult[iface.ToolSet] {
	if result.Error != nil {
		return pagination.PageResult[iface.ToolSet]{Error: mapToolListError(result.Error)}
	}

	items := make([]iface.ToolSet, 0, len(result.Results))
	if len(result.Results) > 0 {
		logical, err := l.s.db.GetToolSet(ctx, result.Results[0].ToolSetId)
		if err != nil {
			return pagination.PageResult[iface.ToolSet]{Error: mapToolSetDatabaseError(err)}
		}

		for i := range result.Results {
			view, err := l.s.hydrateToolSet(ctx, logical, &result.Results[i])
			if err != nil {
				return pagination.PageResult[iface.ToolSet]{Error: err}
			}
			items = append(items, view)
		}
	}

	return pagination.PageResult[iface.ToolSet]{
		Results: items,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listToolSetGenerationsWrapper) executor() database.ListToolSetGenerationsExecutor {
	if l.e != nil {
		return l.e
	}
	return l.l
}

func (l *listToolSetGenerationsWrapper) FetchPage(ctx context.Context) pagination.PageResult[iface.ToolSet] {
	return l.convertPageResult(ctx, l.executor().FetchPage(ctx))
}

// Enumerate stops on a hydration failure rather than handing the callback a page
// that holds only an error.
func (l *listToolSetGenerationsWrapper) Enumerate(
	ctx context.Context,
	callback pagination.EnumerateCallback[iface.ToolSet],
) error {
	err := l.executor().Enumerate(ctx, func(result pagination.PageResult[database.ToolSetGeneration]) (pagination.KeepGoing, error) {
		page := l.convertPageResult(ctx, result)
		if page.Error != nil {
			return pagination.Stop, page.Error
		}
		return callback(page)
	})
	return mapToolListError(err)
}

func (l *listToolSetGenerationsWrapper) Limit(limit int32) iface.ListToolSetGenerationsBuilder {
	return &listToolSetGenerationsWrapper{l: l.l.Limit(limit), s: l.s}
}

func (l *listToolSetGenerationsWrapper) ForToolSetId(id apid.ID) iface.ListToolSetGenerationsBuilder {
	return &listToolSetGenerationsWrapper{l: l.l.ForToolSetId(id), s: l.s}
}

func (l *listToolSetGenerationsWrapper) OrderBy(
	field database.ToolSetGenerationOrderByField,
	by pagination.OrderBy,
) iface.ListToolSetGenerationsBuilder {
	return &listToolSetGenerationsWrapper{l: l.l.OrderBy(field, by), s: l.s}
}

// ListToolSetGenerationsBuilder starts a listing of one ToolSet's generations.
func (s *service) ListToolSetGenerationsBuilder() iface.ListToolSetGenerationsBuilder {
	return &listToolSetGenerationsWrapper{l: s.db.ListToolSetGenerationsBuilder(), s: s}
}

// ListToolSetGenerationsFromCursor continues a generation listing. A parent
// deleted since the cursor was issued surfaces as ErrNotFound from FetchPage.
func (s *service) ListToolSetGenerationsFromCursor(
	ctx context.Context,
	cursor string,
) (iface.ListToolSetGenerationsExecutor, error) {
	e, err := s.db.ListToolSetGenerationsFromCursor(ctx, cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}
	return &listToolSetGenerationsWrapper{e: e, s: s}, nil
}

var (
	_ iface.ListToolSetsBuilder           = (*listToolSetsWrapper)(nil)
	_ iface.ListToolSetGenerationsBuilder = (*listToolSetGenerationsWrapper)(nil)
)

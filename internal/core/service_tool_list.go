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

// mapToolListError reports an invalid Tool or ToolSet filter as a caller error.
// Anything else, such as a database failure, keeps its normal mapping.
func mapToolListError(err error) error {
	if errors.Is(err, database.ErrInvalidListQuery) {
		return fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}
	return mapToolSetDatabaseError(err)
}

// listToolsWrapper adapts a database Tool listing to hydrated core Tools. Exactly
// one of l or e is set: l while building a listing, e when resuming a cursor.
type listToolsWrapper struct {
	l database.ListToolsBuilder
	e database.ListToolsExecutor
	s *service
}

// convertPageResult decrypts each listed Tool's active definition. Any failure
// replaces the whole page with an error.
func (l *listToolsWrapper) convertPageResult(
	ctx context.Context,
	result pagination.PageResult[database.Tool],
) pagination.PageResult[iface.Tool] {
	if result.Error != nil {
		return pagination.PageResult[iface.Tool]{Error: mapToolListError(result.Error)}
	}

	tools := make([]iface.Tool, 0, len(result.Results))
	for i := range result.Results {
		tool, err := l.s.hydrateTool(ctx, &result.Results[i])
		if err != nil {
			return pagination.PageResult[iface.Tool]{Error: err}
		}
		tools = append(tools, tool)
	}

	return pagination.PageResult[iface.Tool]{
		Results: tools,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listToolsWrapper) executor() database.ListToolsExecutor {
	if l.e != nil {
		return l.e
	}
	return l.l
}

func (l *listToolsWrapper) FetchPage(ctx context.Context) pagination.PageResult[iface.Tool] {
	return l.convertPageResult(ctx, l.executor().FetchPage(ctx))
}

// Enumerate stops on a hydration failure rather than handing the callback a page
// that holds only an error.
func (l *listToolsWrapper) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[iface.Tool]) error {
	err := l.executor().Enumerate(ctx, func(result pagination.PageResult[database.Tool]) (pagination.KeepGoing, error) {
		page := l.convertPageResult(ctx, result)
		if page.Error != nil {
			return pagination.Stop, page.Error
		}
		return callback(page)
	})
	return mapToolListError(err)
}

func (l *listToolsWrapper) Limit(limit int32) iface.ListToolsBuilder {
	return &listToolsWrapper{l: l.l.Limit(limit), s: l.s}
}

func (l *listToolsWrapper) ForNamespaceMatcher(matcher string) iface.ListToolsBuilder {
	return &listToolsWrapper{l: l.l.ForNamespaceMatcher(matcher), s: l.s}
}

func (l *listToolsWrapper) ForNamespaceMatchers(matchers []string) iface.ListToolsBuilder {
	return &listToolsWrapper{l: l.l.ForNamespaceMatchers(matchers), s: l.s}
}

func (l *listToolsWrapper) ForName(name common.ResourceName) iface.ListToolsBuilder {
	return &listToolsWrapper{l: l.l.ForName(name), s: l.s}
}

func (l *listToolsWrapper) ForConnectionId(id apid.ID) iface.ListToolsBuilder {
	return &listToolsWrapper{l: l.l.ForConnectionId(id), s: l.s}
}

func (l *listToolsWrapper) ForLabelSelector(selector string) iface.ListToolsBuilder {
	return &listToolsWrapper{l: l.l.ForLabelSelector(selector), s: l.s}
}

func (l *listToolsWrapper) OrderBy(field database.ToolOrderByField, by pagination.OrderBy) iface.ListToolsBuilder {
	return &listToolsWrapper{l: l.l.OrderBy(field, by), s: l.s}
}

// ListToolsBuilder starts a hydrated management Tool listing.
func (s *service) ListToolsBuilder() iface.ListToolsBuilder {
	return &listToolsWrapper{l: s.db.ListToolsBuilder(), s: s}
}

// ListToolsFromCursor continues a listing with the filters saved in the cursor.
func (s *service) ListToolsFromCursor(ctx context.Context, cursor string) (iface.ListToolsExecutor, error) {
	e, err := s.db.ListToolsFromCursor(ctx, cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid cursor: %w", ErrInvalidArgument, err)
	}
	return &listToolsWrapper{e: e, s: s}, nil
}

var _ iface.ListToolsBuilder = (*listToolsWrapper)(nil)

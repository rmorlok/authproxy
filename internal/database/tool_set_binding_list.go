package database

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// MaxToolSetBindingListLimit bounds one page of a connection's ToolSet bindings.
const MaxToolSetBindingListLimit = 100

// ToolSetBindingListing is one live assignment with the parent identity a
// management projection needs. The namespaces are read with the binding, so a
// caller can authorize both parents without loading either one.
type ToolSetBindingListing struct {
	ToolSetBinding
	ConnectionNamespace string
	ToolSetNamespace    string
	ToolSetName         common.ResourceName
}

// ListToolSetBindingsExecutor runs a binding listing one page at a time or across
// every remaining page. Each FetchPage advances the listing, and Enumerate stops
// at the first error.
type ListToolSetBindingsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[ToolSetBindingListing]
	Enumerate(context.Context, pagination.EnumerateCallback[ToolSetBindingListing]) error
}

// ListToolSetBindingsBuilder pages one connection's live assignments whose two
// parents are live, ordered by ToolSet ID. ForConnectionId is required.
//
// ToolSet namespace scope is required too: with no matchers the listing is empty,
// so a caller with no ToolSet access never learns what is bound. Invalid filters
// are reported as ErrInvalidListQuery when the listing runs. Callers must still
// apply per-resource permission checks.
type ListToolSetBindingsBuilder interface {
	ListToolSetBindingsExecutor

	// Limit sets the page size. Non-positive values select the default and values
	// above MaxToolSetBindingListLimit are capped.
	Limit(int32) ListToolSetBindingsBuilder

	// ForConnectionId selects the connection whose assignments are listed.
	ForConnectionId(apid.ID) ListToolSetBindingsBuilder

	// ForToolSetNamespaceMatchers replaces the scope of visible parent ToolSets.
	// This scopes the ToolSet's namespace, which can be an ancestor of the
	// connection's. Replacing means a later call can never widen an earlier scope.
	ForToolSetNamespaceMatchers([]string) ListToolSetBindingsBuilder
}

// listToolSetBindingsFilters implements ListToolSetBindingsBuilder. It is also the
// cursor payload. A connection holds at most one live assignment per ToolSet, so
// the last ToolSet ID is an exact keyset position that writes between pages cannot
// shift, unlike the offsets other Tool listings use.
type listToolSetBindingsFilters struct {
	s                 *service `json:"-"`
	LimitVal          uint64   `json:"limit"`
	ConnectionIdVal   apid.ID  `json:"connectionId"`
	NamespaceMatchers []string `json:"namespaceMatchers,omitempty"`
	AfterToolSetId    apid.ID  `json:"afterToolSetId,omitempty"`
}

func (l *listToolSetBindingsFilters) Limit(limit int32) ListToolSetBindingsBuilder {
	l.LimitVal = toolListPageSize(limit, MaxToolSetBindingListLimit)
	return l
}

func (l *listToolSetBindingsFilters) ForConnectionId(id apid.ID) ListToolSetBindingsBuilder {
	l.ConnectionIdVal = id
	return l
}

func (l *listToolSetBindingsFilters) ForToolSetNamespaceMatchers(matchers []string) ListToolSetBindingsBuilder {
	l.NamespaceMatchers = matchers
	return l
}

// validate checks the required connection and every namespace matcher.
func (l *listToolSetBindingsFilters) validate() error {
	if l.ConnectionIdVal.IsNil() {
		return errors.New("toolset binding list requires a connection id")
	}

	if err := l.ConnectionIdVal.ValidatePrefix(apid.PrefixConnection); err != nil {
		return fmt.Errorf("invalid connection id: %w", err)
	}

	for _, matcher := range l.NamespaceMatchers {
		if err := namespace.ValidateMatcher(matcher); err != nil {
			return fmt.Errorf("invalid toolset namespace matcher: %w", err)
		}
	}

	return nil
}

// FetchPage reads one bounded page and advances past its last ToolSet. A lookahead
// row reports more results and produces a cursor for the next page.
func (l *listToolSetBindingsFilters) FetchPage(ctx context.Context) pagination.PageResult[ToolSetBindingListing] {
	if err := l.validate(); err != nil {
		return pagination.PageResult[ToolSetBindingListing]{Error: fmt.Errorf("%w: %w", ErrInvalidListQuery, err)}
	}

	if len(l.NamespaceMatchers) == 0 {
		return pagination.PageResult[ToolSetBindingListing]{Results: []ToolSetBindingListing{}}
	}

	var binding ToolSetBinding
	columns := append(util.PrependStringToAll("b.", binding.cols()), "c.namespace", "t.namespace", "t.name")
	q := l.s.sq.
		Select(columns...).
		From(ToolSetBindingsTable + " b").
		Join(ToolSetsTable + " t ON t.id = b.tool_set_id").
		Join(ConnectionsTable + " c ON c.id = b.connection_id").
		Where(sq.Eq{
			"b.connection_id": l.ConnectionIdVal,
			"b.deleted_at":    nil,
			"t.deleted_at":    nil,
			"c.deleted_at":    nil,
		}).
		Where(toolNamespaceScope("t.namespace", l.NamespaceMatchers)).
		OrderBy("b.tool_set_id ASC").
		Limit(l.LimitVal + 1)

	if !l.AfterToolSetId.IsNil() {
		q = q.Where(sq.Gt{"b.tool_set_id": l.AfterToolSetId})
	}

	rows, err := q.RunWith(l.s.db).QueryContext(ctx)
	if err != nil {
		return pagination.PageResult[ToolSetBindingListing]{Error: err}
	}
	defer rows.Close()

	items := []ToolSetBindingListing{}
	for rows.Next() {
		var item ToolSetBindingListing
		destinations := append(item.fields(), &item.ConnectionNamespace, &item.ToolSetNamespace, &item.ToolSetName)
		if err := rows.Scan(destinations...); err != nil {
			return pagination.PageResult[ToolSetBindingListing]{Error: err}
		}
		items = append(items, item)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return pagination.PageResult[ToolSetBindingListing]{Error: err}
	}

	hasMore := uint64(len(items)) > l.LimitVal
	if hasMore {
		items = items[:l.LimitVal]
	}
	if len(items) > 0 {
		l.AfterToolSetId = items[len(items)-1].ToolSetId
	}

	cursor := ""
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.cursorEncryptor, l)
		if err != nil {
			return pagination.PageResult[ToolSetBindingListing]{Error: err}
		}
	}

	return pagination.PageResult[ToolSetBindingListing]{Results: items, HasMore: hasMore, Cursor: cursor}
}

// Enumerate visits every remaining page until the callback stops or a page fails.
func (l *listToolSetBindingsFilters) Enumerate(
	ctx context.Context,
	callback pagination.EnumerateCallback[ToolSetBindingListing],
) error {
	var err error
	keepGoing := pagination.Continue
	hasMore := true

	for err == nil && hasMore && bool(keepGoing) {
		result := l.FetchPage(ctx)
		hasMore = result.HasMore

		if result.Error != nil {
			return result.Error
		}
		keepGoing, err = callback(result)
	}

	return err
}

// ListToolSetBindingsBuilder starts a listing of one connection's assignments.
func (s *service) ListToolSetBindingsBuilder() ListToolSetBindingsBuilder {
	return &listToolSetBindingsFilters{s: s, LimitVal: defaultToolListLimit}
}

// ListToolSetBindingsFromCursor continues a listing with its original filters.
func (s *service) ListToolSetBindingsFromCursor(ctx context.Context, cursor string) (ListToolSetBindingsExecutor, error) {
	parsed, err := pagination.ParseCursor[listToolSetBindingsFilters](ctx, s.cursorEncryptor, cursor)
	if err != nil {
		return nil, err
	}

	// Every continuation follows a returned binding, so a position must exist.
	if parsed.AfterToolSetId.IsNil() {
		return nil, errors.New("invalid toolset binding list cursor position")
	}

	parsed.s = s
	return parsed, nil
}

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

// MaxToolSetBindingListLimit bounds one page of ToolSet bindings.
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

// ListToolSetBindingsBuilder pages live assignments whose two parents are live.
// Exactly one parent must be chosen: ForConnectionId orders by ToolSet ID, and
// ForToolSetId orders by connection ID. That other parent's ID is the listing's
// key, and the listing is always key ordered, for FetchPage as well as Enumerate.
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

	// ForToolSetId selects the ToolSet whose assignments are listed.
	ForToolSetId(apid.ID) ListToolSetBindingsBuilder

	// ResumeAfter continues after a position taken from PageResult.Position; an
	// empty position starts at the beginning. The position is the other parent's
	// ID, so it is only meaningful with the same parent chosen.
	ResumeAfter(position string) ListToolSetBindingsBuilder

	// ForToolSetNamespaceMatchers replaces the scope of visible parent ToolSets.
	// This scopes the ToolSet's namespace, which can be an ancestor of the
	// connection's. Replacing means a later call can never widen an earlier scope.
	ForToolSetNamespaceMatchers([]string) ListToolSetBindingsBuilder
}

// toolSetBindingOrderByField is the order type binding listings give listPaging.
// It has no values, because these listings only page in key order.
type toolSetBindingOrderByField string

// listToolSetBindingsFilters implements ListToolSetBindingsBuilder. It is also the
// cursor payload. A parent pair holds at most one live assignment, so the other
// parent's ID is a unique, immutable key. Paging by it keeps the position exact
// while bindings are written or removed between pages, so a reconciler can remove
// bindings while enumerating them without skipping any.
type listToolSetBindingsFilters struct {
	s *service `json:"-"`

	// listPaging owns the page size and key position. It is always in key mode.
	listPaging[toolSetBindingOrderByField]

	ConnectionIdVal   apid.ID  `json:"connectionId,omitempty"`
	ToolSetIdVal      apid.ID  `json:"toolSetId,omitempty"`
	NamespaceMatchers []string `json:"namespaceMatchers,omitempty"`
}

func (l *listToolSetBindingsFilters) Limit(limit int32) ListToolSetBindingsBuilder {
	l.LimitVal = toolListPageSize(limit, MaxToolSetBindingListLimit)
	return l
}

// ResumeAfter continues after position in key order.
func (l *listToolSetBindingsFilters) ResumeAfter(position string) ListToolSetBindingsBuilder {
	l.listPaging.resumeAfter(position)
	return l
}

// keyColumn is the column the listing pages by: the parent that was not chosen.
func (l *listToolSetBindingsFilters) keyColumn() string {
	if !l.ConnectionIdVal.IsNil() {
		return "b.tool_set_id"
	}
	return "b.connection_id"
}

// key reads a row's position in the listing, matching keyColumn.
func (l *listToolSetBindingsFilters) key(item ToolSetBindingListing) string {
	if !l.ConnectionIdVal.IsNil() {
		return string(item.ToolSetId)
	}
	return string(item.ConnectionId)
}

func (l *listToolSetBindingsFilters) ForConnectionId(id apid.ID) ListToolSetBindingsBuilder {
	l.ConnectionIdVal = id
	return l
}

func (l *listToolSetBindingsFilters) ForToolSetId(id apid.ID) ListToolSetBindingsBuilder {
	l.ToolSetIdVal = id
	return l
}

func (l *listToolSetBindingsFilters) ForToolSetNamespaceMatchers(matchers []string) ListToolSetBindingsBuilder {
	l.NamespaceMatchers = matchers
	return l
}

// validate checks the single required parent and every namespace matcher.
func (l *listToolSetBindingsFilters) validate() error {
	switch {
	case l.ConnectionIdVal.IsNil() == l.ToolSetIdVal.IsNil():
		return errors.New("toolset binding list requires exactly one of a connection id or toolset id")
	case !l.ConnectionIdVal.IsNil():
		if err := l.ConnectionIdVal.ValidatePrefix(apid.PrefixConnection); err != nil {
			return fmt.Errorf("invalid connection id: %w", err)
		}
	default:
		if err := validateToolSetID(l.ToolSetIdVal); err != nil {
			return fmt.Errorf("invalid toolset id: %w", err)
		}
	}

	for _, matcher := range l.NamespaceMatchers {
		if err := namespace.ValidateMatcher(matcher); err != nil {
			return fmt.Errorf("invalid toolset namespace matcher: %w", err)
		}
	}

	return nil
}

// FetchPage reads one bounded page in key order and advances past its last row. A
// lookahead row reports more results and produces a cursor for the next page, and
// the page reports its last row's key as its Position.
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
			"b.deleted_at": nil,
			"t.deleted_at": nil,
			"c.deleted_at": nil,
		}).
		Where(toolNamespaceScope("t.namespace", l.NamespaceMatchers))

	if !l.ConnectionIdVal.IsNil() {
		q = q.Where(sq.Eq{"b.connection_id": l.ConnectionIdVal})
	} else {
		q = q.Where(sq.Eq{"b.tool_set_id": l.ToolSetIdVal})
	}
	q = l.listPaging.apply(q, l.keyColumn())

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

	items, hasMore, position := nextPage(&l.listPaging, items, l.key)

	cursor := ""
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.cursorEncryptor, l)
		if err != nil {
			return pagination.PageResult[ToolSetBindingListing]{Error: err}
		}
	}

	return pagination.PageResult[ToolSetBindingListing]{
		Results:  items,
		HasMore:  hasMore,
		Cursor:   cursor,
		Position: position,
	}
}

// Enumerate visits every remaining page until the callback stops or a page fails.
func (l *listToolSetBindingsFilters) Enumerate(
	ctx context.Context,
	callback pagination.EnumerateCallback[ToolSetBindingListing],
) error {
	// A no-op for this always key-ordered listing, kept so it starts like the others.
	l.listPaging.startEnumerate()

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

// ListToolSetBindingsBuilder starts a key-ordered listing of one parent's assignments.
func (s *service) ListToolSetBindingsBuilder() ListToolSetBindingsBuilder {
	l := &listToolSetBindingsFilters{s: s}
	l.LimitVal = defaultToolListLimit
	l.listPaging.resumeAfter("")
	return l
}

// ListToolSetBindingsFromCursor continues a listing with its original filters.
func (s *service) ListToolSetBindingsFromCursor(ctx context.Context, cursor string) (ListToolSetBindingsExecutor, error) {
	parsed, err := pagination.ParseCursor[listToolSetBindingsFilters](ctx, s.cursorEncryptor, cursor)
	if err != nil {
		return nil, err
	}

	// Every continuation follows a returned binding, so a key position must exist.
	if !parsed.Keyset || parsed.After == "" {
		return nil, errors.New("invalid toolset binding list cursor position")
	}

	parsed.s = s
	return parsed, nil
}

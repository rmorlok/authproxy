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
// ForToolSetId orders by connection ID.
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

	// ForToolSetNamespaceMatchers replaces the scope of visible parent ToolSets.
	// This scopes the ToolSet's namespace, which can be an ancestor of the
	// connection's. Replacing means a later call can never widen an earlier scope.
	ForToolSetNamespaceMatchers([]string) ListToolSetBindingsBuilder
}

// listToolSetBindingsFilters implements ListToolSetBindingsBuilder. It is also the
// cursor payload. A parent pair holds at most one live assignment, so the other
// parent's last ID is an exact keyset position that writes between pages cannot
// shift, unlike the offsets other Tool listings use. A reconciler can therefore
// remove bindings while enumerating them without skipping any.
type listToolSetBindingsFilters struct {
	s                 *service `json:"-"`
	LimitVal          uint64   `json:"limit"`
	ConnectionIdVal   apid.ID  `json:"connectionId,omitempty"`
	ToolSetIdVal      apid.ID  `json:"toolSetId,omitempty"`
	NamespaceMatchers []string `json:"namespaceMatchers,omitempty"`
	AfterToolSetId    apid.ID  `json:"afterToolSetId,omitempty"`
	AfterConnectionId apid.ID  `json:"afterConnectionId,omitempty"`
}

func (l *listToolSetBindingsFilters) Limit(limit int32) ListToolSetBindingsBuilder {
	l.LimitVal = toolListPageSize(limit, MaxToolSetBindingListLimit)
	return l
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

// FetchPage reads one bounded page and advances past its last row. A lookahead
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
			"b.deleted_at": nil,
			"t.deleted_at": nil,
			"c.deleted_at": nil,
		}).
		Where(toolNamespaceScope("t.namespace", l.NamespaceMatchers)).
		Limit(l.LimitVal + 1)

	if !l.ConnectionIdVal.IsNil() {
		q = q.Where(sq.Eq{"b.connection_id": l.ConnectionIdVal}).OrderBy("b.tool_set_id ASC")
		if !l.AfterToolSetId.IsNil() {
			q = q.Where(sq.Gt{"b.tool_set_id": l.AfterToolSetId})
		}
	} else {
		q = q.Where(sq.Eq{"b.tool_set_id": l.ToolSetIdVal}).OrderBy("b.connection_id ASC")
		if !l.AfterConnectionId.IsNil() {
			q = q.Where(sq.Gt{"b.connection_id": l.AfterConnectionId})
		}
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
		last := items[len(items)-1]
		if !l.ConnectionIdVal.IsNil() {
			l.AfterToolSetId = last.ToolSetId
		} else {
			l.AfterConnectionId = last.ConnectionId
		}
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

// ListToolSetBindingsBuilder starts a listing of one parent's assignments.
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
	if parsed.AfterToolSetId.IsNil() && parsed.AfterConnectionId.IsNil() {
		return nil, errors.New("invalid toolset binding list cursor position")
	}

	parsed.s = s
	return parsed, nil
}

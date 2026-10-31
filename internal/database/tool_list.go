package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// MaxToolListLimit bounds one management list page. Management pages read full
// identity rows, so the cap matches the internal candidate scan.
const MaxToolListLimit = MaxToolCandidateLimit

// defaultToolListLimit is the Tool and ToolSet page size when a caller sets none.
const defaultToolListLimit = 50

// ToolOrderByField names a column management Tool listings can sort by.
type ToolOrderByField string

const (
	ToolOrderById        ToolOrderByField = "id"
	ToolOrderByNamespace ToolOrderByField = "namespace"
	ToolOrderByName      ToolOrderByField = "name"
	ToolOrderByCreatedAt ToolOrderByField = "created_at"
	ToolOrderByUpdatedAt ToolOrderByField = "updated_at"
)

// IsValidToolOrderByField reports whether field is a supported sort column.
// Listings interpolate the field into SQL, so only these values may reach it.
func IsValidToolOrderByField[T string | ToolOrderByField](field T) bool {
	switch ToolOrderByField(field) {
	case ToolOrderById,
		ToolOrderByNamespace,
		ToolOrderByName,
		ToolOrderByCreatedAt,
		ToolOrderByUpdatedAt:
		return true
	default:
		return false
	}
}

// toolNamespaceScope matches a namespace column against any supplied matcher.
// Literal prefix comparison avoids LIKE's wildcard and case behavior, matching the
// candidate scan so namespace scope agrees on SQLite and PostgreSQL. The column
// must be a trusted constant because it is interpolated into SQL.
func toolNamespaceScope(column string, matchers []string) sq.Or {
	scopes := make(sq.Or, 0, len(matchers))
	for _, matcher := range matchers {
		base := strings.TrimSuffix(matcher, namespace.WildcardSuffix)
		condition := sq.Or{sq.Eq{column: base}}
		if base != matcher {
			prefix := base + namespace.PathSeparator
			condition = append(condition, sq.Expr("substr("+column+", 1, ?) = ?", len(prefix), prefix))
		}
		scopes = append(scopes, condition)
	}
	return scopes
}

// toolListPageSize resolves a requested page size. Non-positive values select
// the default and larger values are capped, so a caller cannot read unbounded
// pages. The result is always between one and limit.
func toolListPageSize(requested int32, limit uint64) uint64 {
	if requested <= 0 {
		return defaultToolListLimit
	}
	return min(uint64(requested), limit)
}

// ListToolsExecutor runs a management Tool listing one page at a time or across
// every remaining page. Each FetchPage advances the listing, and Enumerate stops
// at the first error.
type ListToolsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Tool]
	Enumerate(context.Context, pagination.EnumerateCallback[Tool]) error
}

// ListToolsBuilder narrows a management listing of live Tool identities. Unlike
// ToolCandidateQuery it does not require a live connection, active definition
// projections, or an installed binding, so administrators can inspect and clean
// up every Tool that GetTool would return. All supplied filters intersect.
//
// Namespace scope is required: with no matchers the listing is empty, so a
// missing permission scope never lists every Tool. Invalid filters are reported
// as ErrInvalidListQuery when the listing runs. Results include standalone and
// generated Tools in live namespaces, whether or not their connection is still
// live; callers must still apply per-resource permission checks.
type ListToolsBuilder interface {
	ListToolsExecutor

	// Limit sets the page size. Non-positive values select the default and values
	// above MaxToolListLimit are capped.
	Limit(int32) ListToolsBuilder

	// ForNamespaceMatcher replaces the namespace scope with one matcher.
	ForNamespaceMatcher(string) ListToolsBuilder

	// ForNamespaceMatchers replaces the namespace scope. Replacing, rather than
	// adding to it, means a later call can never widen an earlier scope.
	ForNamespaceMatchers([]string) ListToolsBuilder

	// ForName selects one exact resource name.
	ForName(common.ResourceName) ListToolsBuilder

	// ForConnectionId selects Tools currently bound to one connection.
	ForConnectionId(apid.ID) ListToolsBuilder

	// ForLabelSelector matches Tool labels, including inherited connection identity.
	ForLabelSelector(string) ListToolsBuilder

	// OrderBy selects the sort column and direction. Results always fall back to
	// ID so pages stay deterministic when sort values tie; the default sorts by ID.
	OrderBy(ToolOrderByField, pagination.OrderBy) ListToolsBuilder
}

// listToolsFilters implements ListToolsBuilder. It is also the cursor payload, so
// continuation keeps the original filters and row offset. Like other offset-paged
// resources, rows written between pages can shift it.
type listToolsFilters struct {
	s                 *service            `json:"-"`
	LimitVal          uint64              `json:"limit"`
	Offset            uint64              `json:"offset"`
	NamespaceMatchers []string            `json:"namespaceMatchers,omitempty"`
	NameVal           common.ResourceName `json:"name,omitempty"`
	ConnectionIdVal   apid.ID             `json:"connectionId,omitempty"`
	LabelSelectorVal  string              `json:"labelSelector,omitempty"`
	OrderByFieldVal   ToolOrderByField    `json:"orderByField,omitempty"`
	OrderByVal        pagination.OrderBy  `json:"orderBy,omitempty"`
}

func (l *listToolsFilters) Limit(limit int32) ListToolsBuilder {
	l.LimitVal = toolListPageSize(limit, MaxToolListLimit)
	return l
}

func (l *listToolsFilters) ForNamespaceMatcher(matcher string) ListToolsBuilder {
	return l.ForNamespaceMatchers([]string{matcher})
}

func (l *listToolsFilters) ForNamespaceMatchers(matchers []string) ListToolsBuilder {
	l.NamespaceMatchers = matchers
	return l
}

func (l *listToolsFilters) ForName(name common.ResourceName) ListToolsBuilder {
	l.NameVal = name
	return l
}

func (l *listToolsFilters) ForConnectionId(id apid.ID) ListToolsBuilder {
	l.ConnectionIdVal = id
	return l
}

func (l *listToolsFilters) ForLabelSelector(selector string) ListToolsBuilder {
	l.LabelSelectorVal = selector
	return l
}

func (l *listToolsFilters) OrderBy(field ToolOrderByField, by pagination.OrderBy) ListToolsBuilder {
	l.OrderByFieldVal = field
	l.OrderByVal = by
	return l
}

// validate checks every supplied filter and parses the label selector once.
func (l *listToolsFilters) validate() (LabelSelector, error) {
	for _, matcher := range l.NamespaceMatchers {
		if err := namespace.ValidateMatcher(matcher); err != nil {
			return nil, fmt.Errorf("invalid tool namespace matcher: %w", err)
		}
	}

	if l.NameVal != "" {
		if err := l.NameVal.Validate(); err != nil {
			return nil, fmt.Errorf("invalid tool name: %w", err)
		}
	}

	if !l.ConnectionIdVal.IsNil() {
		if err := validateToolConnectionID(l.ConnectionIdVal); err != nil {
			return nil, err
		}
	}

	if l.OrderByFieldVal != "" && !IsValidToolOrderByField(l.OrderByFieldVal) {
		return nil, fmt.Errorf("invalid tool order by field %q", l.OrderByFieldVal)
	}

	return ParseLabelSelector(l.LabelSelectorVal)
}

// FetchPage reads one bounded page and advances the offset. A lookahead row
// reports more results and produces a cursor for the next page.
func (l *listToolsFilters) FetchPage(ctx context.Context) pagination.PageResult[Tool] {
	selector, err := l.validate()
	if err != nil {
		return pagination.PageResult[Tool]{Error: fmt.Errorf("%w: %w", ErrInvalidListQuery, err)}
	}

	if len(l.NamespaceMatchers) == 0 {
		return pagination.PageResult[Tool]{Results: []Tool{}}
	}

	var row Tool
	q := l.s.sq.
		Select(util.PrependStringToAll("t.", row.cols())...).
		From(ToolsTable + " t").
		Join(NamespacesTable + " n ON n.path = t.namespace").
		Where(sq.Eq{
			"t.deleted_at": nil,
			"n.deleted_at": nil,
		}).
		Limit(l.LimitVal + 1).
		Offset(l.Offset)

	// The field is validated above. ID always breaks ties so offsets stay stable.
	if l.OrderByFieldVal != "" {
		q = q.OrderBy(fmt.Sprintf("t.%s %s", l.OrderByFieldVal, l.OrderByVal.String()))
	}
	q = q.OrderBy("t.id ASC")

	q = q.Where(toolNamespaceScope("t.namespace", l.NamespaceMatchers))

	if l.NameVal != "" {
		q = q.Where(sq.Eq{"t.name": l.NameVal})
	}
	if !l.ConnectionIdVal.IsNil() {
		q = q.Where(sq.Eq{"t.connection_id": l.ConnectionIdVal})
	}
	q = selector.ApplyToSqlBuilderWithProvider(q, "t.labels", l.s.cfg.GetProvider())

	rows, err := q.RunWith(l.s.db).QueryContext(ctx)
	if err != nil {
		return pagination.PageResult[Tool]{Error: err}
	}
	defer rows.Close()

	items := []Tool{}
	for rows.Next() {
		var item Tool
		if err := rows.Scan(item.fields()...); err != nil {
			return pagination.PageResult[Tool]{Error: err}
		}
		items = append(items, item)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return pagination.PageResult[Tool]{Error: err}
	}

	hasMore := uint64(len(items)) > l.LimitVal
	if hasMore {
		items = items[:l.LimitVal]
	}
	l.Offset += uint64(len(items))

	cursor := ""
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.cursorEncryptor, l)
		if err != nil {
			return pagination.PageResult[Tool]{Error: err}
		}
	}

	return pagination.PageResult[Tool]{Results: items, HasMore: hasMore, Cursor: cursor}
}

// Enumerate calls callback with each remaining page until the listing ends, the
// callback stops, or an error occurs.
func (l *listToolsFilters) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[Tool]) error {
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

// ListToolsBuilder starts an ordered management listing with default paging.
func (s *service) ListToolsBuilder() ListToolsBuilder {
	return &listToolsFilters{s: s, LimitVal: defaultToolListLimit}
}

// ListToolsFromCursor resumes a management listing with its saved filters and
// limit. Invalid cursors, including candidate-scan cursors, return an error.
func (s *service) ListToolsFromCursor(ctx context.Context, cursor string) (ListToolsExecutor, error) {
	parsed, err := pagination.ParseCursor[listToolsFilters](ctx, s.cursorEncryptor, cursor)
	if err != nil {
		return nil, err
	}

	// Every continuation follows a returned Tool, so the offset is positive.
	if parsed.Offset == 0 {
		return nil, errors.New("invalid tool list cursor position")
	}

	parsed.s = s
	return parsed, nil
}

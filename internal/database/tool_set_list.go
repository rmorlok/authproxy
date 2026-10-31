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

// MaxToolSetListLimit bounds one management page of logical ToolSets or of one
// ToolSet's generations.
const MaxToolSetListLimit = 100

// ToolSetOrderByField names a column logical ToolSet listings can sort by.
type ToolSetOrderByField string

const (
	ToolSetOrderById        ToolSetOrderByField = "id"
	ToolSetOrderByNamespace ToolSetOrderByField = "namespace"
	ToolSetOrderByName      ToolSetOrderByField = "name"
	ToolSetOrderByCreatedAt ToolSetOrderByField = "created_at"
	ToolSetOrderByUpdatedAt ToolSetOrderByField = "updated_at"
)

// IsValidToolSetOrderByField reports whether field is a supported sort column.
// Listings interpolate the field into SQL, so only these values may reach it.
func IsValidToolSetOrderByField[T string | ToolSetOrderByField](field T) bool {
	switch ToolSetOrderByField(field) {
	case ToolSetOrderById,
		ToolSetOrderByNamespace,
		ToolSetOrderByName,
		ToolSetOrderByCreatedAt,
		ToolSetOrderByUpdatedAt:
		return true
	default:
		return false
	}
}

// ToolSetGenerationOrderByField names a column generation listings can sort by.
type ToolSetGenerationOrderByField string

const (
	ToolSetGenerationOrderByGeneration ToolSetGenerationOrderByField = "generation"
	ToolSetGenerationOrderByState      ToolSetGenerationOrderByField = "state"
	ToolSetGenerationOrderByCreatedAt  ToolSetGenerationOrderByField = "created_at"
	ToolSetGenerationOrderByUpdatedAt  ToolSetGenerationOrderByField = "updated_at"
)

// IsValidToolSetGenerationOrderByField reports whether field is a supported
// sort column. Listings interpolate the field into SQL, so only these may reach it.
func IsValidToolSetGenerationOrderByField[T string | ToolSetGenerationOrderByField](field T) bool {
	switch ToolSetGenerationOrderByField(field) {
	case ToolSetGenerationOrderByGeneration,
		ToolSetGenerationOrderByState,
		ToolSetGenerationOrderByCreatedAt,
		ToolSetGenerationOrderByUpdatedAt:
		return true
	default:
		return false
	}
}

// ListToolSetsExecutor runs a logical ToolSet listing one page at a time or across
// every remaining page. Each FetchPage advances the listing, and Enumerate stops
// at the first error.
type ListToolSetsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[ToolSet]
	Enumerate(context.Context, pagination.EnumerateCallback[ToolSet]) error
}

// ListToolSetsBuilder narrows a management listing of live logical ToolSets in
// live namespaces. All supplied filters intersect; generations are not consulted.
//
// Namespace scope is required: with no matchers the listing is empty, so a
// missing permission scope never lists every ToolSet. Invalid filters are
// reported as ErrInvalidListQuery when the listing runs. Callers must still apply
// per-resource permission checks.
type ListToolSetsBuilder interface {
	ListToolSetsExecutor

	// Limit sets the page size. Non-positive values select the default and values
	// above MaxToolSetListLimit are capped.
	Limit(int32) ListToolSetsBuilder

	// ForNamespaceMatcher replaces the namespace scope with one matcher.
	ForNamespaceMatcher(string) ListToolSetsBuilder

	// ForNamespaceMatchers replaces the namespace scope. Replacing, rather than
	// adding to it, means a later call can never widen an earlier scope.
	ForNamespaceMatchers([]string) ListToolSetsBuilder

	// ForName selects one exact resource name.
	ForName(common.ResourceName) ListToolSetsBuilder

	// ForLabelSelector matches the logical ToolSet's labels, including inherited
	// namespace labels and self-implicit identity labels.
	ForLabelSelector(string) ListToolSetsBuilder

	// OrderBy selects the sort column and direction. Results always fall back to
	// ID so pages stay deterministic when sort values tie; the default sorts by ID.
	OrderBy(ToolSetOrderByField, pagination.OrderBy) ListToolSetsBuilder
}

// listToolSetsFilters implements ListToolSetsBuilder. It is also the cursor
// payload, so continuation keeps the original filters and row offset. Like other
// offset-paged resources, rows written between pages can shift it.
type listToolSetsFilters struct {
	s                 *service            `json:"-"`
	LimitVal          uint64              `json:"limit"`
	Offset            uint64              `json:"offset"`
	NamespaceMatchers []string            `json:"namespaceMatchers,omitempty"`
	NameVal           common.ResourceName `json:"name,omitempty"`
	LabelSelectorVal  string              `json:"labelSelector,omitempty"`
	OrderByFieldVal   ToolSetOrderByField `json:"orderByField,omitempty"`
	OrderByVal        pagination.OrderBy  `json:"orderBy,omitempty"`
}

func (l *listToolSetsFilters) Limit(limit int32) ListToolSetsBuilder {
	l.LimitVal = toolListPageSize(limit, MaxToolSetListLimit)
	return l
}

func (l *listToolSetsFilters) ForNamespaceMatcher(matcher string) ListToolSetsBuilder {
	return l.ForNamespaceMatchers([]string{matcher})
}

func (l *listToolSetsFilters) ForNamespaceMatchers(matchers []string) ListToolSetsBuilder {
	l.NamespaceMatchers = matchers
	return l
}

func (l *listToolSetsFilters) ForName(name common.ResourceName) ListToolSetsBuilder {
	l.NameVal = name
	return l
}

func (l *listToolSetsFilters) ForLabelSelector(selector string) ListToolSetsBuilder {
	l.LabelSelectorVal = selector
	return l
}

func (l *listToolSetsFilters) OrderBy(field ToolSetOrderByField, by pagination.OrderBy) ListToolSetsBuilder {
	l.OrderByFieldVal = field
	l.OrderByVal = by
	return l
}

// validate checks every supplied filter and parses the label selector once.
func (l *listToolSetsFilters) validate() (LabelSelector, error) {
	for _, matcher := range l.NamespaceMatchers {
		if err := namespace.ValidateMatcher(matcher); err != nil {
			return nil, fmt.Errorf("invalid toolset namespace matcher: %w", err)
		}
	}

	if l.NameVal != "" {
		if err := l.NameVal.Validate(); err != nil {
			return nil, fmt.Errorf("invalid toolset name: %w", err)
		}
	}

	if l.OrderByFieldVal != "" && !IsValidToolSetOrderByField(l.OrderByFieldVal) {
		return nil, fmt.Errorf("invalid toolset order by field %q", l.OrderByFieldVal)
	}

	return ParseLabelSelector(l.LabelSelectorVal)
}

// FetchPage reads one bounded page and advances the offset. A lookahead row
// reports more results and produces a cursor for the next page.
func (l *listToolSetsFilters) FetchPage(ctx context.Context) pagination.PageResult[ToolSet] {
	selector, err := l.validate()
	if err != nil {
		return pagination.PageResult[ToolSet]{Error: fmt.Errorf("%w: %w", ErrInvalidListQuery, err)}
	}

	if len(l.NamespaceMatchers) == 0 {
		return pagination.PageResult[ToolSet]{Results: []ToolSet{}}
	}

	var row ToolSet
	q := l.s.sq.
		Select(util.PrependStringToAll("t.", row.cols())...).
		From(ToolSetsTable + " t").
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
	q = selector.ApplyToSqlBuilderWithProvider(q, "t.labels", l.s.cfg.GetProvider())

	rows, err := q.RunWith(l.s.db).QueryContext(ctx)
	if err != nil {
		return pagination.PageResult[ToolSet]{Error: err}
	}
	defer rows.Close()

	items := []ToolSet{}
	for rows.Next() {
		var item ToolSet
		if err := rows.Scan(item.fields()...); err != nil {
			return pagination.PageResult[ToolSet]{Error: err}
		}
		items = append(items, item)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return pagination.PageResult[ToolSet]{Error: err}
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
			return pagination.PageResult[ToolSet]{Error: err}
		}
	}

	return pagination.PageResult[ToolSet]{Results: items, HasMore: hasMore, Cursor: cursor}
}

// Enumerate calls callback with each remaining page until the listing ends, the
// callback stops, or an error occurs.
func (l *listToolSetsFilters) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[ToolSet]) error {
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

// ListToolSetsBuilder starts an ordered logical listing with default paging.
func (s *service) ListToolSetsBuilder() ListToolSetsBuilder {
	return &listToolSetsFilters{s: s, LimitVal: defaultToolListLimit}
}

// ListToolSetsFromCursor resumes a listing with its saved filters and limit.
func (s *service) ListToolSetsFromCursor(ctx context.Context, cursor string) (ListToolSetsExecutor, error) {
	parsed, err := pagination.ParseCursor[listToolSetsFilters](ctx, s.cursorEncryptor, cursor)
	if err != nil {
		return nil, err
	}

	// Every continuation follows a returned ToolSet, so the offset is positive.
	if parsed.Offset == 0 {
		return nil, errors.New("invalid toolset list cursor position")
	}

	parsed.s = s
	return parsed, nil
}

// ListToolSetGenerationsExecutor runs a generation listing one page at a time or
// across every remaining page. Each FetchPage advances the listing, and Enumerate
// stops at the first error.
type ListToolSetGenerationsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[ToolSetGeneration]
	Enumerate(context.Context, pagination.EnumerateCallback[ToolSetGeneration]) error
}

// ListToolSetGenerationsBuilder selects the live generations of one live ToolSet,
// newest first unless OrderBy is set. ForToolSetId is required. A missing or
// deleted parent returns ErrNotFound rather than an empty page, so callers can
// distinguish an unknown ToolSet from one with no live generations.
type ListToolSetGenerationsBuilder interface {
	ListToolSetGenerationsExecutor

	// Limit sets the page size. Non-positive values select the default and values
	// above MaxToolSetListLimit are capped.
	Limit(int32) ListToolSetGenerationsBuilder

	// ForToolSetId selects the parent ToolSet.
	ForToolSetId(apid.ID) ListToolSetGenerationsBuilder

	// OrderBy selects the sort column and direction. Generation number always
	// breaks ties, newest first, so pages stay deterministic.
	OrderBy(ToolSetGenerationOrderByField, pagination.OrderBy) ListToolSetGenerationsBuilder
}

// listToolSetGenerationsFilters implements ListToolSetGenerationsBuilder. It is
// also the cursor payload, so continuation cannot change the ToolSet or ordering.
type listToolSetGenerationsFilters struct {
	s               *service                      `json:"-"`
	LimitVal        uint64                        `json:"limit"`
	Offset          uint64                        `json:"offset"`
	ToolSetIdVal    apid.ID                       `json:"toolSetId"`
	OrderByFieldVal ToolSetGenerationOrderByField `json:"orderByField,omitempty"`
	OrderByVal      pagination.OrderBy            `json:"orderBy,omitempty"`
}

func (l *listToolSetGenerationsFilters) Limit(limit int32) ListToolSetGenerationsBuilder {
	l.LimitVal = toolListPageSize(limit, MaxToolSetListLimit)
	return l
}

func (l *listToolSetGenerationsFilters) ForToolSetId(id apid.ID) ListToolSetGenerationsBuilder {
	l.ToolSetIdVal = id
	return l
}

func (l *listToolSetGenerationsFilters) OrderBy(
	field ToolSetGenerationOrderByField,
	by pagination.OrderBy,
) ListToolSetGenerationsBuilder {
	l.OrderByFieldVal = field
	l.OrderByVal = by
	return l
}

// FetchPage reads one bounded page and advances the offset. It checks parent
// liveness first so deletion hides history.
func (l *listToolSetGenerationsFilters) FetchPage(ctx context.Context) pagination.PageResult[ToolSetGeneration] {
	if l.ToolSetIdVal.IsNil() {
		return pagination.PageResult[ToolSetGeneration]{
			Error: fmt.Errorf("%w: toolset generation list requires a toolset id", ErrInvalidListQuery),
		}
	}
	if l.OrderByFieldVal != "" && !IsValidToolSetGenerationOrderByField(l.OrderByFieldVal) {
		return pagination.PageResult[ToolSetGeneration]{
			Error: fmt.Errorf("%w: invalid toolset generation order by field %q", ErrInvalidListQuery, l.OrderByFieldVal),
		}
	}
	if _, err := l.s.GetToolSet(ctx, l.ToolSetIdVal); err != nil {
		return pagination.PageResult[ToolSetGeneration]{Error: err}
	}

	var row ToolSetGeneration
	q := l.s.sq.
		Select(row.cols()...).
		From(ToolSetGenerationsTable).
		Where(sq.Eq{
			"tool_set_id": l.ToolSetIdVal,
			"deleted_at":  nil,
		}).
		Limit(l.LimitVal + 1).
		Offset(l.Offset)

	// The field is validated above. Generation breaks ties so offsets stay stable.
	if l.OrderByFieldVal != "" {
		q = q.OrderBy(fmt.Sprintf("%s %s", l.OrderByFieldVal, l.OrderByVal.String()))
	}
	q = q.OrderBy("generation DESC")

	rows, err := q.RunWith(l.s.db).QueryContext(ctx)
	if err != nil {
		return pagination.PageResult[ToolSetGeneration]{Error: err}
	}
	defer rows.Close()

	items := []ToolSetGeneration{}
	for rows.Next() {
		var item ToolSetGeneration
		if err := rows.Scan(item.fields()...); err != nil {
			return pagination.PageResult[ToolSetGeneration]{Error: err}
		}
		items = append(items, item)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return pagination.PageResult[ToolSetGeneration]{Error: err}
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
			return pagination.PageResult[ToolSetGeneration]{Error: err}
		}
	}

	return pagination.PageResult[ToolSetGeneration]{Results: items, HasMore: hasMore, Cursor: cursor}
}

// Enumerate calls callback with each remaining page until the listing ends, the
// callback stops, or an error occurs.
func (l *listToolSetGenerationsFilters) Enumerate(
	ctx context.Context,
	callback pagination.EnumerateCallback[ToolSetGeneration],
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

// ListToolSetGenerationsBuilder starts a generation listing with default paging.
func (s *service) ListToolSetGenerationsBuilder() ListToolSetGenerationsBuilder {
	return &listToolSetGenerationsFilters{s: s, LimitVal: defaultToolListLimit}
}

// ListToolSetGenerationsFromCursor resumes a generation listing.
func (s *service) ListToolSetGenerationsFromCursor(
	ctx context.Context,
	cursor string,
) (ListToolSetGenerationsExecutor, error) {
	parsed, err := pagination.ParseCursor[listToolSetGenerationsFilters](ctx, s.cursorEncryptor, cursor)
	if err != nil {
		return nil, err
	}

	// Every continuation follows a returned generation, so the offset is positive.
	if parsed.Offset == 0 {
		return nil, errors.New("invalid toolset generation cursor position")
	}

	parsed.s = s
	return parsed, nil
}

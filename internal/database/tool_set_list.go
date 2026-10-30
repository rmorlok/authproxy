package database

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

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

// ToolSetListQuery narrows a management listing of live logical ToolSets. All
// supplied filters intersect; generations are not consulted.
type ToolSetListQuery struct {
	// NamespaceMatchers is the caller's effective namespace scope. Nil or empty
	// matches nothing, so a missing permission scope never lists every ToolSet.
	NamespaceMatchers []string `json:"namespaceMatchers"`

	// Name optionally selects one exact resource name.
	Name common.ResourceName `json:"name,omitempty"`

	// LabelSelector matches the logical ToolSet's labels, including inherited
	// namespace labels and self-implicit identity labels.
	LabelSelector string `json:"labelSelector,omitempty"`

	// Limit must be between one and MaxToolSetListLimit; there is no default.
	Limit int `json:"limit"`
}

// toolSetListCursor preserves the original query and exclusive ID position, so
// continuation cannot change filters.
type toolSetListCursor struct {
	ToolSetListQuery
	AfterID apid.ID `json:"afterId"`
}

// ToolSetListPage is one ID-ordered page of live logical ToolSets.
type ToolSetListPage struct {
	Items []ToolSet

	// Cursor resumes through ListToolSetsFromCursor and is empty at the end.
	Cursor string
}

// validate checks every supplied filter and parses the label selector once.
func (q ToolSetListQuery) validate() (LabelSelector, error) {
	if q.Limit < 1 || q.Limit > MaxToolSetListLimit {
		return nil, fmt.Errorf("toolset list limit must be between 1 and %d", MaxToolSetListLimit)
	}

	for _, matcher := range q.NamespaceMatchers {
		if err := namespace.ValidateMatcher(matcher); err != nil {
			return nil, fmt.Errorf("invalid toolset namespace matcher: %w", err)
		}
	}

	if q.Name != "" {
		if err := q.Name.Validate(); err != nil {
			return nil, fmt.Errorf("invalid toolset name: %w", err)
		}
	}

	return ParseLabelSelector(q.LabelSelector)
}

// ListToolSets starts an ID-ordered listing of live ToolSets in live
// namespaces. Callers must still apply per-resource permission checks.
func (s *service) ListToolSets(
	ctx context.Context,
	query ToolSetListQuery,
) (ToolSetListPage, error) {
	return s.listToolSets(ctx, query, apid.Nil)
}

// ListToolSetsFromCursor resumes a listing with its saved filters and limit.
func (s *service) ListToolSetsFromCursor(
	ctx context.Context,
	cursor string,
) (ToolSetListPage, error) {
	parsed, err := pagination.ParseCursor[toolSetListCursor](ctx, s.cursorEncryptor, cursor)
	if err != nil {
		return ToolSetListPage{}, err
	}

	// Every continuation follows a returned ToolSet, so the position is required.
	if err := validateToolSetID(parsed.AfterID); err != nil {
		return ToolSetListPage{}, err
	}

	return s.listToolSets(ctx, parsed.ToolSetListQuery, parsed.AfterID)
}

// listToolSets reads one bounded page after the supplied position. A lookahead
// row produces a cursor carrying the original query and the last returned ID.
func (s *service) listToolSets(
	ctx context.Context,
	params ToolSetListQuery,
	afterID apid.ID,
) (ToolSetListPage, error) {
	selector, err := params.validate()
	if err != nil {
		return ToolSetListPage{}, err
	}

	page := ToolSetListPage{Items: []ToolSet{}}
	if len(params.NamespaceMatchers) == 0 {
		return page, nil
	}

	var row ToolSet
	q := s.sq.
		Select(util.PrependStringToAll("t.", row.cols())...).
		From(ToolSetsTable + " t").
		Join(NamespacesTable + " n ON n.path = t.namespace").
		Where(sq.Eq{
			"t.deleted_at": nil,
			"n.deleted_at": nil,
		}).
		OrderBy("t.id ASC").
		Limit(uint64(params.Limit + 1))

	// Literal prefix comparison avoids LIKE's wildcard and case behavior, matching
	// the Tool listing so namespace scope agrees on SQLite and PostgreSQL.
	var scopes sq.Or
	for _, matcher := range params.NamespaceMatchers {
		base := strings.TrimSuffix(matcher, namespace.WildcardSuffix)
		condition := sq.Or{sq.Eq{"t.namespace": base}}
		if base != matcher {
			prefix := base + namespace.PathSeparator
			condition = append(condition, sq.Expr("substr(t.namespace, 1, ?) = ?", len(prefix), prefix))
		}
		scopes = append(scopes, condition)
	}
	q = q.Where(scopes)

	if params.Name != "" {
		q = q.Where(sq.Eq{"t.name": params.Name})
	}
	if !afterID.IsNil() {
		q = q.Where(sq.Gt{"t.id": afterID})
	}
	q = selector.ApplyToSqlBuilderWithProvider(q, "t.labels", s.cfg.GetProvider())

	rows, err := q.RunWith(s.db).QueryContext(ctx)
	if err != nil {
		return ToolSetListPage{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var item ToolSet
		if err := rows.Scan(item.fields()...); err != nil {
			return ToolSetListPage{}, err
		}
		page.Items = append(page.Items, item)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return ToolSetListPage{}, err
	}

	if len(page.Items) > params.Limit {
		page.Items = page.Items[:params.Limit]
		page.Cursor, err = pagination.MakeCursor(ctx, s.cursorEncryptor, toolSetListCursor{
			ToolSetListQuery: params,
			AfterID:          page.Items[len(page.Items)-1].Id,
		})
		if err != nil {
			return ToolSetListPage{}, err
		}
	}

	return page, nil
}

// ToolSetGenerationListQuery selects the live generations of one live ToolSet,
// newest first.
type ToolSetGenerationListQuery struct {
	ToolSetId apid.ID `json:"toolSetId"`

	// Limit must be between one and MaxToolSetListLimit; there is no default.
	Limit int `json:"limit"`
}

// toolSetGenerationListCursor resumes below the last returned generation.
type toolSetGenerationListCursor struct {
	ToolSetGenerationListQuery
	BeforeGeneration uint64 `json:"beforeGeneration"`
}

// ToolSetGenerationListPage is one page of generations in descending order.
type ToolSetGenerationListPage struct {
	Items []ToolSetGeneration

	// Cursor resumes through ListToolSetGenerationsFromCursor and is empty at the end.
	Cursor string
}

// ListToolSetGenerations returns the newest live generations of a live ToolSet.
// A missing or deleted parent returns ErrNotFound rather than an empty page, so
// callers can distinguish an unknown ToolSet from one with no live generations.
func (s *service) ListToolSetGenerations(
	ctx context.Context,
	query ToolSetGenerationListQuery,
) (ToolSetGenerationListPage, error) {
	return s.listToolSetGenerations(ctx, query, math.MaxInt64)
}

// ListToolSetGenerationsFromCursor resumes a generation listing.
func (s *service) ListToolSetGenerationsFromCursor(
	ctx context.Context,
	cursor string,
) (ToolSetGenerationListPage, error) {
	parsed, err := pagination.ParseCursor[toolSetGenerationListCursor](ctx, s.cursorEncryptor, cursor)
	if err != nil {
		return ToolSetGenerationListPage{}, err
	}

	if parsed.BeforeGeneration == 0 || parsed.BeforeGeneration > math.MaxInt64 {
		return ToolSetGenerationListPage{}, errors.New("invalid toolset generation cursor position")
	}

	return s.listToolSetGenerations(ctx, parsed.ToolSetGenerationListQuery, parsed.BeforeGeneration)
}

// listToolSetGenerations reads one page of generations numbered strictly below
// beforeGeneration. It checks parent liveness first so deletion hides history.
func (s *service) listToolSetGenerations(
	ctx context.Context,
	params ToolSetGenerationListQuery,
	beforeGeneration uint64,
) (ToolSetGenerationListPage, error) {
	if params.Limit < 1 || params.Limit > MaxToolSetListLimit {
		return ToolSetGenerationListPage{}, fmt.Errorf("toolset generation list limit must be between 1 and %d", MaxToolSetListLimit)
	}
	if _, err := s.GetToolSet(ctx, params.ToolSetId); err != nil {
		return ToolSetGenerationListPage{}, err
	}

	var row ToolSetGeneration
	rows, err := s.sq.
		Select(row.cols()...).
		From(ToolSetGenerationsTable).
		Where(sq.Eq{
			"tool_set_id": params.ToolSetId,
			"deleted_at":  nil,
		}).
		Where(sq.Lt{"generation": beforeGeneration}).
		OrderBy("generation DESC").
		Limit(uint64(params.Limit + 1)).
		RunWith(s.db).
		QueryContext(ctx)
	if err != nil {
		return ToolSetGenerationListPage{}, err
	}
	defer rows.Close()

	page := ToolSetGenerationListPage{Items: []ToolSetGeneration{}}
	for rows.Next() {
		var item ToolSetGeneration
		if err := rows.Scan(item.fields()...); err != nil {
			return ToolSetGenerationListPage{}, err
		}
		page.Items = append(page.Items, item)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return ToolSetGenerationListPage{}, err
	}

	if len(page.Items) > params.Limit {
		page.Items = page.Items[:params.Limit]
		page.Cursor, err = pagination.MakeCursor(ctx, s.cursorEncryptor, toolSetGenerationListCursor{
			ToolSetGenerationListQuery: params,
			BeforeGeneration:           page.Items[len(page.Items)-1].Generation,
		})
		if err != nil {
			return ToolSetGenerationListPage{}, err
		}
	}

	return page, nil
}

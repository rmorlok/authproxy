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

// ToolListQuery narrows a management listing of live Tool identities. Unlike
// ToolCandidateQuery it does not require a live connection, active definition
// projections, or an installed binding, so administrators can inspect and clean
// up every Tool that GetTool would return. All supplied filters intersect.
type ToolListQuery struct {
	// NamespaceMatchers is the caller's effective namespace scope. Nil or empty
	// matches nothing, so a missing permission scope never lists every Tool.
	NamespaceMatchers []string `json:"namespaceMatchers"`

	// Name optionally selects one exact resource name.
	Name common.ResourceName `json:"name,omitempty"`

	// ConnectionId optionally selects Tools currently bound to one connection.
	ConnectionId apid.ID `json:"connectionId,omitempty"`

	// LabelSelector matches Tool labels, including inherited connection identity.
	LabelSelector string `json:"labelSelector,omitempty"`

	// Limit must be between one and MaxToolListLimit; there is no default.
	Limit int `json:"limit"`
}

// toolListCursor preserves the original query and exclusive ID position inside
// the shared encrypted cursor format, so continuation cannot change filters.
type toolListCursor struct {
	ToolListQuery
	AfterID apid.ID `json:"afterId"`
}

// ToolListPage is one ID-ordered page of live Tool identities. It contains
// management metadata only; definitions stay in the encrypted revision table.
type ToolListPage struct {
	Items []Tool

	// Cursor resumes through ListToolsFromCursor and is empty at the end.
	Cursor string
}

// validate checks every supplied filter and parses the label selector once.
func (q ToolListQuery) validate() (LabelSelector, error) {
	if q.Limit < 1 || q.Limit > MaxToolListLimit {
		return nil, fmt.Errorf("tool list limit must be between 1 and %d", MaxToolListLimit)
	}

	for _, matcher := range q.NamespaceMatchers {
		if err := namespace.ValidateMatcher(matcher); err != nil {
			return nil, fmt.Errorf("invalid tool namespace matcher: %w", err)
		}
	}

	if q.Name != "" {
		if err := q.Name.Validate(); err != nil {
			return nil, fmt.Errorf("invalid tool name: %w", err)
		}
	}

	if !q.ConnectionId.IsNil() {
		if err := validateToolConnectionID(q.ConnectionId); err != nil {
			return nil, err
		}
	}

	return ParseLabelSelector(q.LabelSelector)
}

// ListTools starts an ID-ordered management listing. Results include standalone
// and generated Tools in live namespaces, whether or not their connection is
// still live; callers must still apply per-resource permission checks.
func (s *service) ListTools(
	ctx context.Context,
	query ToolListQuery,
) (ToolListPage, error) {
	return s.listTools(ctx, query, apid.Nil)
}

// ListToolsFromCursor resumes a management listing with its saved filters and
// limit. Invalid cursors, including candidate-scan cursors, return an error.
func (s *service) ListToolsFromCursor(
	ctx context.Context,
	cursor string,
) (ToolListPage, error) {
	parsed, err := pagination.ParseCursor[toolListCursor](
		ctx,
		s.cursorEncryptor,
		cursor,
	)
	if err != nil {
		return ToolListPage{}, err
	}

	// Every continuation follows a returned Tool, so the position is required.
	if err := validateToolID(parsed.AfterID); err != nil {
		return ToolListPage{}, err
	}

	return s.listTools(ctx, parsed.ToolListQuery, parsed.AfterID)
}

// listTools reads one bounded page after the supplied position. A lookahead row
// produces a cursor carrying the original query and the last returned ID.
func (s *service) listTools(
	ctx context.Context,
	params ToolListQuery,
	afterID apid.ID,
) (ToolListPage, error) {
	selector, err := params.validate()
	if err != nil {
		return ToolListPage{}, err
	}

	page := ToolListPage{
		Items: []Tool{},
	}
	if len(params.NamespaceMatchers) == 0 {
		return page, nil
	}

	var row Tool
	q := s.sq.
		Select(util.PrependStringToAll("t.", row.cols())...).
		From(ToolsTable + " t").
		Join(NamespacesTable + " n ON n.path = t.namespace").
		Where(sq.Eq{
			"t.deleted_at": nil,
			"n.deleted_at": nil,
		}).
		OrderBy("t.id ASC").
		Limit(uint64(params.Limit + 1))

	// Literal prefix comparison avoids LIKE's wildcard and case behavior, matching
	// the candidate scan so namespace scope agrees on SQLite and PostgreSQL.
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
	if !params.ConnectionId.IsNil() {
		q = q.Where(sq.Eq{"t.connection_id": params.ConnectionId})
	}
	if !afterID.IsNil() {
		q = q.Where(sq.Gt{"t.id": afterID})
	}
	q = selector.ApplyToSqlBuilderWithProvider(q, "t.labels", s.cfg.GetProvider())

	rows, err := q.RunWith(s.db).QueryContext(ctx)
	if err != nil {
		return ToolListPage{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var item Tool
		if err := rows.Scan(item.fields()...); err != nil {
			return ToolListPage{}, err
		}
		page.Items = append(page.Items, item)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return ToolListPage{}, err
	}

	if len(page.Items) > params.Limit {
		page.Items = page.Items[:params.Limit]
		page.Cursor, err = pagination.MakeCursor(ctx, s.cursorEncryptor, toolListCursor{
			ToolListQuery: params,
			AfterID:       page.Items[len(page.Items)-1].Id,
		})
		if err != nil {
			return ToolListPage{}, err
		}
	}

	return page, nil
}

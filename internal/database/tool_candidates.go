package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// MaxToolCandidateLimit bounds one internal scan, independently of a public
// discovery page. Authorization may discard any or all candidates in a scan.
const MaxToolCandidateLimit = 100

// ToolCandidateQuery narrows an internal traversal of current Tool projections.
// It is not a public search request or permission check. All filters intersect,
// except that namespace matchers and verb aliases each match any member.
type ToolCandidateQuery struct {
	// NamespaceMatchers is an explicit scan scope. Nil or empty matches nothing.
	NamespaceMatchers []string `json:"namespaceMatchers"`
	// LabelSelector matches Tool labels, including inherited connection identity.
	LabelSelector string `json:"labelSelector"`
	// AnyVerbs matches exact literal aliases, never permission wildcard patterns.
	// Nil omits this prefilter; a nonnil empty slice matches nothing. The caller
	// must still check actor/JWT permission intersection for the same Tool alias.
	// Preserve nil versus empty when serializing a continuation cursor.
	AnyVerbs []string `json:"anyVerbs"`
	// Limit must be between one and MaxToolCandidateLimit; there is no default.
	Limit int `json:"limit"`
}

// toolCandidateCursor preserves the original query and exclusive scan position
// inside the shared encrypted cursor format. Embedding the query keeps cursor
// filters in sync with the initial request without exposing the position to callers.
type toolCandidateCursor struct {
	ToolCandidateQuery
	AfterID apid.ID `json:"afterId"`
}

// ToolCandidate contains current metadata needed for later authorization and
// discovery preparation. No executable source, schemas, annotations, resolved
// configuration, or credentials are read. It is not a public descriptor.
type ToolCandidate struct {
	Id             apid.ID
	Namespace      string
	Name           common.ResourceName
	ConnectionId   apid.ID
	ConnectionName common.ResourceName
	ActiveRevision uint64
	Description    string
	Verbs          []string
	Labels         Labels
	// ConnectionLabels is the current connection row's own label snapshot,
	// rather than the Tool's potentially lagging inherited label projection.
	ConnectionLabels Labels
}

// ToolCandidatePage is a bounded internal batch, not an authorized result page.
// Callers must keep scanning after rejected candidates, filter permissions before
// visible ranking/counts/pagination, and bind public cursors to principal, JWT
// scope, and query. Calls do not share a snapshot: inserts before the scan position
// are not revisited, and invocation must recheck the current Tool and parents.
type ToolCandidatePage struct {
	Items []ToolCandidate
	// Cursor resumes through ListToolCandidatesFromCursor with the same filters
	// and limit. It is empty at the end. The opaque cursor preserves a position
	// in the producing database's ID order; it does not confer authorization.
	Cursor string
}

// validate checks all supplied filters, including when an empty scope would
// return no rows. It parses the shared label syntax once for query construction.
func (q ToolCandidateQuery) validate() (LabelSelector, error) {
	if q.Limit < 1 || q.Limit > MaxToolCandidateLimit {
		return nil, fmt.Errorf("tool candidate limit must be between 1 and %d", MaxToolCandidateLimit)
	}
	for _, matcher := range q.NamespaceMatchers {
		if err := namespace.ValidateMatcher(matcher); err != nil {
			return nil, fmt.Errorf("invalid tool candidate namespace matcher: %w", err)
		}
	}
	for _, verb := range q.AnyVerbs {
		if strings.TrimSpace(verb) == "" || strings.TrimSpace(verb) != verb {
			return nil, errors.New("tool candidate verbs must be nonempty without surrounding whitespace")
		}
	}
	return ParseLabelSelector(q.LabelSelector)
}

// ListToolCandidates starts an ID-ordered traversal using only current metadata.
// Tools, their namespaces, connections, and active revisions must be live and
// agree on the binding. Missing legacy projections are excluded until prepared.
// Connection setup, authentication, and probe state do not filter this scan:
// later policy determines readiness and whether unavailable descriptors appear.
func (s *service) ListToolCandidates(ctx context.Context, params ToolCandidateQuery) (ToolCandidatePage, error) {
	return s.listToolCandidates(ctx, params, apid.Nil)
}

// ListToolCandidatesFromCursor resumes a candidate traversal with its saved
// filters, limit, and position. Invalid cursors return an error without querying
// candidates. The cursor protects scan state but does not authorize its results.
func (s *service) ListToolCandidatesFromCursor(ctx context.Context, cursor string) (ToolCandidatePage, error) {
	parsed, err := pagination.ParseCursor[toolCandidateCursor](ctx, s.cursorEncryptor, cursor)
	if err != nil {
		return ToolCandidatePage{}, err
	}
	// Every continuation follows a returned Tool. Requiring its ID also rejects
	// unrelated list cursors that lack this traversal's position.
	if err := validateToolID(parsed.AfterID); err != nil {
		return ToolCandidatePage{}, err
	}
	return s.listToolCandidates(ctx, parsed.ToolCandidateQuery, parsed.AfterID)
}

// listToolCandidates validates the query and reads a bounded batch after the
// supplied position. A lookahead row produces a cursor containing the original
// query and last returned ID, so callers do not need to retain either themselves.
func (s *service) listToolCandidates(ctx context.Context, params ToolCandidateQuery, afterID apid.ID) (ToolCandidatePage, error) {
	selector, err := params.validate()
	if err != nil {
		return ToolCandidatePage{}, err
	}
	if err := ctx.Err(); err != nil {
		return ToolCandidatePage{}, err
	}
	page := ToolCandidatePage{Items: []ToolCandidate{}}
	if len(params.NamespaceMatchers) == 0 || (params.AnyVerbs != nil && len(params.AnyVerbs) == 0) {
		return page, nil
	}

	q := s.sq.Select(
		"t.id", "t.namespace", "t.name", "t.connection_id", "c.name",
		"t.active_revision", "t.description", "t.verbs", "t.labels", "c.labels",
	).
		From(ToolsTable + " t").
		Join(NamespacesTable + " n ON n.path = t.namespace").
		Join(ConnectionsTable + " c ON c.id = t.connection_id AND c.namespace = t.namespace").
		Join(ToolRevisionsTable + " r ON r.tool_id = t.id AND r.revision = t.active_revision AND r.connection_id = t.connection_id").
		Where(sq.Eq{"t.deleted_at": nil, "n.deleted_at": nil, "c.deleted_at": nil, "r.deleted_at": nil}).
		Where(sq.NotEq{"t.description": ""}).
		OrderBy("t.id ASC").
		Limit(uint64(params.Limit + 1))

	// Namespace paths are ASCII. A literal prefix comparison preserves case and
	// underscores on both providers, unlike LIKE's wildcard/case behavior.
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
	q = selector.ApplyToSqlBuilderWithProvider(q.Where(scopes), "t.labels", s.cfg.GetProvider())
	if !afterID.IsNil() {
		q = q.Where(sq.Gt{"t.id": afterID})
	}

	iterator := "json_each(t.verbs) AS tool_verb"
	if s.cfg.GetProvider() == config.DatabaseProviderPostgres {
		iterator = "jsonb_array_elements_text(t.verbs) AS tool_verb(value)"
	}
	// Keep placeholders relative until the outer query numbers all parameters.
	// EXISTS also excludes NULL/empty legacy verb lists without duplicating a
	// Tool when multiple aliases match the filter.
	verbs := s.sq.Select("1").From(iterator).PlaceholderFormat(sq.Question)
	if params.AnyVerbs != nil {
		verbs = verbs.Where(sq.Eq{"tool_verb.value": params.AnyVerbs})
	}
	q = q.Where(sq.Expr("EXISTS (?)", verbs))

	rows, err := q.RunWith(s.db).QueryContext(ctx)
	if err != nil {
		return ToolCandidatePage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ToolCandidate
		if err := rows.Scan(&item.Id, &item.Namespace, &item.Name, &item.ConnectionId,
			&item.ConnectionName, &item.ActiveRevision, &item.Description,
			(*toolVerbsDB)(&item.Verbs), &item.Labels, &item.ConnectionLabels); err != nil {
			return ToolCandidatePage{}, err
		}
		page.Items = append(page.Items, item)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return ToolCandidatePage{}, err
	}
	if len(page.Items) > params.Limit {
		page.Items = page.Items[:params.Limit]
		page.Cursor, err = pagination.MakeCursor(ctx, s.cursorEncryptor, toolCandidateCursor{
			ToolCandidateQuery: params,
			AfterID:            page.Items[len(page.Items)-1].Id,
		})
		if err != nil {
			return ToolCandidatePage{}, err
		}
	}
	return page, nil
}

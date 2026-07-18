package database

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// SearchResourceType identifies a durable resource supported by the Admin UI
// command palette. Request events and task/workflow monitoring records are
// intentionally excluded because they are not stored in the primary database.
type SearchResourceType string

const (
	SearchResourceTypeActor      SearchResourceType = "actor"
	SearchResourceTypeConnection SearchResourceType = "connection"
	SearchResourceTypeConnector  SearchResourceType = "connector"
	SearchResourceTypeNamespace  SearchResourceType = "namespace"
	SearchResourceTypeKey        SearchResourceType = "key"
	SearchResourceTypeRateLimit  SearchResourceType = "rate_limit"
)

var searchResourceTypes = []SearchResourceType{
	SearchResourceTypeActor,
	SearchResourceTypeConnection,
	SearchResourceTypeConnector,
	SearchResourceTypeNamespace,
	SearchResourceTypeKey,
	SearchResourceTypeRateLimit,
}

func SearchResourceTypes() []SearchResourceType {
	return append([]SearchResourceType(nil), searchResourceTypes...)
}

func IsValidSearchResourceType(t SearchResourceType) bool {
	for _, candidate := range searchResourceTypes {
		if t == candidate {
			return true
		}
	}
	return false
}

type SearchResourcesParams struct {
	ResourceType      SearchResourceType
	Query             string
	LabelSelector     string
	NamespaceMatchers []string
	Limit             int
}

type SearchLabelMatch struct {
	Key   string
	Value string
}

type SearchResource struct {
	ResourceType  SearchResourceType
	ResourceID    string
	Namespace     string
	Labels        Labels
	MatchedLabels []SearchLabelMatch
	UpdatedAt     time.Time
	MatchRank     int
}

type SearchResourcesResult struct {
	Items     []SearchResource
	Truncated bool
}

// ResourceSearcher is the replaceable database-facing search contract. The
// live database service implements it with bounded provider-specific queries;
// callers can substitute a projection-backed implementation later without
// changing the Admin API contract.
type ResourceSearcher interface {
	SearchResources(ctx context.Context, params SearchResourcesParams) (SearchResourcesResult, error)
}

type searchSource struct {
	from              string
	idExpression      string
	nsExpression      string
	labelsExpression  string
	updatedExpression string
	withName          string
	withSQL           string
	extraWhere        sq.Sqlizer
}

func searchSourceFor(resourceType SearchResourceType) (searchSource, error) {
	switch resourceType {
	case SearchResourceTypeActor:
		return searchSource{from: "actors src", idExpression: "src.id", nsExpression: "src.namespace", labelsExpression: "src.labels", updatedExpression: "src.updated_at"}, nil
	case SearchResourceTypeConnection:
		return searchSource{from: "connections src", idExpression: "src.id", nsExpression: "src.namespace", labelsExpression: "src.labels", updatedExpression: "src.updated_at"}, nil
	case SearchResourceTypeNamespace:
		return searchSource{from: "namespaces src", idExpression: "src.path", nsExpression: "src.path", labelsExpression: "src.labels", updatedExpression: "src.updated_at"}, nil
	case SearchResourceTypeKey:
		return searchSource{from: "keys src", idExpression: "src.id", nsExpression: "src.namespace", labelsExpression: "src.labels", updatedExpression: "src.updated_at"}, nil
	case SearchResourceTypeRateLimit:
		return searchSource{from: "rate_limits src", idExpression: "src.id", nsExpression: "src.namespace", labelsExpression: "src.labels", updatedExpression: "src.updated_at"}, nil
	case SearchResourceTypeConnector:
		return searchSource{
			from:              "search_connector_rows src",
			idExpression:      "src.id",
			nsExpression:      "src.namespace",
			labelsExpression:  "src.labels",
			updatedExpression: "src.updated_at",
			withName:          "search_connector_rows",
			withSQL: "\n" +
				"SELECT id, namespace, labels, updated_at, state, version, ROW_NUMBER() OVER (\n" +
				"  PARTITION BY id\n" +
				"  ORDER BY CASE state\n" +
				"    WHEN 'primary' THEN 1\n" +
				"    WHEN 'draft' THEN 2\n" +
				"    WHEN 'active' THEN 3\n" +
				"    WHEN 'archived' THEN 4\n" +
				"    ELSE 5\n" +
				"  END, version DESC\n" +
				") AS search_row_num\n" +
				"FROM connector_versions\n" +
				"WHERE deleted_at IS NULL\n",
			extraWhere: sq.Eq{"src.search_row_num": 1},
		}, nil
	default:
		return searchSource{}, fmt.Errorf("unsupported search resource type %q", resourceType)
	}
}

// SearchResources performs a bounded search for exactly one resource type.
// Cross-type concurrency, deadlines, authorization, and result merging live
// at the route layer, where permissions differ by resource type.
func (s *service) SearchResources(ctx context.Context, params SearchResourcesParams) (SearchResourcesResult, error) {
	if !IsValidSearchResourceType(params.ResourceType) {
		return SearchResourcesResult{}, fmt.Errorf("invalid search resource type %q", params.ResourceType)
	}
	if params.Limit <= 0 || params.Limit > 50 {
		return SearchResourcesResult{}, fmt.Errorf("search limit must be between 1 and 50")
	}
	// Search callers pass the permission-constrained namespace matchers for
	// one resource type. Unlike the general list builders, an empty set means
	// the actor has no access, never an unrestricted search.
	if len(params.NamespaceMatchers) == 0 {
		return SearchResourcesResult{Items: []SearchResource{}}, nil
	}
	for _, matcher := range params.NamespaceMatchers {
		if err := namespace.ValidateMatcher(matcher); err != nil {
			return SearchResourcesResult{}, fmt.Errorf("invalid namespace matcher %q: %w", matcher, err)
		}
	}

	source, err := searchSourceFor(params.ResourceType)
	if err != nil {
		return SearchResourcesResult{}, err
	}

	queryText := strings.ToLower(strings.TrimSpace(params.Query))
	rankSQL := "0"
	rankArgs := []interface{}{}
	if queryText != "" {
		exact := labelValuePredicateSQL(s.cfg.GetProvider(), source.labelsExpression, "=")
		prefix := labelValuePredicateSQL(s.cfg.GetProvider(), source.labelsExpression, "LIKE")
		rankSQL = fmt.Sprintf("CASE WHEN %s THEN 0 WHEN %s THEN 1 ELSE 2 END", exact, prefix)
		rankArgs = append(rankArgs, queryText, escapeLike(queryText)+"%")
	}

	q := s.sq.Select(
		source.idExpression,
		source.nsExpression,
		source.labelsExpression,
		source.updatedExpression,
	).Column(sq.Expr(rankSQL, rankArgs...)).From(source.from)

	if source.withName != "" {
		q = q.With(source.withName, sq.Expr(source.withSQL))
	} else {
		q = q.Where(sq.Eq{"src.deleted_at": nil})
	}
	if source.extraWhere != nil {
		q = q.Where(source.extraWhere)
	}
	q = restrictToNamespaceMatchers(q, source.nsExpression, params.NamespaceMatchers)

	var selector LabelSelector
	if params.LabelSelector != "" {
		selector, err = ParseLabelSelector(params.LabelSelector)
		if err != nil {
			return SearchResourcesResult{}, err
		}
		q = selector.ApplyToSqlBuilderWithProvider(q, source.labelsExpression, s.cfg.GetProvider())
	}

	if queryText != "" {
		contains := labelValuePredicateSQL(s.cfg.GetProvider(), source.labelsExpression, "LIKE")
		q = q.Where(sq.Expr(contains, "%"+escapeLike(queryText)+"%"))
	}

	orderBy := make([]string, 0, 3)
	if queryText != "" {
		orderBy = append(orderBy, "5 ASC")
	}
	orderBy = append(orderBy, source.updatedExpression+" DESC", source.idExpression+" ASC")
	q = q.OrderBy(orderBy...).Limit(uint64(params.Limit + 1))
	sqlText, args, err := q.ToSql()
	if err != nil {
		return SearchResourcesResult{}, err
	}

	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return SearchResourcesResult{}, err
	}
	defer rows.Close()

	items := make([]SearchResource, 0, params.Limit+1)
	for rows.Next() {
		var item SearchResource
		var storedLabels Labels
		if err := rows.Scan(&item.ResourceID, &item.Namespace, &storedLabels, &item.UpdatedAt, &item.MatchRank); err != nil {
			return SearchResourcesResult{}, err
		}
		item.ResourceType = params.ResourceType
		item.Labels, _ = SplitUserAndApxyLabels(storedLabels)
		item.MatchedLabels = searchMatchedLabels(storedLabels, item.Labels, selector, queryText)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return SearchResourcesResult{}, err
	}

	truncated := len(items) > params.Limit
	if truncated {
		items = items[:params.Limit]
	}
	return SearchResourcesResult{Items: items, Truncated: truncated}, nil
}

func labelValuePredicateSQL(provider config.DatabaseProvider, labelsExpression, operator string) string {
	valueExpression := "LOWER(CAST(search_label.value AS TEXT))"
	iterator := fmt.Sprintf("json_each(COALESCE(%s, '{}')) AS search_label", labelsExpression)
	if provider == config.DatabaseProviderPostgres {
		valueExpression = "LOWER(search_label.value)"
		iterator = fmt.Sprintf("jsonb_each_text(COALESCE(%s, '{}'::jsonb)) AS search_label(key, value)", labelsExpression)
	}

	if operator == "=" {
		return fmt.Sprintf("EXISTS (SELECT 1 FROM %s WHERE CAST(search_label.key AS TEXT) NOT LIKE 'apxy/%%' AND %s = ?)", iterator, valueExpression)
	}
	return fmt.Sprintf("EXISTS (SELECT 1 FROM %s WHERE CAST(search_label.key AS TEXT) NOT LIKE 'apxy/%%' AND %s LIKE ? ESCAPE '\\')", iterator, valueExpression)
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return replacer.Replace(value)
}

func searchMatchedLabels(stored, user Labels, selector LabelSelector, queryText string) []SearchLabelMatch {
	matches := make(map[string]string)
	if queryText != "" {
		for key, value := range user {
			if strings.Contains(strings.ToLower(value), queryText) {
				matches[key] = value
			}
		}
	}
	for _, requirement := range selector {
		value, ok := stored[requirement.Key]
		if !ok {
			continue
		}
		switch requirement.Operator {
		case LabelOperatorEqual, LabelOperatorExists:
			matches[requirement.Key] = value
		case LabelOperatorNotEqual:
			if value != requirement.Value {
				matches[requirement.Key] = value
			}
		}
	}

	keys := make([]string, 0, len(matches))
	for key := range matches {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]SearchLabelMatch, 0, len(keys))
	for _, key := range keys {
		result = append(result, SearchLabelMatch{Key: key, Value: matches[key]})
	}
	return result
}

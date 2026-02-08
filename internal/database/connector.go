package database

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

/*
 * This file deals with listing connectors collapsed from multiple versions. Most logic is connector version specific.
 */

// Connector object is returned from queries for connectors, with one record per id. It aggregates some information
// across all versions for a connector.
type Connector struct {
	ConnectorVersion
	TotalVersions int64
	States        ConnectorVersionStates
}

type ConnectorOrderByField string

const (
	ConnectorOrderById        ConnectorOrderByField = "id"
	ConnectorOrderByVersion   ConnectorOrderByField = "version"
	ConnectorOrderByNamespace ConnectorOrderByField = "namespace"
	ConnectorOrderByState     ConnectorOrderByField = "state"
	ConnectorOrderByCreatedAt ConnectorOrderByField = "created_at"
	ConnectorOrderByUpdatedAt ConnectorOrderByField = "updated_at"
	ConnectorOrderByType      ConnectorOrderByField = "type"
)

func IsValidConnectorOrderByField[T string | ConnectorOrderByField](field T) bool {
	switch ConnectorOrderByField(field) {
	case ConnectorOrderById,
		ConnectorOrderByVersion,
		ConnectorOrderByNamespace,
		ConnectorOrderByState,
		ConnectorOrderByCreatedAt,
		ConnectorOrderByUpdatedAt,
		ConnectorOrderByType:
		return true
	default:
		return false
	}
}

type ListConnectorsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Connector]
	Enumerate(context.Context, func(pagination.PageResult[Connector]) (keepGoing bool, err error)) error
}

type ListConnectorsBuilder interface {
	ListConnectorsExecutor
	Limit(int32) ListConnectorsBuilder
	ForType(string) ListConnectorsBuilder
	ForId(uuid.UUID) ListConnectorsBuilder
	ForNamespaceMatcher(string) ListConnectorsBuilder
	ForNamespaceMatchers([]string) ListConnectorsBuilder
	ForState(ConnectorVersionState) ListConnectorsBuilder
	ForStates([]ConnectorVersionState) ListConnectorsBuilder
	OrderBy(ConnectorOrderByField, pagination.OrderBy) ListConnectorsBuilder
	IncludeDeleted() ListConnectorsBuilder
	ForLabelSelector(selector string) ListConnectorsBuilder
}

type listConnectorsFilters struct {
	s                 *service                `json:"-"`
	LimitVal          uint64                  `json:"limit"`
	Offset            uint64                  `json:"offset"`
	StatesVal         []ConnectorVersionState `json:"states,omitempty"`
	NamespaceMatchers []string                `json:"namespace_matchers,omitempty"`
	TypeVal           []string                `json:"types,omitempty"`
	IdsVal            []uuid.UUID             `json:"ids,omitempty"`
	OrderByFieldVal   *ConnectorOrderByField  `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy     `json:"order_by"`
	IncludeDeletedVal bool                    `json:"include_deleted,omitempty"`
	LabelSelectorVal  *string                 `json:"label_selector,omitempty"`
	Errors            *multierror.Error       `json:"-"`
}

func (l *listConnectorsFilters) addError(e error) ListConnectorsBuilder {
	l.Errors = multierror.Append(l.Errors, e)
	return l
}

func (l *listConnectorsFilters) Limit(limit int32) ListConnectorsBuilder {
	l.LimitVal = uint64(limit)
	return l
}

func (l *listConnectorsFilters) ForState(state ConnectorVersionState) ListConnectorsBuilder {
	l.StatesVal = []ConnectorVersionState{state}
	return l
}

func (l *listConnectorsFilters) ForStates(states []ConnectorVersionState) ListConnectorsBuilder {
	l.StatesVal = states
	return l
}

func (l *listConnectorsFilters) ForNamespaceMatcher(matcher string) ListConnectorsBuilder {
	if err := ValidateNamespaceMatcher(matcher); err != nil {
		return l.addError(err)
	} else {
		l.NamespaceMatchers = []string{matcher}
	}

	return l
}

func (l *listConnectorsFilters) ForNamespaceMatchers(matchers []string) ListConnectorsBuilder {
	for _, matcher := range matchers {
		if err := ValidateNamespaceMatcher(matcher); err != nil {
			return l.addError(err)
		}
	}
	l.NamespaceMatchers = matchers
	return l
}

func (l *listConnectorsFilters) ForType(t string) ListConnectorsBuilder {
	l.TypeVal = []string{t}
	return l
}

func (l *listConnectorsFilters) ForId(id uuid.UUID) ListConnectorsBuilder {
	l.IdsVal = []uuid.UUID{id}
	return l
}

func (l *listConnectorsFilters) OrderBy(field ConnectorOrderByField, by pagination.OrderBy) ListConnectorsBuilder {
	if IsValidConnectorOrderByField(field) {
		l.OrderByFieldVal = &field
		l.OrderByVal = &by
	}
	return l
}

func (l *listConnectorsFilters) IncludeDeleted() ListConnectorsBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listConnectorsFilters) ForLabelSelector(selector string) ListConnectorsBuilder {
	l.LabelSelectorVal = &selector
	return l
}

func (l *listConnectorsFilters) FromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error) {
	s := l.s
	parsed, err := pagination.ParseCursor[listConnectorsFilters](ctx, s.secretKey, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.s = s

	return l, nil
}

func (l *listConnectorsFilters) fetchPage(ctx context.Context) pagination.PageResult[Connector] {
	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	if err := l.Errors.ErrorOrNil(); err != nil {
		return pagination.PageResult[Connector]{Error: err}
	}

	// Picks out the row that will be returned as primary based on a ranked priority of the states
	rankedRowsCTE := fmt.Sprintf(`
        SELECT
            *,
            ROW_NUMBER() OVER (
                PARTITION BY id
                ORDER BY
                    CASE state
                        WHEN 'primary' THEN 1
                        WHEN 'draft' THEN 2
                        WHEN 'active' THEN 3
                        WHEN 'archived' THEN 4
                        ELSE 5
                    END
            ) AS row_num
        FROM %s
    `, ConnectorVersionsTable)

	// Compute the aggregate state for the connector across all versions
	connectorVersionCountsCTE := fmt.Sprintf(`
        SELECT
            id,
            json_group_array(distinct state) as states,
            count(*) as versions
        FROM %s
        GROUP BY id
    `, ConnectorVersionsTable)

	q := l.s.sq.Select(`
rr.id as id,
rr.namespace as namespace,
rr.labels as labels,
rr.version as version,
rr.state as state,
COALESCE(rr.encrypted_definition, "") as encrypted_definition,
rr.created_at as created_at,
rr.updated_at as updated_at,
rr.deleted_at as deleted_at,
cvc.states as states, 
cvc.versions as total_versions
`).
		With("ranked_rows", sq.Expr(rankedRowsCTE)).
		With("connector_version_counts", sq.Expr(connectorVersionCountsCTE)).
		From("ranked_rows rr").
		Join("connector_version_counts cvc ON cvc.id = rr.id").
		Where("rr.row_num = ?", 1)

	if len(l.TypeVal) > 0 {
		q = q.Where(sq.Eq{"rr.type": l.TypeVal})
	}

	if len(l.IdsVal) > 0 {
		q = q.Where(sq.Eq{"rr.id": l.IdsVal})
	}

	if len(l.StatesVal) > 0 {
		q = q.Where(sq.Eq{"rr.state": l.StatesVal})
	}

	if len(l.NamespaceMatchers) > 0 {
		q = restrictToNamespaceMatchers(q, "rr.namespace", l.NamespaceMatchers)
	}

	if l.LabelSelectorVal != nil {
		selector, err := ParseLabelSelector(*l.LabelSelectorVal)
		if err != nil {
			return pagination.PageResult[Connector]{Error: err}
		}

		q = selector.ApplyToSqlBuilderWithProvider(q, "rr.labels", l.s.cfg.GetProvider())
	}

	if !l.IncludeDeletedVal {
		q = q.Where(sq.Eq{"rr.deleted_at": nil})
	}

	// Always limit to one more than limit to check if there are more records
	q = q.Limit(l.LimitVal + 1).Offset(l.Offset)

	if l.OrderByFieldVal != nil {
		q = q.OrderBy(fmt.Sprintf("%s %s", *l.OrderByFieldVal, l.OrderByVal.String()))
	}

	rows, err := q.RunWith(l.s.db).Query()

	if err != nil {
		return pagination.PageResult[Connector]{Error: err}
	}

	var connectors []Connector
	for rows.Next() {
		var c Connector

		// Scan all fields except States
		err := rows.Scan(
			&c.Id,
			&c.Namespace,
			&c.Labels,
			&c.Version,
			&c.State,
			&c.EncryptedDefinition,
			&c.CreatedAt,
			&c.UpdatedAt,
			&c.DeletedAt,
			&c.States,
			&c.TotalVersions,
		)
		if err != nil {
			return pagination.PageResult[Connector]{Error: err}
		}

		connectors = append(connectors, c)
	}

	l.Offset = l.Offset + uint64(len(connectors)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := uint64(len(connectors)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.secretKey, l)
		if err != nil {
			return pagination.PageResult[Connector]{Error: err}
		}
	}

	return pagination.PageResult[Connector]{
		HasMore: hasMore,
		Results: connectors[:util.MinUint64(l.LimitVal, uint64(len(connectors)))],
		Cursor:  cursor,
	}
}

func (l *listConnectorsFilters) FetchPage(ctx context.Context) pagination.PageResult[Connector] {
	return l.fetchPage(ctx)
}

func (l *listConnectorsFilters) Enumerate(ctx context.Context, callback func(pagination.PageResult[Connector]) (keepGoing bool, err error)) error {
	var err error
	keepGoing := true
	hasMore := true

	for err == nil && hasMore && keepGoing {
		result := l.FetchPage(ctx)
		hasMore = result.HasMore

		if result.Error != nil {
			return result.Error
		}
		keepGoing, err = callback(result)
	}

	return err
}

func (s *service) ListConnectorsBuilder() ListConnectorsBuilder {
	return &listConnectorsFilters{
		s:        s,
		LimitVal: 100,
	}
}

func (s *service) ListConnectorsFromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error) {
	b := &listConnectorsFilters{
		s:        s,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}

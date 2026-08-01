package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

/*
 * This file deals with connector queries that combine a logical connector with
 * one of its definition versions.
 */

// ConnectorWithDefinition combines fields from connectors and
// connector_definition_versions.
type ConnectorWithDefinition struct {
	Id                  apid.ID
	Namespace           string
	Name                scommon.ResourceName
	DefinitionVersionId apid.ID
	Version             uint64
	State               ConnectorDefinitionVersionState
	EncryptedDefinition encfield.EncryptedField
	Labels              Labels
	Annotations         Annotations
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DefinitionCreatedAt time.Time
	DefinitionUpdatedAt time.Time
	EncryptedAt         *time.Time
	DeletedAt           *time.Time
}

func connectorWithDefinitionSelectCols() []string {
	return []string{
		"c.id",
		"c.namespace",
		"c.name",
		"dv.id",
		"dv.version",
		"dv.state",
		"dv.encrypted_definition",
		"c.labels",
		"c.annotations",
		"c.created_at",
		"c.updated_at",
		"dv.created_at",
		"dv.updated_at",
		"dv.encrypted_at",
		"c.deleted_at",
	}
}

func (cv *ConnectorWithDefinition) fields() []any {
	return []any{
		&cv.Id,
		&cv.Namespace,
		&cv.Name,
		&cv.DefinitionVersionId,
		&cv.Version,
		&cv.State,
		&cv.EncryptedDefinition,
		&cv.Labels,
		&cv.Annotations,
		&cv.CreatedAt,
		&cv.UpdatedAt,
		&cv.DefinitionCreatedAt,
		&cv.DefinitionUpdatedAt,
		&cv.EncryptedAt,
		&cv.DeletedAt,
	}
}

func (cv *ConnectorWithDefinition) definitionVersion() ConnectorDefinitionVersion {
	return ConnectorDefinitionVersion{
		Id:                  cv.DefinitionVersionId,
		ConnectorId:         cv.Id,
		Version:             cv.Version,
		State:               cv.State,
		EncryptedDefinition: cv.EncryptedDefinition,
		CreatedAt:           cv.DefinitionCreatedAt,
		UpdatedAt:           cv.DefinitionUpdatedAt,
		EncryptedAt:         cv.EncryptedAt,
	}
}

func (cv *ConnectorWithDefinition) GetId() apid.ID {
	return cv.Id
}

func (cv *ConnectorWithDefinition) GetNamespace() string {
	return cv.Namespace
}

func (cv *ConnectorWithDefinition) GetVersion() uint64 {
	return cv.Version
}

func (cv *ConnectorWithDefinition) Validate() error {
	result := &multierror.Error{}

	if cv.Id.IsNil() {
		result = multierror.Append(result, errors.New("connector id is required"))
	}
	if err := cv.Id.ValidatePrefix(apid.PrefixConnector); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connector id: %w", err))
	}
	if err := namespace.ValidatePath(cv.Namespace); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connector namespace: %w", err))
	}
	if cv.Name != "" {
		if err := cv.Name.Validate(); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid connector name: %w", err))
		}
	}
	if err := cv.Labels.Validate(); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connector labels: %w", err))
	}
	if err := cv.Annotations.Validate(); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connector annotations: %w", err))
	}

	definitionVersion := cv.definitionVersion()
	if definitionVersion.Id.IsNil() {
		definitionVersion.Id = apid.New(apid.PrefixConnectorDefinitionVersion)
	}
	if err := definitionVersion.Validate(); err != nil {
		result = multierror.Append(result, err)
	}

	return result.ErrorOrNil()
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
	FetchPage(context.Context) pagination.PageResult[ConnectorWithDefinition]
	Enumerate(context.Context, pagination.EnumerateCallback[ConnectorWithDefinition]) error
}

type ListConnectorsBuilder interface {
	ListConnectorsExecutor
	Limit(int32) ListConnectorsBuilder
	ForType(string) ListConnectorsBuilder
	ForId(apid.ID) ListConnectorsBuilder
	ForNamespaceMatcher(string) ListConnectorsBuilder
	ForNamespaceMatchers([]string) ListConnectorsBuilder
	ForName(name scommon.ResourceName) ListConnectorsBuilder
	ForState(ConnectorDefinitionVersionState) ListConnectorsBuilder
	ForStates([]ConnectorDefinitionVersionState) ListConnectorsBuilder
	OrderBy(ConnectorOrderByField, pagination.OrderBy) ListConnectorsBuilder
	IncludeDeleted() ListConnectorsBuilder
	ForLabelSelector(selector string) ListConnectorsBuilder
}

type listConnectorsFilters struct {
	s                 *service                          `json:"-"`
	LimitVal          uint64                            `json:"limit"`
	Offset            uint64                            `json:"offset"`
	StatesVal         []ConnectorDefinitionVersionState `json:"states,omitempty"`
	NamespaceMatchers []string                          `json:"namespace_matchers,omitempty"`
	TypeVal           []string                          `json:"types,omitempty"`
	IdsVal            []apid.ID                         `json:"ids,omitempty"`
	NameVal           *scommon.ResourceName             `json:"name,omitempty"`
	OrderByFieldVal   *ConnectorOrderByField            `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy               `json:"order_by"`
	IncludeDeletedVal bool                              `json:"include_deleted,omitempty"`
	LabelSelectorVal  *string                           `json:"label_selector,omitempty"`
	Errors            *multierror.Error                 `json:"-"`
}

func (l *listConnectorsFilters) addError(e error) ListConnectorsBuilder {
	l.Errors = multierror.Append(l.Errors, e)
	return l
}

func (l *listConnectorsFilters) Limit(limit int32) ListConnectorsBuilder {
	l.LimitVal = uint64(limit)
	return l
}

func (l *listConnectorsFilters) ForState(state ConnectorDefinitionVersionState) ListConnectorsBuilder {
	l.StatesVal = []ConnectorDefinitionVersionState{state}
	return l
}

func (l *listConnectorsFilters) ForStates(states []ConnectorDefinitionVersionState) ListConnectorsBuilder {
	l.StatesVal = states
	return l
}

func (l *listConnectorsFilters) ForNamespaceMatcher(matcher string) ListConnectorsBuilder {
	if err := namespace.ValidateMatcher(matcher); err != nil {
		return l.addError(err)
	} else {
		l.NamespaceMatchers = []string{matcher}
	}

	return l
}

func (l *listConnectorsFilters) ForNamespaceMatchers(matchers []string) ListConnectorsBuilder {
	for _, matcher := range matchers {
		if err := namespace.ValidateMatcher(matcher); err != nil {
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

func (l *listConnectorsFilters) ForId(id apid.ID) ListConnectorsBuilder {
	l.IdsVal = []apid.ID{id}
	return l
}

func (l *listConnectorsFilters) ForName(name scommon.ResourceName) ListConnectorsBuilder {
	if err := name.Validate(); err != nil {
		return l.addError(err)
	}
	l.NameVal = &name
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
	parsed, err := pagination.ParseCursor[listConnectorsFilters](ctx, s.cursorEncryptor, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.s = s

	return l, nil
}

func (l *listConnectorsFilters) fetchPage(ctx context.Context) pagination.PageResult[ConnectorWithDefinition] {
	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	if err := l.Errors.ErrorOrNil(); err != nil {
		return pagination.PageResult[ConnectorWithDefinition]{Error: err}
	}

	// Picks out the row that will be returned as primary based on a ranked priority of the states
	rankedRowsCTE := fmt.Sprintf(`
        SELECT
            *,
            ROW_NUMBER() OVER (
                PARTITION BY connector_id
                ORDER BY
                    CASE state
                        WHEN 'primary' THEN 1
                        WHEN 'draft' THEN 2
                        WHEN 'active' THEN 3
                        WHEN 'archived' THEN 4
                        ELSE 5
                    END,
                    version DESC
            ) AS row_num
        FROM %s
    `, ConnectorDefinitionVersionsTable)

	q := l.s.sq.Select(`
c.id as id,
c.namespace as namespace,
c.name as name,
rr.id as definition_version_id,
rr.version as version,
rr.state as state,
rr.encrypted_definition as encrypted_definition,
c.labels as labels,
c.annotations as annotations,
c.created_at as created_at,
c.updated_at as updated_at,
rr.created_at as definition_created_at,
rr.updated_at as definition_updated_at,
rr.encrypted_at as encrypted_at,
c.deleted_at as deleted_at
`).
		With("ranked_rows", sq.Expr(rankedRowsCTE)).
		From(ConnectorsTable+" c").
		Join("ranked_rows rr ON rr.connector_id = c.id").
		Where("rr.row_num = ?", 1)

	if len(l.TypeVal) > 0 {
		typeExpr := "json_extract(c.labels, '$.type')"
		if l.s.cfg.GetProvider() == sconfig.DatabaseProviderPostgres {
			typeExpr = "c.labels ->> 'type'"
		}
		q = q.Where(sq.Eq{typeExpr: l.TypeVal})
	}

	if len(l.IdsVal) > 0 {
		q = q.Where(sq.Eq{"c.id": l.IdsVal})
	}

	if l.NameVal != nil {
		q = q.Where(sq.Eq{"c.name": *l.NameVal})
	}

	if len(l.StatesVal) > 0 {
		q = q.Where(sq.Eq{"rr.state": l.StatesVal})
	}

	if len(l.NamespaceMatchers) > 0 {
		q = restrictToNamespaceMatchers(q, "c.namespace", l.NamespaceMatchers)
	}

	if l.LabelSelectorVal != nil {
		selector, err := ParseLabelSelector(*l.LabelSelectorVal)
		if err != nil {
			return pagination.PageResult[ConnectorWithDefinition]{Error: err}
		}

		q = selector.ApplyToSqlBuilderWithProvider(q, "c.labels", l.s.cfg.GetProvider())
	}

	if !l.IncludeDeletedVal {
		q = q.Where(sq.Eq{"c.deleted_at": nil})
	}

	// Always limit to one more than limit to check if there are more records
	q = q.Limit(l.LimitVal + 1).Offset(l.Offset)

	if l.OrderByFieldVal != nil {
		orderCol := string(*l.OrderByFieldVal)
		switch *l.OrderByFieldVal {
		case ConnectorOrderByVersion, ConnectorOrderByState:
			orderCol = "rr." + orderCol
		case ConnectorOrderByType:
			orderCol = "json_extract(c.labels, '$.type')"
			if l.s.cfg.GetProvider() == sconfig.DatabaseProviderPostgres {
				orderCol = "c.labels ->> 'type'"
			}
		default:
			orderCol = "c." + orderCol
		}
		q = q.OrderBy(fmt.Sprintf("%s %s", orderCol, l.OrderByVal.String()))
	}

	sqlStr, sqlArgs, sqlErr := q.ToSql()
	rows, err := q.RunWith(l.s.db).Query()

	if err != nil {
		if sqlErr == nil {
			l.s.logger.Error("list connectors query failed", "sql", sqlStr, "args", sqlArgs, "error", err)
		} else {
			l.s.logger.Error("list connectors query failed", "error", err, "sql_error", sqlErr)
		}
		return pagination.PageResult[ConnectorWithDefinition]{Error: err}
	}

	var connectors []ConnectorWithDefinition
	for rows.Next() {
		var c ConnectorWithDefinition
		err := rows.Scan(c.fields()...)
		if err != nil {
			return pagination.PageResult[ConnectorWithDefinition]{Error: err}
		}

		connectors = append(connectors, c)
	}

	l.Offset = l.Offset + uint64(len(connectors)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := uint64(len(connectors)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.cursorEncryptor, l)
		if err != nil {
			return pagination.PageResult[ConnectorWithDefinition]{Error: err}
		}
	}

	return pagination.PageResult[ConnectorWithDefinition]{
		HasMore: hasMore,
		Results: connectors[:util.MinUint64(l.LimitVal, uint64(len(connectors)))],
		Cursor:  cursor,
	}
}

func (l *listConnectorsFilters) FetchPage(ctx context.Context) pagination.PageResult[ConnectorWithDefinition] {
	return l.fetchPage(ctx)
}

func (l *listConnectorsFilters) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[ConnectorWithDefinition]) error {
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

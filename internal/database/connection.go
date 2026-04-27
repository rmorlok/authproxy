package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

func init() {
	RegisterEncryptedField(EncryptedFieldRegistration{
		Table:          ConnectionsTable,
		PrimaryKeyCols: []string{"id"},
		EncryptedCols:  []string{"encrypted_configuration"},
		NamespaceCol:   "namespace",
	})
}

type ConnectionState string

const (
	ConnectionStateCreated       ConnectionState = "created"
	ConnectionStateReady         ConnectionState = "ready"
	ConnectionStateDisabled      ConnectionState = "disabled"
	ConnectionStateDisconnecting ConnectionState = "disconnecting"
	ConnectionStateDisconnected  ConnectionState = "disconnected"
)

func IsValidConnectionState[T string | ConnectionState](state T) bool {
	switch ConnectionState(state) {
	case ConnectionStateCreated,
		ConnectionStateReady,
		ConnectionStateDisabled,
		ConnectionStateDisconnecting,
		ConnectionStateDisconnected:
		return true
	default:
		return false
	}
}

const ConnectionsTable = "connections"

type Connection struct {
	Id                      apid.ID
	Namespace               string
	State                   ConnectionState
	ConnectorId             apid.ID
	ConnectorVersion        uint64
	Labels                  Labels
	Annotations             Annotations
	EncryptedConfiguration  *encfield.EncryptedField
	EncryptedAt             *time.Time
	SetupStep               *cschema.SetupStep
	SetupError              *string
	CreatedAt               time.Time
	UpdatedAt               time.Time
	DeletedAt               *time.Time
}

func (c *Connection) cols() []string {
	return []string{
		"id",
		"namespace",
		"state",
		"connector_id",
		"connector_version",
		"labels",
		"annotations",
		"encrypted_configuration",
		"encrypted_at",
		"setup_step",
		"setup_error",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

func (c *Connection) fields() []any {
	return []any{
		&c.Id,
		&c.Namespace,
		&c.State,
		&c.ConnectorId,
		&c.ConnectorVersion,
		&c.Labels,
		&c.Annotations,
		&c.EncryptedConfiguration,
		&c.EncryptedAt,
		&c.SetupStep,
		&c.SetupError,
		&c.CreatedAt,
		&c.UpdatedAt,
		&c.DeletedAt,
	}
}

func (c *Connection) values() []any {
	return []any{
		c.Id,
		c.Namespace,
		c.State,
		c.ConnectorId,
		c.ConnectorVersion,
		c.Labels,
		c.Annotations,
		c.EncryptedConfiguration,
		c.EncryptedAt,
		c.SetupStep,
		c.SetupError,
		c.CreatedAt,
		c.UpdatedAt,
		c.DeletedAt,
	}
}

func (c *Connection) GetId() apid.ID {
	return c.Id
}

func (c *Connection) GetConnectorId() apid.ID {
	return c.ConnectorId
}

func (c *Connection) GetConnectorVersion() uint64 {
	return c.ConnectorVersion
}

func (c *Connection) GetNamespace() string {
	return c.Namespace
}

func (c *Connection) GetLabels() map[string]string {
	return c.Labels
}

func (c *Connection) GetAnnotations() map[string]string {
	return c.Annotations
}

func (c *Connection) Validate() error {
	result := &multierror.Error{}

	if c.Id == apid.Nil {
		result = multierror.Append(result, errors.New("connection id is required"))
	}

	if err := c.Id.ValidatePrefix(apid.PrefixConnection); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connection id: %w", err))
	}

	if err := ValidateNamespacePath(c.Namespace); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connection namespace path: %w", err))
	}

	if !IsValidConnectionState(c.State) {
		result = multierror.Append(result, errors.New("invalid connection state"))
	}

	if c.ConnectorId == apid.Nil {
		result = multierror.Append(result, errors.New("connection connector id is required"))
	}

	if err := c.ConnectorId.ValidatePrefix(apid.PrefixConnectorVersion); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connection connector id: %w", err))
	}

	if c.ConnectorVersion == 0 {
		result = multierror.Append(result, errors.New("connection connector version is required"))
	}

	if err := c.Labels.Validate(); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connection labels: %w", err))
	}

	if err := c.Annotations.Validate(); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connection annotations: %w", err))
	}

	return result.ErrorOrNil()
}

func (s *service) CreateConnection(ctx context.Context, c *Connection) error {
	if c == nil {
		return errors.New("connection is required")
	}

	if err := c.Validate(); err != nil {
		return err
	}

	cpy := *c
	now := apctx.GetClock(ctx).Now()
	cpy.CreatedAt = now
	cpy.UpdatedAt = now

	result, err := s.sq.
		Insert(ConnectionsTable).
		Columns(cpy.cols()...).
		Values(cpy.values()...).
		RunWith(s.db).
		Exec()
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		return errors.New("failed to create connection; no rows inserted")
	}

	return nil
}

func (s *service) GetConnection(ctx context.Context, id apid.ID) (*Connection, error) {
	var result Connection
	err := s.sq.
		Select(result.cols()...).
		From(ConnectionsTable).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		QueryRow().
		Scan(result.fields()...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &result, nil
}

func (s *service) DeleteConnection(ctx context.Context, id apid.ID) error {
	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(ConnectionsTable).
		Set("updated_at", now).
		Set("deleted_at", now).
		Where(sq.Eq{"id": id}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to soft delete connection: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to soft delete connection: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	if affected > 1 {
		return errors.New("multiple connections were soft deleted")
	}

	return nil
}

func (s *service) SetConnectionState(ctx context.Context, id apid.ID, state ConnectionState) error {
	if id == apid.Nil {
		return errors.New("connection id is required")
	}

	if !IsValidConnectionState(state) {
		return errors.New("invalid connection state")
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(ConnectionsTable).
		Set("updated_at", now).
		Set("state", state).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to soft delete connection: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to soft delete connection: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	if affected > 1 {
		return fmt.Errorf("multiple connections had state updated: %w", ErrViolation)
	}

	return nil
}

func (s *service) SetConnectionSetupStep(ctx context.Context, id apid.ID, setupStep *cschema.SetupStep) error {
	if id == apid.Nil {
		return errors.New("connection id is required")
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(ConnectionsTable).
		Set("updated_at", now).
		Set("setup_step", setupStep).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to update connection setup step: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update connection setup step: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	if affected > 1 {
		return fmt.Errorf("multiple connections had setup step updated: %w", ErrViolation)
	}

	return nil
}

func (s *service) SetConnectionSetupError(ctx context.Context, id apid.ID, setupError *string) error {
	if id == apid.Nil {
		return errors.New("connection id is required")
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(ConnectionsTable).
		Set("updated_at", now).
		Set("setup_error", setupError).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to update connection setup error: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update connection setup error: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	if affected > 1 {
		return fmt.Errorf("multiple connections had setup error updated: %w", ErrViolation)
	}

	return nil
}

func (s *service) SetConnectionEncryptedConfiguration(ctx context.Context, id apid.ID, encryptedConfig *encfield.EncryptedField) error {
	if id == apid.Nil {
		return errors.New("connection id is required")
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(ConnectionsTable).
		Set("updated_at", now).
		Set("encrypted_configuration", encryptedConfig).
		Set("encrypted_at", &now).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to update connection configuration: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update connection configuration: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	if affected > 1 {
		return fmt.Errorf("multiple connections had configuration updated: %w", ErrViolation)
	}

	return nil
}

type ConnectionOrderByField string

const (
	ConnectionOrderById        ConnectionOrderByField = "id"
	ConnectionOrderByNamespace ConnectionOrderByField = "namespace"
	ConnectionOrderByState     ConnectionOrderByField = "state"
	ConnectionOrderByCreatedAt ConnectionOrderByField = "created_at"
	ConnectionOrderByUpdatedAt ConnectionOrderByField = "updated_at"
)

func IsValidConnectionOrderByField[T string | ConnectionOrderByField](field T) bool {
	switch ConnectionOrderByField(field) {
	case ConnectionOrderById,
		ConnectionOrderByNamespace,
		ConnectionOrderByState,
		ConnectionOrderByCreatedAt,
		ConnectionOrderByUpdatedAt:
		return true
	default:
		return false
	}
}

type ListConnectionsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Connection]
	Enumerate(context.Context, func(pagination.PageResult[Connection]) (keepGoing bool, err error)) error
}

type ListConnectionsBuilder interface {
	ListConnectionsExecutor
	Limit(int32) ListConnectionsBuilder
	ForState(ConnectionState) ListConnectionsBuilder
	ForStates([]ConnectionState) ListConnectionsBuilder
	ForNamespaceMatcher(matcher string) ListConnectionsBuilder
	ForNamespaceMatchers(matchers []string) ListConnectionsBuilder
	OrderBy(ConnectionOrderByField, pagination.OrderBy) ListConnectionsBuilder
	IncludeDeleted() ListConnectionsBuilder
	WithDeletedHandling(DeletedHandling) ListConnectionsBuilder
	ForLabelSelector(selector string) ListConnectionsBuilder
	WithSetupStepNotNull() ListConnectionsBuilder
	UpdatedBefore(t time.Time) ListConnectionsBuilder
}

type listConnectionsFilters struct {
	s                    *service                `json:"-"`
	LimitVal             uint64                  `json:"limit"`
	Offset               uint64                  `json:"offset"`
	StatesVal            []ConnectionState       `json:"states,omitempty"`
	NamespaceMatchers    []string                `json:"namespace_matchers,omitempty"`
	OrderByFieldVal      *ConnectionOrderByField `json:"order_by_field"`
	OrderByVal           *pagination.OrderBy     `json:"order_by"`
	IncludeDeletedVal    bool                    `json:"include_deleted,omitempty"`
	LabelSelectorVal     *string                 `json:"label_selector,omitempty"`
	SetupStepNotNullVal  bool                    `json:"setup_step_not_null,omitempty"`
	UpdatedBeforeVal     *time.Time              `json:"updated_before,omitempty"`
	Errors               *multierror.Error       `json:"-"`
}

func (l *listConnectionsFilters) addError(e error) ListConnectionsBuilder {
	l.Errors = multierror.Append(l.Errors, e)
	return l
}

func (l *listConnectionsFilters) Limit(limit int32) ListConnectionsBuilder {
	l.LimitVal = uint64(limit)
	return l
}

func (l *listConnectionsFilters) ForState(state ConnectionState) ListConnectionsBuilder {
	return l.ForStates([]ConnectionState{state})
}

func (l *listConnectionsFilters) ForNamespaceMatcher(matcher string) ListConnectionsBuilder {
	if err := ValidateNamespaceMatcher(matcher); err != nil {
		return l.addError(err)
	} else {
		l.NamespaceMatchers = []string{matcher}
	}

	return l
}

func (l *listConnectionsFilters) ForNamespaceMatchers(matchers []string) ListConnectionsBuilder {
	for _, matcher := range matchers {
		if err := ValidateNamespaceMatcher(matcher); err != nil {
			return l.addError(err)
		}
	}
	l.NamespaceMatchers = matchers
	return l
}

func (l *listConnectionsFilters) ForStates(states []ConnectionState) ListConnectionsBuilder {
	l.StatesVal = states
	return l
}

func (l *listConnectionsFilters) OrderBy(field ConnectionOrderByField, by pagination.OrderBy) ListConnectionsBuilder {
	if IsValidConnectionOrderByField(field) {
		l.OrderByFieldVal = &field
		l.OrderByVal = &by
	} else {
		return l.addError(fmt.Errorf("invalid order by field: %v", field))
	}
	return l
}

func (l *listConnectionsFilters) IncludeDeleted() ListConnectionsBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listConnectionsFilters) WithDeletedHandling(h DeletedHandling) ListConnectionsBuilder {
	if h == DeletedHandlingExclude {
		l.IncludeDeletedVal = false
	} else {
		l.IncludeDeletedVal = true
	}
	return l
}

func (l *listConnectionsFilters) ForLabelSelector(selector string) ListConnectionsBuilder {
	l.LabelSelectorVal = &selector
	return l
}

func (l *listConnectionsFilters) WithSetupStepNotNull() ListConnectionsBuilder {
	l.SetupStepNotNullVal = true
	return l
}

func (l *listConnectionsFilters) UpdatedBefore(t time.Time) ListConnectionsBuilder {
	l.UpdatedBeforeVal = &t
	return l
}

func (l *listConnectionsFilters) FromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error) {
	s := l.s
	parsed, err := pagination.ParseCursor[listConnectionsFilters](ctx, s.cursorEncryptor, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.s = s

	return l, nil
}

func (l *listConnectionsFilters) applyRestrictions(ctx context.Context) sq.SelectBuilder {
	q := l.s.sq.
		Select(util.ToPtr(Connection{}).cols()...).
		From(ConnectionsTable)

	if l.LabelSelectorVal != nil {
		selector, err := ParseLabelSelector(*l.LabelSelectorVal)
		if err != nil {
			l.addError(err)
		} else {
			q = selector.ApplyToSqlBuilderWithProvider(q, "labels", l.s.cfg.GetProvider())
		}
	}

	if !l.IncludeDeletedVal {
		q = q.Where(sq.Eq{"deleted_at": nil})
	}

	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	// Always limit to one more than limit to check if there are more records
	q = q.Limit(l.LimitVal + 1).Offset(l.Offset)

	if len(l.StatesVal) > 0 {
		q = q.Where(sq.Eq{"state": l.StatesVal})
	}

	if len(l.NamespaceMatchers) > 0 {
		q = restrictToNamespaceMatchers(q, "namespace", l.NamespaceMatchers)
	}

	if l.SetupStepNotNullVal {
		q = q.Where(sq.NotEq{"setup_step": nil})
	}

	if l.UpdatedBeforeVal != nil {
		q = q.Where(sq.Lt{"updated_at": *l.UpdatedBeforeVal})
	}

	if l.OrderByFieldVal != nil {
		q = q.OrderBy(fmt.Sprintf("%s %s", string(*l.OrderByFieldVal), l.OrderByVal.String()))
	}

	return q
}

func (l *listConnectionsFilters) fetchPage(ctx context.Context) pagination.PageResult[Connection] {
	var err error

	if err = l.Errors.ErrorOrNil(); err != nil {
		return pagination.PageResult[Connection]{Error: err}
	}

	rows, err := l.applyRestrictions(ctx).
		RunWith(l.s.db).
		Query()
	if err != nil {
		return pagination.PageResult[Connection]{Error: err}
	}
	defer rows.Close()

	var results []Connection
	for rows.Next() {
		var r Connection
		err := rows.Scan(r.fields()...)
		if err != nil {
			return pagination.PageResult[Connection]{Error: err}
		}
		results = append(results, r)
	}

	l.Offset = l.Offset + uint64(len(results)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := uint64(len(results)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.cursorEncryptor, l)
		if err != nil {
			return pagination.PageResult[Connection]{Error: err}
		}
	}

	return pagination.PageResult[Connection]{
		HasMore: hasMore,
		Results: results[:util.MinUint64(l.LimitVal, uint64(len(results)))],
		Cursor:  cursor,
	}
}

func (l *listConnectionsFilters) FetchPage(ctx context.Context) pagination.PageResult[Connection] {
	return l.fetchPage(ctx)
}

func (l *listConnectionsFilters) Enumerate(ctx context.Context, callback func(pagination.PageResult[Connection]) (keepGoing bool, err error)) error {
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

// UpdateConnectionLabels replaces all labels on a connection within a single transaction.
// Unlike PutConnectionLabels, this does a full replacement rather than a merge.
func (s *service) UpdateConnectionLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Connection, error) {
	if id == apid.Nil {
		return nil, errors.New("connection id is required")
	}

	if labels != nil {
		if err := ValidateLabels(labels); err != nil {
			return nil, fmt.Errorf("invalid labels: %w", err)
		}
	}

	var result *Connection

	err := s.transaction(func(tx *sql.Tx) error {
		// Get the current connection within the transaction
		var conn Connection
		err := s.sq.
			Select(conn.cols()...).
			From(ConnectionsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(conn.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		now, err := s.updateLabelsInTableTx(ctx, tx, ConnectionsTable, sq.Eq{"id": id, "deleted_at": nil}, Labels(labels))
		if err != nil {
			return err
		}

		conn.Labels = labels
		conn.UpdatedAt = now
		result = &conn
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// PutConnectionLabels adds or updates the specified labels on a connection within a single transaction.
// Existing labels not in the provided map are preserved.
func (s *service) PutConnectionLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Connection, error) {
	if id == apid.Nil {
		return nil, errors.New("connection id is required")
	}

	if len(labels) == 0 {
		return s.GetConnection(ctx, id)
	}

	if err := ValidateLabels(labels); err != nil {
		return nil, fmt.Errorf("invalid labels: %w", err)
	}

	var result *Connection

	err := s.transaction(func(tx *sql.Tx) error {
		// Get the current connection within the transaction
		var conn Connection
		err := s.sq.
			Select(conn.cols()...).
			From(ConnectionsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(conn.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		mergedLabels, now, err := s.putLabelsInTableTx(ctx, tx, ConnectionsTable, sq.Eq{"id": id, "deleted_at": nil}, labels)
		if err != nil {
			return err
		}

		conn.Labels = mergedLabels
		conn.UpdatedAt = now
		result = &conn
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// UpdateConnectionAnnotations replaces all annotations on a connection within a single transaction.
func (s *service) UpdateConnectionAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Connection, error) {
	if id == apid.Nil {
		return nil, errors.New("connection id is required")
	}

	if annotations != nil {
		if err := ValidateAnnotations(annotations); err != nil {
			return nil, fmt.Errorf("invalid annotations: %w", err)
		}
	}

	var result *Connection

	err := s.transaction(func(tx *sql.Tx) error {
		var conn Connection
		err := s.sq.
			Select(conn.cols()...).
			From(ConnectionsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(conn.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		now, err := s.updateAnnotationsInTableTx(ctx, tx, ConnectionsTable, sq.Eq{"id": id, "deleted_at": nil}, Annotations(annotations))
		if err != nil {
			return err
		}

		conn.Annotations = annotations
		conn.UpdatedAt = now
		result = &conn
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// PutConnectionAnnotations adds or updates the specified annotations on a connection within a single transaction.
func (s *service) PutConnectionAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Connection, error) {
	if id == apid.Nil {
		return nil, errors.New("connection id is required")
	}

	if len(annotations) == 0 {
		return s.GetConnection(ctx, id)
	}

	if err := ValidateAnnotations(annotations); err != nil {
		return nil, fmt.Errorf("invalid annotations: %w", err)
	}

	var result *Connection

	err := s.transaction(func(tx *sql.Tx) error {
		var conn Connection
		err := s.sq.
			Select(conn.cols()...).
			From(ConnectionsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(conn.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		mergedAnnotations, now, err := s.putAnnotationsInTableTx(ctx, tx, ConnectionsTable, sq.Eq{"id": id, "deleted_at": nil}, annotations)
		if err != nil {
			return err
		}

		conn.Annotations = mergedAnnotations
		conn.UpdatedAt = now
		result = &conn
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteConnectionAnnotations removes the specified annotation keys from a connection within a single transaction.
func (s *service) DeleteConnectionAnnotations(ctx context.Context, id apid.ID, keys []string) (*Connection, error) {
	if id == apid.Nil {
		return nil, errors.New("connection id is required")
	}

	if len(keys) == 0 {
		return s.GetConnection(ctx, id)
	}

	var result *Connection

	err := s.transaction(func(tx *sql.Tx) error {
		var conn Connection
		err := s.sq.
			Select(conn.cols()...).
			From(ConnectionsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(conn.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		remainingAnnotations, now, err := s.deleteAnnotationsInTableTx(ctx, tx, ConnectionsTable, sq.Eq{"id": id, "deleted_at": nil}, keys)
		if err != nil {
			return err
		}

		conn.Annotations = remainingAnnotations
		conn.UpdatedAt = now
		result = &conn
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteConnectionLabels removes the specified label keys from a connection within a single transaction.
// Keys that don't exist are ignored.
func (s *service) DeleteConnectionLabels(ctx context.Context, id apid.ID, keys []string) (*Connection, error) {
	if id == apid.Nil {
		return nil, errors.New("connection id is required")
	}

	if len(keys) == 0 {
		return s.GetConnection(ctx, id)
	}

	var result *Connection

	err := s.transaction(func(tx *sql.Tx) error {
		// Get the current connection within the transaction
		var conn Connection
		err := s.sq.
			Select(conn.cols()...).
			From(ConnectionsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(conn.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		remainingLabels, now, err := s.deleteLabelsInTableTx(ctx, tx, ConnectionsTable, sq.Eq{"id": id, "deleted_at": nil}, keys)
		if err != nil {
			return err
		}

		conn.Labels = remainingLabels
		conn.UpdatedAt = now
		result = &conn
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *service) ListConnectionsBuilder() ListConnectionsBuilder {
	return &listConnectionsFilters{
		s:        s,
		LimitVal: 100,
	}
}

func (s *service) ListConnectionsFromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error) {
	b := &listConnectionsFilters{
		s:        s,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}

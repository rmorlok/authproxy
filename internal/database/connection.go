package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

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
	Id               uuid.UUID
	Namespace        string
	State            ConnectionState
	ConnectorId      uuid.UUID
	ConnectorVersion uint64
	Labels           Labels
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

func (c *Connection) cols() []string {
	return []string{
		"id",
		"namespace",
		"state",
		"connector_id",
		"connector_version",
		"labels",
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
		c.CreatedAt,
		c.UpdatedAt,
		c.DeletedAt,
	}
}

func (c *Connection) GetId() uuid.UUID {
	return c.Id
}

func (c *Connection) GetConnectorId() uuid.UUID {
	return c.ConnectorId
}

func (c *Connection) GetConnectorVersion() uint64 {
	return c.ConnectorVersion
}

func (c *Connection) GetNamespace() string {
	return c.Namespace
}

func (c *Connection) Validate() error {
	result := &multierror.Error{}

	if c.Id == uuid.Nil {
		result = multierror.Append(result, errors.New("connection id is required"))
	}

	if err := ValidateNamespacePath(c.Namespace); err != nil {
		result = multierror.Append(result, errors.Wrap(err, "invalid connection namespace path"))
	}

	if !IsValidConnectionState(c.State) {
		result = multierror.Append(result, errors.New("invalid connection state"))
	}

	if c.ConnectorId == uuid.Nil {
		result = multierror.Append(result, errors.New("connection connector id is required"))
	}

	if c.ConnectorVersion == 0 {
		result = multierror.Append(result, errors.New("connection connector version is required"))
	}

	if err := c.Labels.Validate(); err != nil {
		result = multierror.Append(result, errors.Wrap(err, "invalid connection labels"))
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

func (s *service) GetConnection(ctx context.Context, id uuid.UUID) (*Connection, error) {
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

func (s *service) DeleteConnection(ctx context.Context, id uuid.UUID) error {
	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(ConnectionsTable).
		Set("updated_at", now).
		Set("deleted_at", now).
		Where(sq.Eq{"id": id}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return errors.Wrap(err, "failed to soft delete connection")
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to soft delete connection")
	}

	if affected == 0 {
		return ErrNotFound
	}

	if affected > 1 {
		return errors.New("multiple connections were soft deleted")
	}

	return nil
}

func (s *service) SetConnectionState(ctx context.Context, id uuid.UUID, state ConnectionState) error {
	if id == uuid.Nil {
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
		Where(sq.Eq{"id": id}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return errors.Wrap(err, "failed to soft delete connection")
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to soft delete connection")
	}

	if affected == 0 {
		return ErrNotFound
	}

	if affected > 1 {
		return errors.Wrap(ErrViolation, "multiple connections had state updated")
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
}

type listConnectionsFilters struct {
	s                 *service                `json:"-"`
	LimitVal          uint64                  `json:"limit"`
	Offset            uint64                  `json:"offset"`
	StatesVal         []ConnectionState       `json:"states,omitempty"`
	NamespaceMatchers []string                `json:"namespace_matchers,omitempty"`
	OrderByFieldVal   *ConnectionOrderByField `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy     `json:"order_by"`
	IncludeDeletedVal bool                    `json:"include_deleted,omitempty"`
	LabelSelectorVal  *string                 `json:"label_selector,omitempty"`
	Errors            *multierror.Error       `json:"-"`
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

func (l *listConnectionsFilters) FromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error) {
	s := l.s
	parsed, err := pagination.ParseCursor[listConnectionsFilters](ctx, s.secretKey, cursor)

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
			q = selector.ApplyToSqlBuilder(q, "labels")
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
		cursor, err = pagination.MakeCursor(ctx, l.s.secretKey, l)
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
func (s *service) UpdateConnectionLabels(ctx context.Context, id uuid.UUID, labels map[string]string) (*Connection, error) {
	if id == uuid.Nil {
		return nil, errors.New("connection id is required")
	}

	if labels != nil {
		if err := ValidateLabels(labels); err != nil {
			return nil, errors.Wrap(err, "invalid labels")
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
func (s *service) PutConnectionLabels(ctx context.Context, id uuid.UUID, labels map[string]string) (*Connection, error) {
	if id == uuid.Nil {
		return nil, errors.New("connection id is required")
	}

	if len(labels) == 0 {
		return s.GetConnection(ctx, id)
	}

	if err := ValidateLabels(labels); err != nil {
		return nil, errors.Wrap(err, "invalid labels")
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

// DeleteConnectionLabels removes the specified label keys from a connection within a single transaction.
// Keys that don't exist are ignored.
func (s *service) DeleteConnectionLabels(ctx context.Context, id uuid.UUID, keys []string) (*Connection, error) {
	if id == uuid.Nil {
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

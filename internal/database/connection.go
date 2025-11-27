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
	"gorm.io/gorm"
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
	ID               uuid.UUID
	Namespace        string
	State            ConnectionState
	ConnectorId      uuid.UUID
	ConnectorVersion uint64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt
}

func (c *Connection) cols() []string {
	return []string{
		"id",
		"namespace",
		"state",
		"connector_id",
		"connector_version",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

func (c *Connection) fields() []any {
	return []any{
		&c.ID,
		&c.Namespace,
		&c.State,
		&c.ConnectorId,
		&c.ConnectorVersion,
		&c.CreatedAt,
		&c.UpdatedAt,
		&c.DeletedAt,
	}
}

func (c *Connection) values() []any {
	return []any{
		c.ID,
		c.Namespace,
		c.State,
		c.ConnectorId,
		c.ConnectorVersion,
		c.CreatedAt,
		c.UpdatedAt,
		c.DeletedAt,
	}
}

func (c *Connection) GetID() uuid.UUID {
	return c.ID
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

	if c.ID == uuid.Nil {
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
	ConnectionOrderByState     ConnectionOrderByField = "state"
	ConnectionOrderByCreatedAt ConnectionOrderByField = "created_at"
	ConnectionOrderByUpdatedAt ConnectionOrderByField = "updated_at"
)

func IsValidConnectionOrderByField[T string | ConnectionOrderByField](field T) bool {
	switch ConnectionOrderByField(field) {
	case ConnectionOrderById:
		return true
	case ConnectionOrderByState:
		return true
	case ConnectionOrderByCreatedAt:
		return true
	case ConnectionOrderByUpdatedAt:
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
	OrderBy(ConnectionOrderByField, pagination.OrderBy) ListConnectionsBuilder
	IncludeDeleted() ListConnectionsBuilder
	WithDeletedHandling(DeletedHandling) ListConnectionsBuilder
}

type listConnectionsFilters struct {
	s                 *service                `json:"-"`
	LimitVal          uint64                  `json:"limit"`
	Offset            uint64                  `json:"offset"`
	StatesVal         []ConnectionState       `json:"states,omitempty"`
	OrderByFieldVal   *ConnectionOrderByField `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy     `json:"order_by"`
	IncludeDeletedVal bool                    `json:"include_deleted,omitempty"`
}

func (l *listConnectionsFilters) Limit(limit int32) ListConnectionsBuilder {
	l.LimitVal = uint64(limit)
	return l
}

func (l *listConnectionsFilters) ForState(state ConnectionState) ListConnectionsBuilder {
	return l.ForStates([]ConnectionState{state})
}

func (l *listConnectionsFilters) ForStates(states []ConnectionState) ListConnectionsBuilder {
	l.StatesVal = states
	return l
}

func (l *listConnectionsFilters) OrderBy(field ConnectionOrderByField, by pagination.OrderBy) ListConnectionsBuilder {
	if IsValidConnectionOrderByField(field) {
		l.OrderByFieldVal = &field
		l.OrderByVal = &by
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

	if l.OrderByFieldVal != nil {
		q = q.OrderBy(fmt.Sprintf("%s %s", string(*l.OrderByFieldVal), l.OrderByVal.String()))
	}

	return q
}

func (l *listConnectionsFilters) fetchPage(ctx context.Context) pagination.PageResult[Connection] {
	var err error

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

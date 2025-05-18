package database

import (
	"context"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/util"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type ConnectionState string

const (
	ConnectionStateCreated  ConnectionState = "created"
	ConnectionStateReady    ConnectionState = "ready"
	ConnectionStateDisabled ConnectionState = "disabled"
)

type Connection struct {
	ID          uuid.UUID       `gorm:"column:id;primaryKey"`
	State       ConnectionState `gorm:"column:state"`
	ConnectorId uuid.UUID       `gorm:"column:connector_id"`
	CreatedAt   time.Time       `gorm:"column:created_at"`
	UpdatedAt   time.Time       `gorm:"column:updated_at"`
	DeletedAt   gorm.DeletedAt  `gorm:"column:deleted_at;index"`
}

func (db *gormDB) CreateConnection(ctx context.Context, c *Connection) error {
	sess := db.session(ctx)
	result := sess.Create(&c)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return nil
	}

	return nil
}

func (db *gormDB) GetConnection(ctx context.Context, id uuid.UUID) (*Connection, error) {
	sess := db.session(ctx)

	var c Connection
	result := sess.First(&c, id)
	if result.Error != nil {
		if errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &c, nil
}

type ConnectionOrderByField string

const (
	ConnectionOrderByCreatedAt ConnectionOrderByField = "created_at"
)

type ListConnectionsExecutor interface {
	FetchPage(context.Context) PageResult[Connection]
	Enumerate(context.Context, func(PageResult[Connection]) (keepGoing bool, err error)) error
}

type ListConnectionsBuilder interface {
	ListConnectionsExecutor
	Limit(int32) ListConnectionsBuilder
	ForConnectionState(ConnectionState) ListConnectionsBuilder
	OrderBy(ConnectionOrderByField, OrderBy) ListConnectionsBuilder
	IncludeDeleted() ListConnectionsBuilder
}

type listConnectionsFilters struct {
	db                *gormDB                 `json:"-"`
	LimitVal          int32                   `json:"limit"`
	Offset            int32                   `json:"offset"`
	StatesVal         []ConnectionState       `json:"states,omitempty"`
	OrderByFieldVal   *ConnectionOrderByField `json:"order_by_field"`
	OrderByVal        *OrderBy                `json:"order_by"`
	IncludeDeletedVal bool                    `json:"include_deleted,omitempty"`
}

func (l *listConnectionsFilters) Limit(limit int32) ListConnectionsBuilder {
	l.LimitVal = limit
	return l
}

func (l *listConnectionsFilters) ForConnectionState(state ConnectionState) ListConnectionsBuilder {
	l.StatesVal = []ConnectionState{state}
	return l
}

func (l *listConnectionsFilters) OrderBy(field ConnectionOrderByField, by OrderBy) ListConnectionsBuilder {
	l.OrderByFieldVal = &field
	l.OrderByVal = &by
	return l
}

func (l *listConnectionsFilters) IncludeDeleted() ListConnectionsBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listConnectionsFilters) FromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error) {
	db := l.db
	parsed, err := parseCursor[listConnectionsFilters](ctx, db.secretKey, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.db = db

	return l, nil
}

func (l *listConnectionsFilters) applyRestrictions(ctx context.Context) *gorm.DB {
	q := l.db.session(ctx)

	if l.IncludeDeletedVal {
		q = q.Unscoped()
	}

	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	// Always limit to one more than limit to check if there are more records
	q = q.Limit(int(l.LimitVal + 1)).Offset(int(l.Offset))

	if len(l.StatesVal) > 0 {
		q = q.Where("state IN ?", l.StatesVal)
	}

	if l.OrderByFieldVal != nil {
		q.Order(clause.OrderByColumn{Column: clause.Column{Name: string(*l.OrderByFieldVal)}, Desc: true})
	}

	return q
}

func (l *listConnectionsFilters) fetchPage(ctx context.Context) PageResult[Connection] {
	var err error
	var connections []Connection
	result := l.applyRestrictions(ctx).Find(&connections)
	if result.Error != nil {
		return PageResult[Connection]{Error: result.Error}
	}

	l.Offset = l.Offset + int32(len(connections)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := int32(len(connections)) > l.LimitVal
	if hasMore {
		cursor, err = makeCursor(ctx, l.db.secretKey, l)
		if err != nil {
			return PageResult[Connection]{Error: err}
		}
	}

	return PageResult[Connection]{
		HasMore: hasMore,
		Results: connections[:util.MinInt32(l.LimitVal, int32(len(connections)))],
		Cursor:  cursor,
	}
}

func (l *listConnectionsFilters) FetchPage(ctx context.Context) PageResult[Connection] {
	return l.fetchPage(ctx)
}

func (l *listConnectionsFilters) Enumerate(ctx context.Context, callback func(PageResult[Connection]) (keepGoing bool, err error)) error {
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

func (db *gormDB) ListConnectionsBuilder() ListConnectionsBuilder {
	return &listConnectionsFilters{
		db:       db,
		LimitVal: 100,
	}
}

func (db *gormDB) ListConnectionsFromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error) {
	b := &listConnectionsFilters{
		db:       db,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}

package database

import (
	"context"
	"fmt"
	"regexp"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/config/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"gorm.io/gorm"
)

// Namespace is the grouping of resources within AuthProxy.
type Namespace struct {
	Path      string         `gorm:"column:path;primarykey"`
	State     NamespaceState `gorm:"column:state"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

var validPathRegex = regexp.MustCompile(`^root(?:/[a-zA-Z0-9_]+[a-zA-Z0-9_\-]*)*$`)

var (
	ValidateNamespacePath        = common.ValidateNamespacePath
	SplitNamespacePathToPrefixes = common.SplitNamespacePathToPrefixes
)

type NamespaceState string

const (
	NamespaceStateActive     NamespaceState = "active"
	NamespaceStateDestroying NamespaceState = "destroying"
	NamespaceStateDestroyed  NamespaceState = "destroyed"
)

type NamespaceOrderByField string

const (
	NamespaceOrderByPath      NamespaceOrderByField = "path"
	NamespaceOrderByState     NamespaceOrderByField = "state"
	NamespaceOrderByCreatedAt NamespaceOrderByField = "created_at"
	NamespaceOrderByUpdatedAt NamespaceOrderByField = "updated_at"
)

func IsValidNamespaceOrderByField[T string | NamespaceOrderByField](field T) bool {
	switch NamespaceOrderByField(field) {
	case NamespaceOrderByPath,
		NamespaceOrderByState,
		NamespaceOrderByCreatedAt,
		NamespaceOrderByUpdatedAt:
		return true
	default:
		return false
	}
}

func advanceNamespaceState(cur NamespaceState, next NamespaceState) NamespaceState {
	switch cur {
	case NamespaceState(""):
		return next
	case NamespaceStateActive:
		return next
	case NamespaceStateDestroying:
		if next == NamespaceStateDestroyed {
			return next
		}
		return cur
	case NamespaceStateDestroyed:
		return cur
	}

	return next
}

func (s *service) CreateNamespace(ctx context.Context, ns *Namespace) error {
	err := ValidateNamespacePath(ns.Path)
	if err != nil {
		return err
	}

	return s.gorm.Transaction(func(tx *gorm.DB) error {
		prefixes := SplitNamespacePathToPrefixes(ns.Path)

		// Start out with the specified state or default to active
		state := advanceNamespaceState(ns.State, NamespaceStateActive)

		for i := 0; i < len(prefixes)-1; i++ {
			var parent Namespace
			result := tx.First(&parent, "path = ? ", prefixes[i])
			if result.Error != nil {
				if errors.As(result.Error, &gorm.ErrRecordNotFound) {
					return errors.Errorf("cannot create namespace '%s' because parent namespace '%s' does not exist or is deleted", ns.Path, prefixes[i])
				}
				return result.Error
			}

			state = advanceNamespaceState(state, parent.State)
		}

		var existing Namespace
		result := tx.First(&existing, "path = ? ", ns.Path)
		if result.Error != nil && !errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return result.Error
		} else if result.Error == nil {
			return errors.Errorf("cannot create namespace '%s' because it already exists", ns.Path)
		}

		// Update the state so that we are always bound by parent namespace state
		ns.State = state

		result = tx.Create(ns)
		if result.Error != nil {
			return result.Error
		}

		return nil
	})
}

func (s *service) GetNamespace(ctx context.Context, path string) (*Namespace, error) {
	sess := s.session(ctx)

	var ns Namespace
	result := sess.First(&ns, "path = ? ", path)
	if result.Error != nil {
		if errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}

	return &ns, nil
}

func (s *service) DeleteNamespace(ctx context.Context, path string) error {
	sess := s.session(ctx)
	result := sess.Delete(&Namespace{}, "path = ?", path)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *service) SetNamespaceState(ctx context.Context, path string, state NamespaceState) error {
	sqlDb, err := s.gorm.DB()
	if err != nil {
		return err
	}

	sqb := sq.StatementBuilder.RunWith(sqlDb)

	result, err := sqb.
		Update("namespaces").
		Set("state", state).
		Set("updated_at", apctx.GetClock(ctx).Now()).
		Where("path = ?", path).
		ExecContext(ctx)

	if err != nil {
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if count == 0 {
		return ErrNotFound
	}

	if count > 1 {
		return ErrViolation
	}

	return nil
}

type ListNamespacesExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Namespace]
	Enumerate(context.Context, func(pagination.PageResult[Namespace]) (keepGoing bool, err error)) error
}

type ListNamespacesBuilder interface {
	ListNamespacesExecutor
	Limit(int32) ListNamespacesBuilder
	ForPathPrefix(path string) ListNamespacesBuilder
	ForState(NamespaceState) ListNamespacesBuilder
	OrderBy(NamespaceOrderByField, pagination.OrderBy) ListNamespacesBuilder
	IncludeDeleted() ListNamespacesBuilder
}

type listNamespacesFilters struct {
	db                *service               `json:"-"`
	LimitVal          int32                  `json:"limit"`
	Offset            int32                  `json:"offset"`
	StatesVal         []NamespaceState       `json:"states,omitempty"`
	PathPrefix        string                 `json:"path_prefix,omitempty"`
	OrderByFieldVal   *NamespaceOrderByField `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy    `json:"order_by"`
	IncludeDeletedVal bool                   `json:"include_deleted,omitempty"`
}

func (l *listNamespacesFilters) Limit(limit int32) ListNamespacesBuilder {
	l.LimitVal = limit
	return l
}

func (l *listNamespacesFilters) ForState(state NamespaceState) ListNamespacesBuilder {
	l.StatesVal = []NamespaceState{state}
	return l
}

func (l *listNamespacesFilters) ForPathPrefix(prefix string) ListNamespacesBuilder {
	l.PathPrefix = prefix
	return l
}

func (l *listNamespacesFilters) OrderBy(field NamespaceOrderByField, by pagination.OrderBy) ListNamespacesBuilder {
	l.OrderByFieldVal = &field
	l.OrderByVal = &by
	return l
}

func (l *listNamespacesFilters) IncludeDeleted() ListNamespacesBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listNamespacesFilters) FromCursor(ctx context.Context, cursor string) (ListNamespacesExecutor, error) {
	db := l.db
	parsed, err := pagination.ParseCursor[listNamespacesFilters](ctx, db.secretKey, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.db = db

	return l, nil
}

func (l *listNamespacesFilters) fetchPage(ctx context.Context) pagination.PageResult[Namespace] {

	q := l.db.session(ctx)

	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	query := sq.Select(`
n.path,
n.state,
n.created_at,
n.updated_at,
n.deleted_at`).
		From("namespaces n")

	if l.PathPrefix != "" {
		if len(l.PathPrefix) >= 2 && l.PathPrefix[len(l.PathPrefix)-1] == '/' {
			query = query.Where("(n.path = ? OR n.path LIKE ?)", l.PathPrefix[:len(l.PathPrefix)-2], l.PathPrefix+"%")
		} else {
			query = query.Where("(n.path = ? OR n.path LIKE ?)", l.PathPrefix, l.PathPrefix+"/%")
		}
	}

	if len(l.StatesVal) > 0 {
		query = query.Where("n.state IN ?", l.StatesVal)
	}

	if l.IncludeDeletedVal {
		q = q.Unscoped()
	} else {
		query = query.Where("n.deleted_at IS NULL")
	}

	// Always limit to one more than limit to check if there are more records
	query = query.Limit(uint64(l.LimitVal + 1)).Offset(uint64(l.Offset))

	if l.OrderByFieldVal != nil {
		query = query.OrderBy(fmt.Sprintf("%s %s", *l.OrderByFieldVal, l.OrderByVal.String()))
	}

	sql, args, err := query.ToSql()
	if err != nil {
		// SQL generation should be deterministic
		panic(errors.Errorf("failed to build query: %s", err))
	}

	rows, err := q.Raw(sql, args...).Rows()

	if err != nil {
		return pagination.PageResult[Namespace]{Error: err}
	}

	var connectors []Namespace
	for rows.Next() {
		var c Namespace

		// Scan all fields except States
		err := rows.Scan(
			&c.Path,
			&c.State,
			&c.CreatedAt,
			&c.UpdatedAt,
			&c.DeletedAt,
		)
		if err != nil {
			return pagination.PageResult[Namespace]{Error: err}
		}

		connectors = append(connectors, c)
	}

	l.Offset = l.Offset + int32(len(connectors)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := int32(len(connectors)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.db.secretKey, l)
		if err != nil {
			return pagination.PageResult[Namespace]{Error: err}
		}
	}

	return pagination.PageResult[Namespace]{
		HasMore: hasMore,
		Results: connectors[:util.MinInt32(l.LimitVal, int32(len(connectors)))],
		Cursor:  cursor,
	}
}

func (l *listNamespacesFilters) FetchPage(ctx context.Context) pagination.PageResult[Namespace] {
	return l.fetchPage(ctx)
}

func (l *listNamespacesFilters) Enumerate(ctx context.Context, callback func(pagination.PageResult[Namespace]) (keepGoing bool, err error)) error {
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

func (s *service) ListNamespacesBuilder() ListNamespacesBuilder {
	return &listNamespacesFilters{
		db:       s,
		LimitVal: 100,
	}
}

func (s *service) ListNamespacesFromCursor(ctx context.Context, cursor string) (ListNamespacesExecutor, error) {
	b := &listNamespacesFilters{
		db:       s,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}

package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/api_common"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const NamespacesTable = "namespaces"

// Namespace is the grouping of resources within AuthProxy.
type Namespace struct {
	Path      string
	depth     uint64
	State     NamespaceState
	Labels    Labels
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

func (ns *Namespace) GetNamespace() string {
	return ns.Path
}

func (ns *Namespace) cols() []string {
	return []string{
		"path",
		"depth",
		"state",
		"labels",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

func (ns *Namespace) fields() []any {
	return []any{
		&ns.Path,
		&ns.depth,
		&ns.State,
		&ns.Labels,
		&ns.CreatedAt,
		&ns.UpdatedAt,
		&ns.DeletedAt,
	}
}

func (ns *Namespace) values() []any {
	return []any{
		ns.Path,
		ns.depth,
		ns.State,
		ns.Labels,
		ns.CreatedAt,
		ns.UpdatedAt,
		ns.DeletedAt,
	}
}

func (ns *Namespace) normalize() {
	if ns.State == "" {
		ns.State = NamespaceStateActive
	}

	ns.depth = DepthOfNamespacePath(ns.Path)
}

func (ns *Namespace) Validate() error {
	result := &multierror.Error{}

	if err := ValidateNamespacePath(ns.Path); err != nil {
		result = multierror.Append(result, errors.Wrap(err, "invalid namespace path"))
	}

	if !IsValidNamespaceState(ns.State) {
		result = multierror.Append(result, errors.New("invalid namespace state"))
	}

	if err := ns.Labels.Validate(); err != nil {
		result = multierror.Append(result, errors.Wrap(err, "invalid namespace labels"))
	}

	return result.ErrorOrNil()
}

var (
	ValidateNamespacePath        = aschema.ValidateNamespacePath
	ValidateNamespaceMatcher     = aschema.ValidateNamespaceMatcher
	SplitNamespacePathToPrefixes = aschema.SplitNamespacePathToPrefixes
	DepthOfNamespacePath         = aschema.DepthOfNamespacePath
)

// restrictToNamespaceMatcher applies a restriction where the query must match the namespace matcher.
func restrictToNamespaceMatcher(q sq.SelectBuilder, field string, matcher string) sq.SelectBuilder {
	if strings.HasSuffix(matcher, ".**") {
		statedNamespace := matcher[:len(matcher)-3]
		return q.Where(sq.Or{
			sq.Eq{field: statedNamespace},
			sq.Like{field: statedNamespace + ".%"},
		})
	} else {
		return q.Where(sq.Eq{field: matcher})
	}
}

// namespaceMatcherCondition returns a squirrel condition for a single namespace matcher.
func namespaceMatcherCondition(field string, matcher string) sq.Sqlizer {
	if strings.HasSuffix(matcher, ".**") {
		statedNamespace := matcher[:len(matcher)-3]
		return sq.Or{
			sq.Eq{field: statedNamespace},
			sq.Like{field: statedNamespace + ".%"},
		}
	} else {
		return sq.Eq{field: matcher}
	}
}

// restrictToNamespaceMatchers applies a restriction where the query must match at least one of the namespace matchers
// (OR logic). If matchers is empty, no restriction is applied.
func restrictToNamespaceMatchers(q sq.SelectBuilder, field string, matchers []string) sq.SelectBuilder {
	if len(matchers) == 0 {
		return q
	}

	if len(matchers) == 1 {
		return restrictToNamespaceMatcher(q, field, matchers[0])
	}

	conditions := make([]sq.Sqlizer, 0, len(matchers))
	for _, matcher := range matchers {
		conditions = append(conditions, namespaceMatcherCondition(field, matcher))
	}

	return q.Where(sq.Or(conditions))
}

type NamespaceState string

const (
	NamespaceStateActive     NamespaceState = "active"
	NamespaceStateDestroying NamespaceState = "destroying"
	NamespaceStateDestroyed  NamespaceState = "destroyed"
)

func IsValidNamespaceState[T string | NamespaceState](state T) bool {
	switch NamespaceState(state) {
	case NamespaceStateActive, NamespaceStateDestroying, NamespaceStateDestroyed:
		return true
	default:
		return false
	}
}

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

func (s *service) getNamespaceByPath(ctx context.Context, tx sq.BaseRunner, path string) (*Namespace, error) {
	var result Namespace
	err := s.sq.
		Select(result.cols()...).
		From(NamespacesTable).
		Where(sq.Eq{
			"path":       path,
			"deleted_at": nil,
		}).
		RunWith(tx).QueryRow().
		Scan(result.fields()...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &result, nil
}

func (s *service) createNamespace(ctx context.Context, tx *sql.Tx, ns *Namespace) error {
	prefixes := SplitNamespacePathToPrefixes(ns.Path)
	state := ns.State

	for i := 0; i < len(prefixes)-1; i++ {
		parent, err := s.getNamespaceByPath(ctx, tx, prefixes[i])
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithPublicErrf("cannot create namespace '%s' because parent namespace '%s' does not exist or is deleted", ns.Path, prefixes[i]).
					Build()
			}
			return err
		}

		state = advanceNamespaceState(state, parent.State)
	}

	// Check if the namespace already exists
	_, err := s.getNamespaceByPath(ctx, tx, ns.Path)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	} else if err == nil {
		return errors.Errorf("cannot create namespace '%s' because it already exists", ns.Path)
	}

	// Update the state so that we are always bound by the parent namespace state
	ns.State = state

	now := apctx.GetClock(ctx).Now()
	ns.CreatedAt = now
	ns.UpdatedAt = now

	dbResult, err := s.sq.
		Insert(NamespacesTable).
		Columns(ns.cols()...).
		Values(ns.values()...).
		RunWith(tx).
		Exec()
	if err != nil {
		return errors.Wrap(err, "failed to create namespace")
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to create namespace")
	}

	if affected == 0 {
		return errors.New("failed to create namespace; no rows inserted")
	}

	return nil
}

func (s *service) CreateNamespace(ctx context.Context, ns *Namespace) error {
	ns.normalize()
	if err := ns.Validate(); err != nil {
		return err
	}

	err := s.transaction(func(tx *sql.Tx) error {
		return s.createNamespace(ctx, tx, ns)
	})

	return err
}

// EnsureNamespaceByPath ensures that a namespace exists with the given path, creating it if necessary. It will
// recursively create all parent namespaces if they do not exist. If any parent is not active, it will return an error.
func (s *service) EnsureNamespaceByPath(ctx context.Context, path string) error {
	if err := aschema.ValidateNamespacePath(path); err != nil {
		return err
	}

	err := s.transaction(func(tx *sql.Tx) error {
		prefixes := SplitNamespacePathToPrefixes(path)

		for i := 0; i < len(prefixes); i++ {
			ns, err := s.getNamespaceByPath(ctx, tx, prefixes[i])
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					err = s.createNamespace(ctx, tx, &Namespace{
						Path:  prefixes[i],
						State: NamespaceStateActive,
					})
					if err != nil {
						return err
					}
					continue
				}

				return err
			}

			if ns.State != NamespaceStateActive {
				return api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithPublicErrf("namespace '%s' is not active", prefixes[i]).
					Build()
			}
		}

		return nil
	})

	return err
}

func (s *service) GetNamespace(ctx context.Context, path string) (*Namespace, error) {
	return s.getNamespaceByPath(ctx, s.db, path)
}

func (s *service) DeleteNamespace(ctx context.Context, path string) error {
	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(NamespacesTable).
		Set("updated_at", now).
		Set("deleted_at", now).
		Where(sq.Eq{"path": path}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return errors.Wrap(err, "failed to soft delete namespace")
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to soft delete namespace")
	}

	if affected == 0 {
		return ErrNotFound
	}

	if affected > 1 {
		return errors.Wrap(ErrViolation, "multiple namespaces were soft deleted")
	}

	return nil
}

func (s *service) SetNamespaceState(ctx context.Context, path string, state NamespaceState) error {
	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(NamespacesTable).
		Set("updated_at", now).
		Set("state", state).
		Where(sq.Eq{"path": path}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return errors.Wrap(err, "failed to soft delete namespace")
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to soft delete namespace")
	}

	if affected == 0 {
		return errors.Wrap(ErrNotFound, "failed to set namespace state")
	}

	if affected > 1 {
		return errors.Wrap(ErrViolation, "multiple namespaces were soft deleted")
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
	ForDepth(depth uint64) ListNamespacesBuilder
	ForChildrenOf(path string) ListNamespacesBuilder
	ForNamespaceMatcher(matcher string) ListNamespacesBuilder
	ForNamespaceMatchers(matchers []string) ListNamespacesBuilder
	ForState(NamespaceState) ListNamespacesBuilder
	OrderBy(NamespaceOrderByField, pagination.OrderBy) ListNamespacesBuilder
	IncludeDeleted() ListNamespacesBuilder
	ForLabelSelector(selector string) ListNamespacesBuilder
}

type listNamespacesFilters struct {
	s                 *service               `json:"-"`
	LimitVal          uint64                 `json:"limit"`
	Offset            uint64                 `json:"offset"`
	StatesVal         []NamespaceState       `json:"states,omitempty"`
	PathPrefixVal     string                 `json:"path_prefix,omitempty"`
	DepthVal          *uint64                `json:"depth,omitempty"`
	NamespaceMatchers []string               `json:"namespace_matchers,omitempty"`
	OrderByFieldVal   *NamespaceOrderByField `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy    `json:"order_by"`
	IncludeDeletedVal bool                   `json:"include_deleted,omitempty"`
	LabelSelectorVal  *string                `json:"label_selector,omitempty"`
	Errors            *multierror.Error      `json:"-"`
}

func (l *listNamespacesFilters) addError(e error) ListNamespacesBuilder {
	l.Errors = multierror.Append(l.Errors, e)
	return l
}

func (l *listNamespacesFilters) Limit(limit int32) ListNamespacesBuilder {
	l.LimitVal = uint64(limit)
	return l
}

func (l *listNamespacesFilters) ForState(state NamespaceState) ListNamespacesBuilder {
	l.StatesVal = []NamespaceState{state}
	return l
}

func (l *listNamespacesFilters) ForPathPrefix(prefix string) ListNamespacesBuilder {
	l.PathPrefixVal = prefix
	return l
}

func (l *listNamespacesFilters) ForDepth(depth uint64) ListNamespacesBuilder {
	l.DepthVal = &depth
	return l
}

func (l *listNamespacesFilters) ForChildrenOf(path string) ListNamespacesBuilder {
	if err := ValidateNamespacePath(path); err != nil {
		return l.addError(err)
	} else {
		currDepth := DepthOfNamespacePath(path)
		return l.
			ForDepth(currDepth + 1).
			ForPathPrefix(path)
	}
}

func (l *listNamespacesFilters) ForNamespaceMatcher(matcher string) ListNamespacesBuilder {
	if err := ValidateNamespaceMatcher(matcher); err != nil {
		return l.addError(err)
	} else {
		l.NamespaceMatchers = []string{matcher}
	}

	return l
}

func (l *listNamespacesFilters) ForNamespaceMatchers(matchers []string) ListNamespacesBuilder {
	for _, matcher := range matchers {
		if err := ValidateNamespaceMatcher(matcher); err != nil {
			return l.addError(err)
		}
	}
	l.NamespaceMatchers = matchers
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

func (l *listNamespacesFilters) ForLabelSelector(selector string) ListNamespacesBuilder {
	l.LabelSelectorVal = &selector
	return l
}

func (l *listNamespacesFilters) FromCursor(ctx context.Context, cursor string) (ListNamespacesExecutor, error) {
	s := l.s
	parsed, err := pagination.ParseCursor[listNamespacesFilters](ctx, s.secretKey, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.s = s

	return l, nil
}

func (l *listNamespacesFilters) applyRestrictions(ctx context.Context) sq.SelectBuilder {
	q := l.s.sq.
		Select(util.ToPtr(Namespace{}).cols()...).
		From(NamespacesTable)

	if l.LabelSelectorVal != nil {
		selector, err := ParseLabelSelector(*l.LabelSelectorVal)
		if err != nil {
			l.addError(err)
		} else {
			q = selector.ApplyToSqlBuilder(q, "labels")
		}
	}

	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	if l.PathPrefixVal != "" {
		if len(l.PathPrefixVal) >= 2 && string(l.PathPrefixVal[len(l.PathPrefixVal)-1]) == aschema.NamespacePathSeparator {
			q = q.Where("(path = ? OR path LIKE ?)", l.PathPrefixVal[:len(l.PathPrefixVal)-2], l.PathPrefixVal+"%")
		} else {
			q = q.Where("(path = ? OR path LIKE ?)", l.PathPrefixVal, l.PathPrefixVal+aschema.NamespacePathSeparator+"%")
		}
	}

	if l.DepthVal != nil {
		q = q.Where(sq.Eq{"depth": *l.DepthVal})
	}

	if len(l.StatesVal) > 0 {
		q = q.Where(sq.Eq{"state": l.StatesVal})
	}

	if !l.IncludeDeletedVal {
		q = q.Where(sq.Eq{"deleted_at": nil})
	}

	if len(l.NamespaceMatchers) > 0 {
		q = restrictToNamespaceMatchers(q, "path", l.NamespaceMatchers)
	}

	// Always limit to one more than limit to check if there are more records
	q = q.Limit(l.LimitVal + 1).Offset(l.Offset)

	if l.OrderByFieldVal != nil {
		q = q.OrderBy(fmt.Sprintf("%s %s", *l.OrderByFieldVal, l.OrderByVal.String()))
	}

	return q
}

func (l *listNamespacesFilters) fetchPage(ctx context.Context) pagination.PageResult[Namespace] {
	var err error

	if err = l.Errors.ErrorOrNil(); err != nil {
		return pagination.PageResult[Namespace]{Error: err}
	}

	rows, err := l.applyRestrictions(ctx).
		RunWith(l.s.db).
		Query()
	if err != nil {
		return pagination.PageResult[Namespace]{Error: err}
	}
	defer rows.Close()

	var results []Namespace
	for rows.Next() {
		var r Namespace
		err := rows.Scan(r.fields()...)
		if err != nil {
			return pagination.PageResult[Namespace]{Error: err}
		}
		results = append(results, r)
	}

	l.Offset = l.Offset + uint64(len(results)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := uint64(len(results)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.secretKey, l)
		if err != nil {
			return pagination.PageResult[Namespace]{Error: err}
		}
	}

	return pagination.PageResult[Namespace]{
		HasMore: hasMore,
		Results: results[:util.MinUint64(l.LimitVal, uint64(len(results)))],
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
		s:        s,
		LimitVal: 100,
	}
}

func (s *service) ListNamespacesFromCursor(ctx context.Context, cursor string) (ListNamespacesExecutor, error) {
	b := &listNamespacesFilters{
		s:        s,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}

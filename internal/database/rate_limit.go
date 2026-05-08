package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	rlschema "github.com/rmorlok/authproxy/internal/schema/rate_limit"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const RateLimitsTable = "rate_limits"

// RateLimit is the database envelope for a rate-limit resource. Definition
// holds the JSON-serialised configuration (mode, selector, bucket, algorithm).
type RateLimit struct {
	Id          apid.ID
	Namespace   string
	Definition  rlschema.RateLimit
	Labels      Labels
	Annotations Annotations
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

func (rl *RateLimit) GetNamespace() string {
	return rl.Namespace
}

func (rl *RateLimit) cols() []string {
	return []string{
		"id",
		"namespace",
		"definition",
		"labels",
		"annotations",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

func (rl *RateLimit) fields() []any {
	return []any{
		&rl.Id,
		&rl.Namespace,
		(*rateLimitDefDB)(&rl.Definition),
		&rl.Labels,
		&rl.Annotations,
		&rl.CreatedAt,
		&rl.UpdatedAt,
		&rl.DeletedAt,
	}
}

func (rl *RateLimit) values() []any {
	return []any{
		rl.Id,
		rl.Namespace,
		rateLimitDefDB(rl.Definition),
		rl.Labels,
		rl.Annotations,
		rl.CreatedAt,
		rl.UpdatedAt,
		rl.DeletedAt,
	}
}

// rateLimitDefDB is a database-side wrapper around rlschema.RateLimit that
// implements driver.Valuer and sql.Scanner for the definition column.
type rateLimitDefDB rlschema.RateLimit

func (d rateLimitDefDB) Value() (driver.Value, error) {
	return json.Marshal(rlschema.RateLimit(d))
}

func (d *rateLimitDefDB) Scan(src interface{}) error {
	var b []byte
	switch v := src.(type) {
	case nil:
		return nil
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("rateLimitDefDB: cannot scan %T", src)
	}
	return json.Unmarshal(b, (*rlschema.RateLimit)(d))
}

func (rl *RateLimit) Validate() error {
	result := &multierror.Error{}

	if rl.Id.IsNil() {
		result = multierror.Append(result, errors.New("id is required"))
	} else if err := rl.Id.ValidatePrefix(apid.PrefixRateLimit); err != nil {
		result = multierror.Append(result, err)
	}

	if rl.Namespace == "" {
		result = multierror.Append(result, errors.New("namespace is required"))
	}

	if err := rl.Definition.Validate(); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid definition: %w", err))
	}

	if err := rl.Labels.Validate(); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid labels: %w", err))
	}

	if err := rl.Annotations.Validate(); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid annotations: %w", err))
	}

	return result.ErrorOrNil()
}

func (s *service) GetRateLimit(ctx context.Context, id apid.ID) (*RateLimit, error) {
	var result RateLimit
	err := s.sq.
		Select(result.cols()...).
		From(RateLimitsTable).
		Where(sq.Eq{
			"id":         id,
			"deleted_at": nil,
		}).
		RunWith(s.db).QueryRow().
		Scan(result.fields()...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (s *service) CreateRateLimit(ctx context.Context, rl *RateLimit) error {
	if err := rl.Validate(); err != nil {
		return err
	}

	return s.transaction(func(tx *sql.Tx) error {
		nsLabels, err := s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
			"path":       rl.Namespace,
			"deleted_at": nil,
		})
		if err != nil {
			return err
		}

		rl.Labels = ApplyParentCarryForward(
			rl.Labels,
			ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels},
		)
		rl.Labels = InjectSelfImplicitLabels(rl.Id, rl.Namespace, rl.Labels)

		now := apctx.GetClock(ctx).Now()
		rl.CreatedAt = now
		rl.UpdatedAt = now

		dbResult, err := s.sq.
			Insert(RateLimitsTable).
			Columns(rl.cols()...).
			Values(rl.values()...).
			RunWith(tx).
			Exec()
		if err != nil {
			return fmt.Errorf("failed to create rate limit: %w", err)
		}

		affected, err := dbResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to create rate limit: %w", err)
		}

		if affected == 0 {
			return errors.New("failed to create rate limit; no rows inserted")
		}

		return nil
	})
}

// UpdateRateLimitDefinition replaces the definition column. The caller is
// responsible for validation; this method runs Validate() on a candidate
// RateLimit so misconfigured definitions never reach the database.
func (s *service) UpdateRateLimitDefinition(ctx context.Context, id apid.ID, def rlschema.RateLimit) (*RateLimit, error) {
	if id.IsNil() {
		return nil, errors.New("rate limit id is required")
	}

	candidate := &RateLimit{
		Id:         id,
		Namespace:  "validation-only",
		Definition: def,
	}
	if err := candidate.Definition.Validate(); err != nil {
		return nil, fmt.Errorf("invalid definition: %w", err)
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(RateLimitsTable).
		Set("definition", rateLimitDefDB(def)).
		Set("updated_at", now).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to update rate limit definition: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update rate limit definition: %w", err)
	}
	if affected == 0 {
		return nil, ErrNotFound
	}

	return s.GetRateLimit(ctx, id)
}

func (s *service) DeleteRateLimit(ctx context.Context, id apid.ID) error {
	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(RateLimitsTable).
		Set("updated_at", now).
		Set("deleted_at", now).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to soft delete rate limit: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to soft delete rate limit: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateRateLimitLabels replaces all user labels on a rate limit.
func (s *service) UpdateRateLimitLabels(ctx context.Context, id apid.ID, labels map[string]string) (*RateLimit, error) {
	if id.IsNil() {
		return nil, errors.New("rate limit id is required")
	}

	if labels != nil {
		if err := ValidateUserLabels(labels); err != nil {
			return nil, fmt.Errorf("invalid labels: %w", err)
		}
	}

	var result *RateLimit
	err := s.transaction(func(tx *sql.Tx) error {
		var rl RateLimit
		err := s.sq.
			Select(rl.cols()...).
			From(RateLimitsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(rl.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		merged, now, err := s.replaceUserLabelsInTableTx(ctx, tx, RateLimitsTable, sq.Eq{"id": id, "deleted_at": nil}, Labels(labels))
		if err != nil {
			return err
		}

		rl.Labels = merged
		rl.UpdatedAt = now
		result = &rl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// PutRateLimitLabels merges the supplied labels into the existing set.
func (s *service) PutRateLimitLabels(ctx context.Context, id apid.ID, labels map[string]string) (*RateLimit, error) {
	if id.IsNil() {
		return nil, errors.New("rate limit id is required")
	}

	if len(labels) == 0 {
		return s.GetRateLimit(ctx, id)
	}

	if err := ValidateUserLabels(labels); err != nil {
		return nil, fmt.Errorf("invalid labels: %w", err)
	}

	var result *RateLimit
	err := s.transaction(func(tx *sql.Tx) error {
		var rl RateLimit
		err := s.sq.
			Select(rl.cols()...).
			From(RateLimitsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(rl.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		mergedLabels, now, err := s.putLabelsInTableTx(ctx, tx, RateLimitsTable, sq.Eq{"id": id, "deleted_at": nil}, labels)
		if err != nil {
			return err
		}

		rl.Labels = mergedLabels
		rl.UpdatedAt = now
		result = &rl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteRateLimitLabels removes the specified user-label keys.
func (s *service) DeleteRateLimitLabels(ctx context.Context, id apid.ID, keys []string) (*RateLimit, error) {
	if id.IsNil() {
		return nil, errors.New("rate limit id is required")
	}

	if len(keys) == 0 {
		return s.GetRateLimit(ctx, id)
	}

	if err := ValidateUserLabelDeletionKeys(keys); err != nil {
		return nil, fmt.Errorf("invalid label keys: %w", err)
	}

	var result *RateLimit
	err := s.transaction(func(tx *sql.Tx) error {
		var rl RateLimit
		err := s.sq.
			Select(rl.cols()...).
			From(RateLimitsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(rl.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		remainingLabels, now, err := s.deleteLabelsInTableTx(ctx, tx, RateLimitsTable, sq.Eq{"id": id, "deleted_at": nil}, keys)
		if err != nil {
			return err
		}

		rl.Labels = remainingLabels
		rl.UpdatedAt = now
		result = &rl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// UpdateRateLimitAnnotations replaces all annotations on a rate limit.
func (s *service) UpdateRateLimitAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*RateLimit, error) {
	if id.IsNil() {
		return nil, errors.New("rate limit id is required")
	}

	if annotations != nil {
		if err := ValidateAnnotations(annotations); err != nil {
			return nil, fmt.Errorf("invalid annotations: %w", err)
		}
	}

	var result *RateLimit
	err := s.transaction(func(tx *sql.Tx) error {
		var rl RateLimit
		err := s.sq.
			Select(rl.cols()...).
			From(RateLimitsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(rl.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		now, err := s.updateAnnotationsInTableTx(ctx, tx, RateLimitsTable, sq.Eq{"id": id, "deleted_at": nil}, Annotations(annotations))
		if err != nil {
			return err
		}

		rl.Annotations = annotations
		rl.UpdatedAt = now
		result = &rl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// PutRateLimitAnnotations merges the supplied annotations into the existing set.
func (s *service) PutRateLimitAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*RateLimit, error) {
	if id.IsNil() {
		return nil, errors.New("rate limit id is required")
	}

	if len(annotations) == 0 {
		return s.GetRateLimit(ctx, id)
	}

	if err := ValidateAnnotations(annotations); err != nil {
		return nil, fmt.Errorf("invalid annotations: %w", err)
	}

	var result *RateLimit
	err := s.transaction(func(tx *sql.Tx) error {
		var rl RateLimit
		err := s.sq.
			Select(rl.cols()...).
			From(RateLimitsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(rl.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		mergedAnnotations, now, err := s.putAnnotationsInTableTx(ctx, tx, RateLimitsTable, sq.Eq{"id": id, "deleted_at": nil}, annotations)
		if err != nil {
			return err
		}

		rl.Annotations = mergedAnnotations
		rl.UpdatedAt = now
		result = &rl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteRateLimitAnnotations removes the specified annotation keys.
func (s *service) DeleteRateLimitAnnotations(ctx context.Context, id apid.ID, keys []string) (*RateLimit, error) {
	if id.IsNil() {
		return nil, errors.New("rate limit id is required")
	}

	if len(keys) == 0 {
		return s.GetRateLimit(ctx, id)
	}

	var result *RateLimit
	err := s.transaction(func(tx *sql.Tx) error {
		var rl RateLimit
		err := s.sq.
			Select(rl.cols()...).
			From(RateLimitsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(rl.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		remainingAnnotations, now, err := s.deleteAnnotationsInTableTx(ctx, tx, RateLimitsTable, sq.Eq{"id": id, "deleted_at": nil}, keys)
		if err != nil {
			return err
		}

		rl.Annotations = remainingAnnotations
		rl.UpdatedAt = now
		result = &rl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ----- list builder -----

type RateLimitOrderByField string

const (
	RateLimitOrderByCreatedAt RateLimitOrderByField = "created_at"
	RateLimitOrderByUpdatedAt RateLimitOrderByField = "updated_at"
	RateLimitOrderByNamespace RateLimitOrderByField = "namespace"
)

func IsValidRateLimitOrderByField[T string | RateLimitOrderByField](field T) bool {
	switch RateLimitOrderByField(field) {
	case RateLimitOrderByCreatedAt,
		RateLimitOrderByUpdatedAt,
		RateLimitOrderByNamespace:
		return true
	default:
		return false
	}
}

type ListRateLimitsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[RateLimit]
	Enumerate(context.Context, pagination.EnumerateCallback[RateLimit]) error
}

type ListRateLimitsBuilder interface {
	ListRateLimitsExecutor
	Limit(int32) ListRateLimitsBuilder
	ForNamespaceMatcher(matcher string) ListRateLimitsBuilder
	ForNamespaceMatchers(matchers []string) ListRateLimitsBuilder
	OrderBy(RateLimitOrderByField, pagination.OrderBy) ListRateLimitsBuilder
	IncludeDeleted() ListRateLimitsBuilder
	ForLabelSelector(selector string) ListRateLimitsBuilder
}

type listRateLimitsFilters struct {
	s                 *service               `json:"-"`
	LimitVal          uint64                 `json:"limit"`
	Offset            uint64                 `json:"offset"`
	NamespaceMatchers []string               `json:"namespace_matchers,omitempty"`
	OrderByFieldVal   *RateLimitOrderByField `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy    `json:"order_by"`
	IncludeDeletedVal bool                   `json:"include_deleted,omitempty"`
	LabelSelectorVal  *string                `json:"label_selector,omitempty"`
	Errors            *multierror.Error      `json:"-"`
}

func (l *listRateLimitsFilters) addError(e error) ListRateLimitsBuilder {
	l.Errors = multierror.Append(l.Errors, e)
	return l
}

func (l *listRateLimitsFilters) Limit(limit int32) ListRateLimitsBuilder {
	l.LimitVal = uint64(limit)
	return l
}

func (l *listRateLimitsFilters) ForNamespaceMatcher(matcher string) ListRateLimitsBuilder {
	if err := ValidateNamespaceMatcher(matcher); err != nil {
		return l.addError(err)
	}
	l.NamespaceMatchers = []string{matcher}
	return l
}

func (l *listRateLimitsFilters) ForNamespaceMatchers(matchers []string) ListRateLimitsBuilder {
	for _, matcher := range matchers {
		if err := ValidateNamespaceMatcher(matcher); err != nil {
			return l.addError(err)
		}
	}
	l.NamespaceMatchers = matchers
	return l
}

func (l *listRateLimitsFilters) OrderBy(field RateLimitOrderByField, by pagination.OrderBy) ListRateLimitsBuilder {
	l.OrderByFieldVal = &field
	l.OrderByVal = &by
	return l
}

func (l *listRateLimitsFilters) IncludeDeleted() ListRateLimitsBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listRateLimitsFilters) ForLabelSelector(selector string) ListRateLimitsBuilder {
	l.LabelSelectorVal = &selector
	return l
}

func (l *listRateLimitsFilters) FromCursor(ctx context.Context, cursor string) (ListRateLimitsExecutor, error) {
	s := l.s
	parsed, err := pagination.ParseCursor[listRateLimitsFilters](ctx, s.cursorEncryptor, cursor)
	if err != nil {
		return nil, err
	}
	*l = *parsed
	l.s = s
	return l, nil
}

func (l *listRateLimitsFilters) applyRestrictions(ctx context.Context) sq.SelectBuilder {
	q := l.s.sq.
		Select(util.ToPtr(RateLimit{}).cols()...).
		From(RateLimitsTable)

	if l.LabelSelectorVal != nil {
		selector, err := ParseLabelSelector(*l.LabelSelectorVal)
		if err != nil {
			l.addError(err)
		} else {
			q = selector.ApplyToSqlBuilderWithProvider(q, "labels", l.s.cfg.GetProvider())
		}
	}

	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	if !l.IncludeDeletedVal {
		q = q.Where(sq.Eq{"deleted_at": nil})
	}

	if len(l.NamespaceMatchers) > 0 {
		q = restrictToNamespaceMatchers(q, "namespace", l.NamespaceMatchers)
	}

	q = q.Limit(l.LimitVal + 1).Offset(l.Offset)

	if l.OrderByFieldVal != nil {
		q = q.OrderBy(fmt.Sprintf("%s %s", *l.OrderByFieldVal, l.OrderByVal.String()))
	}

	return q
}

func (l *listRateLimitsFilters) fetchPage(ctx context.Context) pagination.PageResult[RateLimit] {
	var err error

	if err = l.Errors.ErrorOrNil(); err != nil {
		return pagination.PageResult[RateLimit]{Error: err}
	}

	rows, err := l.applyRestrictions(ctx).
		RunWith(l.s.db).
		Query()
	if err != nil {
		return pagination.PageResult[RateLimit]{Error: err}
	}
	defer rows.Close()

	var results []RateLimit
	for rows.Next() {
		var r RateLimit
		err := rows.Scan(r.fields()...)
		if err != nil {
			return pagination.PageResult[RateLimit]{Error: err}
		}
		results = append(results, r)
	}

	l.Offset = l.Offset + uint64(len(results)) - 1

	cursor := ""
	hasMore := uint64(len(results)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.cursorEncryptor, l)
		if err != nil {
			return pagination.PageResult[RateLimit]{Error: err}
		}
	}

	return pagination.PageResult[RateLimit]{
		HasMore: hasMore,
		Results: results[:util.MinUint64(l.LimitVal, uint64(len(results)))],
		Cursor:  cursor,
	}
}

func (l *listRateLimitsFilters) FetchPage(ctx context.Context) pagination.PageResult[RateLimit] {
	return l.fetchPage(ctx)
}

func (l *listRateLimitsFilters) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[RateLimit]) error {
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

func (s *service) ListRateLimitsBuilder() ListRateLimitsBuilder {
	return &listRateLimitsFilters{
		s:        s,
		LimitVal: 100,
	}
}

func (s *service) ListRateLimitsFromCursor(ctx context.Context, cursor string) (ListRateLimitsExecutor, error) {
	b := &listRateLimitsFilters{
		s:        s,
		LimitVal: 100,
	}
	return b.FromCursor(ctx, cursor)
}

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
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

func init() {
	RegisterEncryptedField(EncryptedFieldRegistration{
		Table:          EncryptionKeysTable,
		PrimaryKeyCols: []string{"id"},
		EncryptedCols:  []string{"encrypted_key_data"},
		NamespaceCol:   "namespace",
	})
}

const EncryptionKeysTable = "encryption_keys"

// GlobalEncryptionKeyID is the ID of the global encryption key created by migration. It is the root
// of the encryption key hierarchy and must not be deleted.
var GlobalEncryptionKeyID = apid.ID("ek_global")

type EncryptionKeyState string

const (
	EncryptionKeyStateActive   EncryptionKeyState = "active"
	EncryptionKeyStateDisabled EncryptionKeyState = "disabled"
)

func IsValidEncryptionKeyState[T string | EncryptionKeyState](state T) bool {
	switch EncryptionKeyState(state) {
	case EncryptionKeyStateActive, EncryptionKeyStateDisabled:
		return true
	default:
		return false
	}
}

type EncryptionKeyOrderByField string

const (
	EncryptionKeyOrderByState     EncryptionKeyOrderByField = "state"
	EncryptionKeyOrderByCreatedAt EncryptionKeyOrderByField = "created_at"
	EncryptionKeyOrderByUpdatedAt EncryptionKeyOrderByField = "updated_at"
)

func IsValidEncryptionKeyOrderByField[T string | EncryptionKeyOrderByField](field T) bool {
	switch EncryptionKeyOrderByField(field) {
	case EncryptionKeyOrderByState,
		EncryptionKeyOrderByCreatedAt,
		EncryptionKeyOrderByUpdatedAt:
		return true
	default:
		return false
	}
}

// EncryptionKey represents a user-managed encryption key configuration.
type EncryptionKey struct {
	Id               apid.ID
	Namespace        string
	EncryptedKeyData *encfield.EncryptedField
	State            EncryptionKeyState
	Labels           Labels
	Annotations      Annotations
	CreatedAt        time.Time
	UpdatedAt        time.Time
	EncryptedAt      *time.Time
	DeletedAt        *time.Time
}

func (ek *EncryptionKey) GetNamespace() string {
	return ek.Namespace
}

func (ek *EncryptionKey) cols() []string {
	return []string{
		"id",
		"namespace",
		"encrypted_key_data",
		"state",
		"labels",
		"annotations",
		"created_at",
		"updated_at",
		"encrypted_at",
		"deleted_at",
	}
}

func (ek *EncryptionKey) fields() []any {
	return []any{
		&ek.Id,
		&ek.Namespace,
		&ek.EncryptedKeyData,
		&ek.State,
		&ek.Labels,
		&ek.Annotations,
		&ek.CreatedAt,
		&ek.UpdatedAt,
		&ek.EncryptedAt,
		&ek.DeletedAt,
	}
}

func (ek *EncryptionKey) values() []any {
	return []any{
		ek.Id,
		ek.Namespace,
		ek.EncryptedKeyData,
		ek.State,
		ek.Labels,
		ek.Annotations,
		ek.CreatedAt,
		ek.UpdatedAt,
		ek.EncryptedAt,
		ek.DeletedAt,
	}
}

func (ek *EncryptionKey) normalize() {
	if ek.State == "" {
		ek.State = EncryptionKeyStateActive
	}
}

func (ek *EncryptionKey) Validate() error {
	result := &multierror.Error{}

	if ek.Id.IsNil() {
		result = multierror.Append(result, errors.New("id is required"))
	} else if err := ek.Id.ValidatePrefix(apid.PrefixEncryptionKey); err != nil {
		result = multierror.Append(result, err)
	}

	if ek.Namespace == "" {
		result = multierror.Append(result, errors.New("namespace is required"))
	}

	if !IsValidEncryptionKeyState(ek.State) {
		result = multierror.Append(result, errors.New("invalid encryption key state"))
	}

	if err := ek.Labels.Validate(); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid labels: %w", err))
	}

	if err := ek.Annotations.Validate(); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid annotations: %w", err))
	}

	return result.ErrorOrNil()
}

func (s *service) GetEncryptionKey(ctx context.Context, id apid.ID) (*EncryptionKey, error) {
	var result EncryptionKey
	err := s.sq.
		Select(result.cols()...).
		From(EncryptionKeysTable).
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

func (s *service) CreateEncryptionKey(ctx context.Context, ek *EncryptionKey) error {
	ek.normalize()
	if err := ek.Validate(); err != nil {
		return err
	}

	now := apctx.GetClock(ctx).Now()
	ek.CreatedAt = now
	ek.UpdatedAt = now

	dbResult, err := s.sq.
		Insert(EncryptionKeysTable).
		Columns(ek.cols()...).
		Values(ek.values()...).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to create encryption key: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to create encryption key: %w", err)
	}

	if affected == 0 {
		return errors.New("failed to create encryption key; no rows inserted")
	}

	return nil
}

func (s *service) UpdateEncryptionKey(ctx context.Context, id apid.ID, updates map[string]interface{}) (*EncryptionKey, error) {
	now := apctx.GetClock(ctx).Now()
	updates["updated_at"] = now

	q := s.sq.
		Update(EncryptionKeysTable).
		Where(sq.Eq{"id": id, "deleted_at": nil})

	for k, v := range updates {
		q = q.Set(k, v)
	}

	dbResult, err := q.RunWith(s.db).Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to update encryption key: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update encryption key: %w", err)
	}

	if affected == 0 {
		return nil, ErrNotFound
	}

	return s.GetEncryptionKey(ctx, id)
}

func (s *service) DeleteEncryptionKey(ctx context.Context, id apid.ID) error {
	if id == GlobalEncryptionKeyID {
		return fmt.Errorf("cannot delete the global encryption key: %w", ErrProtected)
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(EncryptionKeysTable).
		Set("updated_at", now).
		Set("deleted_at", now).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to soft delete encryption key: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to soft delete encryption key: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	// Soft-delete all associated encryption key versions
	if err := s.DeleteEncryptionKeyVersionsForEncryptionKey(ctx, id); err != nil {
		return fmt.Errorf("failed to delete encryption key versions for encryption key: %w", err)
	}

	return nil
}

func (s *service) SetEncryptionKeyState(ctx context.Context, id apid.ID, state EncryptionKeyState) error {
	if !IsValidEncryptionKeyState(state) {
		return errors.New("invalid encryption key state")
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(EncryptionKeysTable).
		Set("updated_at", now).
		Set("state", state).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to set encryption key state: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to set encryption key state: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdateEncryptionKeyLabels replaces all labels on an encryption key within a single transaction.
func (s *service) UpdateEncryptionKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (*EncryptionKey, error) {
	if id.IsNil() {
		return nil, errors.New("encryption key id is required")
	}

	if labels != nil {
		if err := ValidateLabels(labels); err != nil {
			return nil, fmt.Errorf("invalid labels: %w", err)
		}
	}

	var result *EncryptionKey

	err := s.transaction(func(tx *sql.Tx) error {
		// Verify the key exists
		var ek EncryptionKey
		err := s.sq.
			Select(ek.cols()...).
			From(EncryptionKeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		now, err := s.updateLabelsInTableTx(ctx, tx, EncryptionKeysTable, sq.Eq{"id": id, "deleted_at": nil}, Labels(labels))
		if err != nil {
			return err
		}

		ek.Labels = labels
		ek.UpdatedAt = now
		result = &ek
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// PutEncryptionKeyLabels adds or updates the specified labels on an encryption key within a single transaction.
func (s *service) PutEncryptionKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (*EncryptionKey, error) {
	if id.IsNil() {
		return nil, errors.New("encryption key id is required")
	}

	if len(labels) == 0 {
		return s.GetEncryptionKey(ctx, id)
	}

	if err := ValidateLabels(labels); err != nil {
		return nil, fmt.Errorf("invalid labels: %w", err)
	}

	var result *EncryptionKey

	err := s.transaction(func(tx *sql.Tx) error {
		var ek EncryptionKey
		err := s.sq.
			Select(ek.cols()...).
			From(EncryptionKeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		mergedLabels, now, err := s.putLabelsInTableTx(ctx, tx, EncryptionKeysTable, sq.Eq{"id": id, "deleted_at": nil}, labels)
		if err != nil {
			return err
		}

		ek.Labels = mergedLabels
		ek.UpdatedAt = now
		result = &ek
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteEncryptionKeyLabels removes the specified label keys from an encryption key within a single transaction.
func (s *service) DeleteEncryptionKeyLabels(ctx context.Context, id apid.ID, keys []string) (*EncryptionKey, error) {
	if id.IsNil() {
		return nil, errors.New("encryption key id is required")
	}

	if len(keys) == 0 {
		return s.GetEncryptionKey(ctx, id)
	}

	var result *EncryptionKey

	err := s.transaction(func(tx *sql.Tx) error {
		var ek EncryptionKey
		err := s.sq.
			Select(ek.cols()...).
			From(EncryptionKeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		remainingLabels, now, err := s.deleteLabelsInTableTx(ctx, tx, EncryptionKeysTable, sq.Eq{"id": id, "deleted_at": nil}, keys)
		if err != nil {
			return err
		}

		ek.Labels = remainingLabels
		ek.UpdatedAt = now
		result = &ek
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// UpdateEncryptionKeyAnnotations replaces all annotations on an encryption key within a single transaction.
func (s *service) UpdateEncryptionKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*EncryptionKey, error) {
	if id.IsNil() {
		return nil, errors.New("encryption key id is required")
	}

	if annotations != nil {
		if err := ValidateAnnotations(annotations); err != nil {
			return nil, fmt.Errorf("invalid annotations: %w", err)
		}
	}

	var result *EncryptionKey

	err := s.transaction(func(tx *sql.Tx) error {
		var ek EncryptionKey
		err := s.sq.
			Select(ek.cols()...).
			From(EncryptionKeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		now, err := s.updateAnnotationsInTableTx(ctx, tx, EncryptionKeysTable, sq.Eq{"id": id, "deleted_at": nil}, Annotations(annotations))
		if err != nil {
			return err
		}

		ek.Annotations = annotations
		ek.UpdatedAt = now
		result = &ek
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// PutEncryptionKeyAnnotations adds or updates the specified annotations on an encryption key within a single transaction.
func (s *service) PutEncryptionKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*EncryptionKey, error) {
	if id.IsNil() {
		return nil, errors.New("encryption key id is required")
	}

	if len(annotations) == 0 {
		return s.GetEncryptionKey(ctx, id)
	}

	if err := ValidateAnnotations(annotations); err != nil {
		return nil, fmt.Errorf("invalid annotations: %w", err)
	}

	var result *EncryptionKey

	err := s.transaction(func(tx *sql.Tx) error {
		var ek EncryptionKey
		err := s.sq.
			Select(ek.cols()...).
			From(EncryptionKeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		mergedAnnotations, now, err := s.putAnnotationsInTableTx(ctx, tx, EncryptionKeysTable, sq.Eq{"id": id, "deleted_at": nil}, annotations)
		if err != nil {
			return err
		}

		ek.Annotations = mergedAnnotations
		ek.UpdatedAt = now
		result = &ek
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteEncryptionKeyAnnotations removes the specified annotation keys from an encryption key within a single transaction.
func (s *service) DeleteEncryptionKeyAnnotations(ctx context.Context, id apid.ID, keys []string) (*EncryptionKey, error) {
	if id.IsNil() {
		return nil, errors.New("encryption key id is required")
	}

	if len(keys) == 0 {
		return s.GetEncryptionKey(ctx, id)
	}

	var result *EncryptionKey

	err := s.transaction(func(tx *sql.Tx) error {
		var ek EncryptionKey
		err := s.sq.
			Select(ek.cols()...).
			From(EncryptionKeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		remainingAnnotations, now, err := s.deleteAnnotationsInTableTx(ctx, tx, EncryptionKeysTable, sq.Eq{"id": id, "deleted_at": nil}, keys)
		if err != nil {
			return err
		}

		ek.Annotations = remainingAnnotations
		ek.UpdatedAt = now
		result = &ek
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

type ListEncryptionKeysExecutor interface {
	FetchPage(context.Context) pagination.PageResult[EncryptionKey]
	Enumerate(context.Context, func(pagination.PageResult[EncryptionKey]) (keepGoing pagination.KeepGoing, err error)) error
}

type ListEncryptionKeysBuilder interface {
	ListEncryptionKeysExecutor
	Limit(int32) ListEncryptionKeysBuilder
	ForNamespaceMatcher(matcher string) ListEncryptionKeysBuilder
	ForNamespaceMatchers(matchers []string) ListEncryptionKeysBuilder
	ForState(EncryptionKeyState) ListEncryptionKeysBuilder
	OrderBy(EncryptionKeyOrderByField, pagination.OrderBy) ListEncryptionKeysBuilder
	IncludeDeleted() ListEncryptionKeysBuilder
	ForLabelSelector(selector string) ListEncryptionKeysBuilder
}

type listEncryptionKeysFilters struct {
	s                 *service                   `json:"-"`
	LimitVal          uint64                     `json:"limit"`
	Offset            uint64                     `json:"offset"`
	StatesVal         []EncryptionKeyState       `json:"states,omitempty"`
	NamespaceMatchers []string                   `json:"namespace_matchers,omitempty"`
	OrderByFieldVal   *EncryptionKeyOrderByField `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy        `json:"order_by"`
	IncludeDeletedVal bool                       `json:"include_deleted,omitempty"`
	LabelSelectorVal  *string                    `json:"label_selector,omitempty"`
	Errors            *multierror.Error          `json:"-"`
}

func (l *listEncryptionKeysFilters) addError(e error) ListEncryptionKeysBuilder {
	l.Errors = multierror.Append(l.Errors, e)
	return l
}

func (l *listEncryptionKeysFilters) Limit(limit int32) ListEncryptionKeysBuilder {
	l.LimitVal = uint64(limit)
	return l
}

func (l *listEncryptionKeysFilters) ForState(state EncryptionKeyState) ListEncryptionKeysBuilder {
	l.StatesVal = []EncryptionKeyState{state}
	return l
}

func (l *listEncryptionKeysFilters) ForNamespaceMatcher(matcher string) ListEncryptionKeysBuilder {
	if err := ValidateNamespaceMatcher(matcher); err != nil {
		return l.addError(err)
	}
	l.NamespaceMatchers = []string{matcher}
	return l
}

func (l *listEncryptionKeysFilters) ForNamespaceMatchers(matchers []string) ListEncryptionKeysBuilder {
	for _, matcher := range matchers {
		if err := ValidateNamespaceMatcher(matcher); err != nil {
			return l.addError(err)
		}
	}
	l.NamespaceMatchers = matchers
	return l
}

func (l *listEncryptionKeysFilters) OrderBy(field EncryptionKeyOrderByField, by pagination.OrderBy) ListEncryptionKeysBuilder {
	l.OrderByFieldVal = &field
	l.OrderByVal = &by
	return l
}

func (l *listEncryptionKeysFilters) IncludeDeleted() ListEncryptionKeysBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listEncryptionKeysFilters) ForLabelSelector(selector string) ListEncryptionKeysBuilder {
	l.LabelSelectorVal = &selector
	return l
}

func (l *listEncryptionKeysFilters) FromCursor(ctx context.Context, cursor string) (ListEncryptionKeysExecutor, error) {
	s := l.s
	parsed, err := pagination.ParseCursor[listEncryptionKeysFilters](ctx, s.cursorEncryptor, cursor)
	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.s = s

	return l, nil
}

func (l *listEncryptionKeysFilters) applyRestrictions(ctx context.Context) sq.SelectBuilder {
	q := l.s.sq.
		Select(util.ToPtr(EncryptionKey{}).cols()...).
		From(EncryptionKeysTable)

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

	if len(l.StatesVal) > 0 {
		q = q.Where(sq.Eq{"state": l.StatesVal})
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

func (l *listEncryptionKeysFilters) fetchPage(ctx context.Context) pagination.PageResult[EncryptionKey] {
	var err error

	if err = l.Errors.ErrorOrNil(); err != nil {
		return pagination.PageResult[EncryptionKey]{Error: err}
	}

	rows, err := l.applyRestrictions(ctx).
		RunWith(l.s.db).
		Query()
	if err != nil {
		return pagination.PageResult[EncryptionKey]{Error: err}
	}
	defer rows.Close()

	var results []EncryptionKey
	for rows.Next() {
		var r EncryptionKey
		err := rows.Scan(r.fields()...)
		if err != nil {
			return pagination.PageResult[EncryptionKey]{Error: err}
		}
		results = append(results, r)
	}

	l.Offset = l.Offset + uint64(len(results)) - 1

	cursor := ""
	hasMore := uint64(len(results)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.cursorEncryptor, l)
		if err != nil {
			return pagination.PageResult[EncryptionKey]{Error: err}
		}
	}

	return pagination.PageResult[EncryptionKey]{
		HasMore: hasMore,
		Results: results[:util.MinUint64(l.LimitVal, uint64(len(results)))],
		Cursor:  cursor,
	}
}

func (l *listEncryptionKeysFilters) FetchPage(ctx context.Context) pagination.PageResult[EncryptionKey] {
	return l.fetchPage(ctx)
}

func (l *listEncryptionKeysFilters) Enumerate(ctx context.Context, callback func(pagination.PageResult[EncryptionKey]) (keepGoing pagination.KeepGoing, err error)) error {
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

func (s *service) ListEncryptionKeysBuilder() ListEncryptionKeysBuilder {
	return &listEncryptionKeysFilters{
		s:        s,
		LimitVal: 100,
	}
}

func (s *service) ListEncryptionKeysFromCursor(ctx context.Context, cursor string) (ListEncryptionKeysExecutor, error) {
	b := &listEncryptionKeysFilters{
		s:        s,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}

// EnumerateEncryptionKeysInDependencyOrder walks all non-deleted encryption keys in breadth-first
// order rooted at the global key (ek_global). Encryption keys form a tree: the root key has nil
// EncryptedKeyData, while every other key's EncryptedKeyData.ID references an encryption_key_version
// row whose encryption_key_id points to the parent encryption key. The method loads all keys and
// key-version mappings into memory, builds this tree, then invokes the callback once per depth level
// starting at depth 0 (the root). Keys whose parent cannot be resolved — because their
// EncryptedKeyData.ID references a missing or deleted encryption key version — are collected and
// returned as orphans. Return keepGoing=false from the callback to halt the walk early; orphans are
// still returned in that case.
func (s *service) EnumerateEncryptionKeysInDependencyOrder(
	ctx context.Context,
	callback func(keys []*EncryptionKey, depth int) (keepGoing pagination.KeepGoing, err error),
) ([]*EncryptionKey, error) {
	// Load all non-deleted encryption keys
	rows, err := s.sq.
		Select(util.ToPtr(EncryptionKey{}).cols()...).
		From(EncryptionKeysTable).
		Where(sq.Eq{"deleted_at": nil}).
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, fmt.Errorf("failed to query encryption keys: %w", err)
	}

	allKeys := make(map[apid.ID]*EncryptionKey)
	for rows.Next() {
		var ek EncryptionKey
		if err := rows.Scan(ek.fields()...); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan encryption key: %w", err)
		}
		allKeys[ek.Id] = &ek
	}
	rows.Close()

	if len(allKeys) == 0 {
		return nil, nil
	}

	// Load all non-deleted encryption key versions to build ekv_id -> encryption_key_id map
	ekvRows, err := s.sq.
		Select("id", "encryption_key_id").
		From(EncryptionKeyVersionsTable).
		Where(sq.Eq{"deleted_at": nil}).
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, fmt.Errorf("failed to query encryption key versions: %w", err)
	}

	ekvToEK := make(map[apid.ID]apid.ID) // ekv ID -> encryption key ID
	for ekvRows.Next() {
		var ekvID, ekID apid.ID
		if err := ekvRows.Scan(&ekvID, &ekID); err != nil {
			ekvRows.Close()
			return nil, fmt.Errorf("failed to scan encryption key version: %w", err)
		}
		ekvToEK[ekvID] = ekID
	}
	ekvRows.Close()

	// Build parent map and find root(s), collecting orphans
	// parentOf[childEKID] = parentEKID
	parentOf := make(map[apid.ID]apid.ID)
	var roots []*EncryptionKey
	var orphans []*EncryptionKey

	for _, ek := range allKeys {
		if ek.EncryptedKeyData == nil || ek.EncryptedKeyData.IsZero() {
			roots = append(roots, ek)
		} else {
			// EncryptedKeyData.ID is an encryption_key_version ID
			parentEKID, ok := ekvToEK[ek.EncryptedKeyData.ID]
			if ok {
				parentOf[ek.Id] = parentEKID
			} else {
				orphans = append(orphans, ek)
			}
		}
	}

	// Build children map
	children := make(map[apid.ID][]*EncryptionKey)
	for childID, parentID := range parentOf {
		children[parentID] = append(children[parentID], allKeys[childID])
	}

	// BFS by depth level
	currentLevel := roots
	depth := 0

	for len(currentLevel) > 0 {
		keepGoing, err := callback(currentLevel, depth)
		if err != nil {
			return orphans, err
		}
		if !keepGoing {
			return orphans, nil
		}

		var nextLevel []*EncryptionKey
		for _, ek := range currentLevel {
			nextLevel = append(nextLevel, children[ek.Id]...)
		}

		currentLevel = nextLevel
		depth++
	}

	return orphans, nil
}

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
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const KeysTable = "keys"

// GlobalKeyID is the ID of the global key created by migration. It is the root
// of the key hierarchy and must not be deleted.
var GlobalKeyID = apid.ID("key_global")

type KeyUsage string

const (
	KeyUsageDataEncryption KeyUsage = "data_encryption"
)

func IsValidKeyUsage[T string | KeyUsage](usage T) bool {
	switch KeyUsage(usage) {
	case KeyUsageDataEncryption:
		return true
	default:
		return false
	}
}

type KeyMaterialType string

const (
	KeyMaterialTypeSymmetric KeyMaterialType = "symmetric"
	KeyMaterialTypePublic    KeyMaterialType = "public"
	KeyMaterialTypePrivate   KeyMaterialType = "private"
	KeyMaterialTypeExternal  KeyMaterialType = "external"
)

func IsValidKeyMaterialType[T string | KeyMaterialType](materialType T) bool {
	switch KeyMaterialType(materialType) {
	case KeyMaterialTypeSymmetric,
		KeyMaterialTypePublic,
		KeyMaterialTypePrivate,
		KeyMaterialTypeExternal:
		return true
	default:
		return false
	}
}

type KeyState string

const (
	KeyStateActive   KeyState = "active"
	KeyStateDisabled KeyState = "disabled"
)

func IsValidKeyState[T string | KeyState](state T) bool {
	switch KeyState(state) {
	case KeyStateActive, KeyStateDisabled:
		return true
	default:
		return false
	}
}

type KeyOrderByField string

const (
	KeyOrderByState     KeyOrderByField = "state"
	KeyOrderByCreatedAt KeyOrderByField = "created_at"
	KeyOrderByUpdatedAt KeyOrderByField = "updated_at"
)

func IsValidKeyOrderByField[T string | KeyOrderByField](field T) bool {
	switch KeyOrderByField(field) {
	case KeyOrderByState,
		KeyOrderByCreatedAt,
		KeyOrderByUpdatedAt:
		return true
	default:
		return false
	}
}

// Key represents a user-managed key configuration.
type Key struct {
	Id           apid.ID
	Namespace    string
	Usage        KeyUsage
	MaterialType KeyMaterialType
	// Key provider configuration is encrypted at rest, but it is not registered
	// with namespace re-encryption. It defines the key hierarchy, so rewrapping
	// it based on namespace targets can make a key depend on its own DEK.
	EncryptedKeyData *encfield.EncryptedField
	State            KeyState
	Labels           Labels
	Annotations      Annotations
	CreatedAt        time.Time
	UpdatedAt        time.Time
	EncryptedAt      *time.Time
	DeletedAt        *time.Time
}

func (ek *Key) GetNamespace() string {
	return ek.Namespace
}

func (ek *Key) cols() []string {
	return []string{
		"id",
		"namespace",
		"usage",
		"material_type",
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

func (ek *Key) fields() []any {
	return []any{
		&ek.Id,
		&ek.Namespace,
		&ek.Usage,
		&ek.MaterialType,
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

func (ek *Key) values() []any {
	return []any{
		ek.Id,
		ek.Namespace,
		ek.Usage,
		ek.MaterialType,
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

func (ek *Key) normalize() {
	if ek.Usage == "" {
		ek.Usage = KeyUsageDataEncryption
	}
	if ek.MaterialType == "" {
		ek.MaterialType = KeyMaterialTypeSymmetric
	}
	if ek.State == "" {
		ek.State = KeyStateActive
	}
}

func (ek *Key) Validate() error {
	result := &multierror.Error{}

	if ek.Id.IsNil() {
		result = multierror.Append(result, errors.New("id is required"))
	} else if err := ek.Id.ValidatePrefix(apid.PrefixKey); err != nil {
		result = multierror.Append(result, err)
	}

	if ek.Namespace == "" {
		result = multierror.Append(result, errors.New("namespace is required"))
	}

	if !IsValidKeyUsage(ek.Usage) {
		result = multierror.Append(result, errors.New("invalid encryption key usage"))
	}

	if !IsValidKeyMaterialType(ek.MaterialType) {
		result = multierror.Append(result, errors.New("invalid encryption key material type"))
	}

	if !IsValidKeyState(ek.State) {
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

func (s *service) GetKey(ctx context.Context, id apid.ID) (*Key, error) {
	var result Key
	err := s.sq.
		Select(result.cols()...).
		From(KeysTable).
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

func (s *service) CreateKey(ctx context.Context, ek *Key) error {
	ek.normalize()
	if err := ek.Validate(); err != nil {
		return err
	}

	return s.transaction(func(tx *sql.Tx) error {
		nsLabels, err := s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
			"path":       ek.Namespace,
			"deleted_at": nil,
		})
		if err != nil {
			return err
		}

		ek.Labels = ApplyParentCarryForward(
			ek.Labels,
			ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels},
		)
		ek.Labels = InjectSelfImplicitLabels(ek.Id, ek.Namespace, ek.Labels)
		now := apctx.GetClock(ctx).Now()
		ek.CreatedAt = now
		ek.UpdatedAt = now

		dbResult, err := s.sq.
			Insert(KeysTable).
			Columns(ek.cols()...).
			Values(ek.values()...).
			RunWith(tx).
			Exec()
		if err != nil {
			return fmt.Errorf("failed to create key: %w", err)
		}

		affected, err := dbResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to create key: %w", err)
		}

		if affected == 0 {
			return errors.New("failed to create key; no rows inserted")
		}

		return nil
	})
}

func (s *service) UpdateKey(ctx context.Context, id apid.ID, updates map[string]interface{}) (*Key, error) {
	now := apctx.GetClock(ctx).Now()
	updates["updated_at"] = now

	q := s.sq.
		Update(KeysTable).
		Where(sq.Eq{"id": id, "deleted_at": nil})

	for k, v := range updates {
		q = q.Set(k, v)
	}

	dbResult, err := q.RunWith(s.db).Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to update key: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update key: %w", err)
	}

	if affected == 0 {
		return nil, ErrNotFound
	}

	return s.GetKey(ctx, id)
}

func (s *service) DeleteKey(ctx context.Context, id apid.ID) error {
	if id == GlobalKeyID {
		return fmt.Errorf("cannot delete the global key: %w", ErrProtected)
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(KeysTable).
		Set("updated_at", now).
		Set("deleted_at", now).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to soft delete key: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to soft delete key: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	if err := s.deleteDataEncryptionKeysForKey(ctx, id, now); err != nil {
		return fmt.Errorf("failed to delete data encryption keys for key: %w", err)
	}

	return nil
}

func (s *service) deleteDataEncryptionKeysForKey(ctx context.Context, keyId apid.ID, deletedAt time.Time) error {
	_, err := s.sq.
		Update(DataEncryptionKeysTable).
		Set("updated_at", deletedAt).
		Set("deleted_at", deletedAt).
		Where(sq.Eq{
			"key_id":     keyId,
			"deleted_at": nil,
		}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to soft delete data encryption keys for key %s: %w", keyId, err)
	}

	return nil
}

func (s *service) SetKeyState(ctx context.Context, id apid.ID, state KeyState) error {
	if !IsValidKeyState(state) {
		return errors.New("invalid key state")
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(KeysTable).
		Set("updated_at", now).
		Set("state", state).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to set key state: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to set key state: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdateKeyLabels replaces all labels on an encryption key within a single transaction.
func (s *service) UpdateKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Key, error) {
	if id.IsNil() {
		return nil, errors.New("encryption key id is required")
	}

	if labels != nil {
		if err := ValidateUserLabels(labels); err != nil {
			return nil, fmt.Errorf("invalid labels: %w", err)
		}
	}

	var result *Key

	err := s.transaction(func(tx *sql.Tx) error {
		// Verify the key exists
		var ek Key
		err := s.sq.
			Select(ek.cols()...).
			From(KeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		merged, now, err := s.replaceUserLabelsInTableTx(ctx, tx, KeysTable, sq.Eq{"id": id, "deleted_at": nil}, Labels(labels))
		if err != nil {
			return err
		}

		ek.Labels = merged
		ek.UpdatedAt = now
		result = &ek
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// PutKeyLabels adds or updates the specified labels on an encryption key within a single transaction.
func (s *service) PutKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Key, error) {
	if id.IsNil() {
		return nil, errors.New("encryption key id is required")
	}

	if len(labels) == 0 {
		return s.GetKey(ctx, id)
	}

	if err := ValidateUserLabels(labels); err != nil {
		return nil, fmt.Errorf("invalid labels: %w", err)
	}

	var result *Key

	err := s.transaction(func(tx *sql.Tx) error {
		var ek Key
		err := s.sq.
			Select(ek.cols()...).
			From(KeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		mergedLabels, now, err := s.putLabelsInTableTx(ctx, tx, KeysTable, sq.Eq{"id": id, "deleted_at": nil}, labels)
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

// DeleteKeyLabels removes the specified label keys from an encryption key within a single transaction.
func (s *service) DeleteKeyLabels(ctx context.Context, id apid.ID, keys []string) (*Key, error) {
	if id.IsNil() {
		return nil, errors.New("encryption key id is required")
	}

	if len(keys) == 0 {
		return s.GetKey(ctx, id)
	}

	if err := ValidateUserLabelDeletionKeys(keys); err != nil {
		return nil, fmt.Errorf("invalid label keys: %w", err)
	}

	var result *Key

	err := s.transaction(func(tx *sql.Tx) error {
		var ek Key
		err := s.sq.
			Select(ek.cols()...).
			From(KeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		remainingLabels, now, err := s.deleteLabelsInTableTx(ctx, tx, KeysTable, sq.Eq{"id": id, "deleted_at": nil}, keys)
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

// UpdateKeyAnnotations replaces all annotations on an encryption key within a single transaction.
func (s *service) UpdateKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Key, error) {
	if id.IsNil() {
		return nil, errors.New("encryption key id is required")
	}

	if annotations != nil {
		if err := ValidateAnnotations(annotations); err != nil {
			return nil, fmt.Errorf("invalid annotations: %w", err)
		}
	}

	var result *Key

	err := s.transaction(func(tx *sql.Tx) error {
		var ek Key
		err := s.sq.
			Select(ek.cols()...).
			From(KeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		now, err := s.updateAnnotationsInTableTx(ctx, tx, KeysTable, sq.Eq{"id": id, "deleted_at": nil}, Annotations(annotations))
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

// PutKeyAnnotations adds or updates the specified annotations on an encryption key within a single transaction.
func (s *service) PutKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Key, error) {
	if id.IsNil() {
		return nil, errors.New("encryption key id is required")
	}

	if len(annotations) == 0 {
		return s.GetKey(ctx, id)
	}

	if err := ValidateAnnotations(annotations); err != nil {
		return nil, fmt.Errorf("invalid annotations: %w", err)
	}

	var result *Key

	err := s.transaction(func(tx *sql.Tx) error {
		var ek Key
		err := s.sq.
			Select(ek.cols()...).
			From(KeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		mergedAnnotations, now, err := s.putAnnotationsInTableTx(ctx, tx, KeysTable, sq.Eq{"id": id, "deleted_at": nil}, annotations)
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

// DeleteKeyAnnotations removes the specified annotation keys from an encryption key within a single transaction.
func (s *service) DeleteKeyAnnotations(ctx context.Context, id apid.ID, keys []string) (*Key, error) {
	if id.IsNil() {
		return nil, errors.New("encryption key id is required")
	}

	if len(keys) == 0 {
		return s.GetKey(ctx, id)
	}

	var result *Key

	err := s.transaction(func(tx *sql.Tx) error {
		var ek Key
		err := s.sq.
			Select(ek.cols()...).
			From(KeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		remainingAnnotations, now, err := s.deleteAnnotationsInTableTx(ctx, tx, KeysTable, sq.Eq{"id": id, "deleted_at": nil}, keys)
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

type ListKeysExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Key]
	Enumerate(context.Context, pagination.EnumerateCallback[Key]) error
}

type ListKeysBuilder interface {
	ListKeysExecutor
	Limit(int32) ListKeysBuilder
	ForNamespaceMatcher(matcher string) ListKeysBuilder
	ForNamespaceMatchers(matchers []string) ListKeysBuilder
	ForState(KeyState) ListKeysBuilder
	OrderBy(KeyOrderByField, pagination.OrderBy) ListKeysBuilder
	IncludeDeleted() ListKeysBuilder
	ForLabelSelector(selector string) ListKeysBuilder
}

type listKeysFilters struct {
	s                 *service            `json:"-"`
	LimitVal          uint64              `json:"limit"`
	Offset            uint64              `json:"offset"`
	StatesVal         []KeyState          `json:"states,omitempty"`
	NamespaceMatchers []string            `json:"namespace_matchers,omitempty"`
	OrderByFieldVal   *KeyOrderByField    `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy `json:"order_by"`
	IncludeDeletedVal bool                `json:"include_deleted,omitempty"`
	LabelSelectorVal  *string             `json:"label_selector,omitempty"`
	Errors            *multierror.Error   `json:"-"`
}

func (l *listKeysFilters) addError(e error) ListKeysBuilder {
	l.Errors = multierror.Append(l.Errors, e)
	return l
}

func (l *listKeysFilters) Limit(limit int32) ListKeysBuilder {
	l.LimitVal = uint64(limit)
	return l
}

func (l *listKeysFilters) ForState(state KeyState) ListKeysBuilder {
	l.StatesVal = []KeyState{state}
	return l
}

func (l *listKeysFilters) ForNamespaceMatcher(matcher string) ListKeysBuilder {
	if err := namespace.ValidateNamespaceMatcher(matcher); err != nil {
		return l.addError(err)
	}
	l.NamespaceMatchers = []string{matcher}
	return l
}

func (l *listKeysFilters) ForNamespaceMatchers(matchers []string) ListKeysBuilder {
	for _, matcher := range matchers {
		if err := namespace.ValidateNamespaceMatcher(matcher); err != nil {
			return l.addError(err)
		}
	}
	l.NamespaceMatchers = matchers
	return l
}

func (l *listKeysFilters) OrderBy(field KeyOrderByField, by pagination.OrderBy) ListKeysBuilder {
	l.OrderByFieldVal = &field
	l.OrderByVal = &by
	return l
}

func (l *listKeysFilters) IncludeDeleted() ListKeysBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listKeysFilters) ForLabelSelector(selector string) ListKeysBuilder {
	l.LabelSelectorVal = &selector
	return l
}

func (l *listKeysFilters) FromCursor(ctx context.Context, cursor string) (ListKeysExecutor, error) {
	s := l.s
	parsed, err := pagination.ParseCursor[listKeysFilters](ctx, s.cursorEncryptor, cursor)
	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.s = s

	return l, nil
}

func (l *listKeysFilters) applyRestrictions(ctx context.Context) sq.SelectBuilder {
	q := l.s.sq.
		Select(util.ToPtr(Key{}).cols()...).
		From(KeysTable)

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

func (l *listKeysFilters) fetchPage(ctx context.Context) pagination.PageResult[Key] {
	var err error

	if err = l.Errors.ErrorOrNil(); err != nil {
		return pagination.PageResult[Key]{Error: err}
	}

	rows, err := l.applyRestrictions(ctx).
		RunWith(l.s.db).
		Query()
	if err != nil {
		return pagination.PageResult[Key]{Error: err}
	}
	defer rows.Close()

	var results []Key
	for rows.Next() {
		var r Key
		err := rows.Scan(r.fields()...)
		if err != nil {
			return pagination.PageResult[Key]{Error: err}
		}
		results = append(results, r)
	}

	l.Offset = l.Offset + uint64(len(results)) - 1

	cursor := ""
	hasMore := uint64(len(results)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.cursorEncryptor, l)
		if err != nil {
			return pagination.PageResult[Key]{Error: err}
		}
	}

	return pagination.PageResult[Key]{
		HasMore: hasMore,
		Results: results[:util.MinUint64(l.LimitVal, uint64(len(results)))],
		Cursor:  cursor,
	}
}

func (l *listKeysFilters) FetchPage(ctx context.Context) pagination.PageResult[Key] {
	return l.fetchPage(ctx)
}

func (l *listKeysFilters) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[Key]) error {
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

func (s *service) ListKeysBuilder() ListKeysBuilder {
	return &listKeysFilters{
		s:        s,
		LimitVal: 100,
	}
}

func (s *service) ListKeysFromCursor(ctx context.Context, cursor string) (ListKeysExecutor, error) {
	b := &listKeysFilters{
		s:        s,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}

// EnumerateKeysInDependencyOrder walks all non-deleted keys in breadth-first order rooted at
// the unencrypted root key. Keys form a tree: the root key has nil EncryptedKeyData, while every
// other key's EncryptedKeyData.ID references the DEK that wrapped its key data. The DEK's key_id
// points to the parent key.
func (s *service) EnumerateKeysInDependencyOrder(
	ctx context.Context,
	callback func(keys []*Key, depth int) (keepGoing pagination.KeepGoing, err error),
) ([]*Key, error) {
	rows, err := s.sq.
		Select(util.ToPtr(Key{}).cols()...).
		From(KeysTable).
		Where(sq.Eq{"deleted_at": nil}).
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, fmt.Errorf("failed to query keys: %w", err)
	}

	allKeys := make(map[apid.ID]*Key)
	for rows.Next() {
		var ek Key
		if err := rows.Scan(ek.fields()...); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan key: %w", err)
		}
		allKeys[ek.Id] = &ek
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("failed to query keys: %w", err)
	}
	rows.Close()

	if len(allKeys) == 0 {
		return nil, nil
	}

	dekRows, err := s.sq.
		Select("id", "key_id").
		From(DataEncryptionKeysTable).
		Where(sq.Eq{"deleted_at": nil}).
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, fmt.Errorf("failed to query data encryption keys: %w", err)
	}

	dekToKey := make(map[apid.ID]apid.ID)
	for dekRows.Next() {
		var dekID, keyID apid.ID
		if err := dekRows.Scan(&dekID, &keyID); err != nil {
			dekRows.Close()
			return nil, fmt.Errorf("failed to scan data encryption key: %w", err)
		}
		dekToKey[dekID] = keyID
	}
	if err := dekRows.Err(); err != nil {
		dekRows.Close()
		return nil, fmt.Errorf("failed to query data encryption keys: %w", err)
	}
	dekRows.Close()

	// Build parent map and find root(s), collecting orphans.
	parentOf := make(map[apid.ID]apid.ID)
	var roots []*Key
	var orphans []*Key

	for _, ek := range allKeys {
		if ek.EncryptedKeyData == nil || ek.EncryptedKeyData.IsZero() {
			roots = append(roots, ek)
		} else {
			parentKeyID, ok := dekToKey[ek.EncryptedKeyData.ID]
			if !ok || allKeys[parentKeyID] == nil {
				orphans = append(orphans, ek)
			} else {
				parentOf[ek.Id] = parentKeyID
			}
		}
	}

	// Build children map
	children := make(map[apid.ID][]*Key)
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

		var nextLevel []*Key
		for _, ek := range currentLevel {
			nextLevel = append(nextLevel, children[ek.Id]...)
		}

		currentLevel = nextLevel
		depth++
	}

	return orphans, nil
}

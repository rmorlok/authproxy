package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type ActorOrderByField string

const (
	ActorOrderByCreatedAt  ActorOrderByField = "created_at"
	ActorOrderByUpdatedAt  ActorOrderByField = "updated_at"
	ActorOrderByNamespace  ActorOrderByField = "namespace"
	ActorOrderByExternalId ActorOrderByField = "external_id"
	ActorOrderByDeletedAt  ActorOrderByField = "deleted_at"
)

// Permissions is a custom type for a slice of permissions. The values are serlized to json.
type Permissions []aschema.Permission

// Value implements the driver.Valuer interface for Permissions
func (p Permissions) Value() (driver.Value, error) {
	if len(p) == 0 {
		return nil, nil
	}

	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements the sql.Scanner interface for Permissions
func (p *Permissions) Scan(value interface{}) error {
	if value == nil {
		*p = nil
		return nil
	}

	switch v := value.(type) {
	case string:
		return json.Unmarshal([]byte(v), p)
	case []byte:
		return json.Unmarshal(v, p)
	default:
		return fmt.Errorf("cannot convert %T to Permissions", value)
	}
}

// IsValidActorOrderByField checks if the given value is a valid ActorOrderByField.
func IsValidActorOrderByField[T string | ActorOrderByField](field T) bool {
	switch ActorOrderByField(field) {
	case ActorOrderByCreatedAt,
		ActorOrderByUpdatedAt,
		ActorOrderByNamespace,
		ActorOrderByExternalId,
		ActorOrderByDeletedAt:
		return true
	default:
		return false
	}
}

func init() {
	RegisterEncryptedField(EncryptedFieldRegistration{
		Table:          ActorTable,
		PrimaryKeyCols: []string{"id"},
		EncryptedCols:  []string{"encrypted_key"},
		NamespaceCol:   "namespace",
	})
}

const ActorTable = "actors"

// Actor is some entity taking action within the system.
type Actor struct {
	Id           apid.ID
	Namespace    string
	ExternalId   string
	Permissions  Permissions
	Labels       Labels
	Annotations  Annotations
	EncryptedKey *encfield.EncryptedField
	CreatedAt    time.Time
	UpdatedAt    time.Time
	EncryptedAt  *time.Time
	DeletedAt    *time.Time
}

func (a *Actor) GetExternalId() string {
	return a.ExternalId
}

func (a *Actor) GetPermissions() []aschema.Permission {
	return a.Permissions
}

// CanSelfSign returns true if this actor has an encrypted key and can self-sign requests
func (a *Actor) CanSelfSign() bool {
	if a == nil {
		return false
	}
	return a.EncryptedKey != nil
}

func (a *Actor) cols() []string {
	return []string{
		"id",
		"namespace",
		"external_id",
		"permissions",
		"labels",
		"annotations",
		"encrypted_key",
		"created_at",
		"updated_at",
		"encrypted_at",
		"deleted_at",
	}
}

func (a *Actor) fields() []any {
	return []any{
		&a.Id,
		&a.Namespace,
		&a.ExternalId,
		&a.Permissions,
		&a.Labels,
		&a.Annotations,
		&a.EncryptedKey,
		&a.CreatedAt,
		&a.UpdatedAt,
		&a.EncryptedAt,
		&a.DeletedAt,
	}
}

func (a *Actor) values() []any {
	return []any{
		a.Id,
		a.Namespace,
		a.ExternalId,
		a.Permissions,
		a.Labels,
		a.Annotations,
		a.EncryptedKey,
		a.CreatedAt,
		a.UpdatedAt,
		a.EncryptedAt,
		a.DeletedAt,
	}
}

func (a *Actor) setFromData(d IActorData) {
	a.Namespace = d.GetNamespace()
	a.ExternalId = d.GetExternalId()
	a.Permissions = d.GetPermissions()
	a.Labels = d.GetLabels()

	// Annotations follow PATCH semantics: a nil map leaves existing annotations
	// unchanged so server-side metadata is not wiped by callers (e.g. JWT auth)
	// that don't carry annotations.
	if annotations := d.GetAnnotations(); annotations != nil {
		a.Annotations = annotations
	}

	// Handle extended fields via type assertion
	if extended, ok := d.(IActorDataExtended); ok {
		a.EncryptedKey = extended.GetEncryptedKey()
	}
}

func (a *Actor) sameAsData(d IActorData) bool {
	// Compare labels — only the user portion. apxy/ labels are system-managed
	// and never appear in IActorData, so excluding them avoids spurious updates.
	aUserLabels, _ := SplitUserAndApxyLabels(a.Labels)
	dLabels := d.GetLabels()

	if len(aUserLabels) != len(dLabels) {
		return false
	}
	for k, v := range aUserLabels {
		if dv, ok := dLabels[k]; !ok || dv != v {
			return false
		}
	}

	// Compare annotations only when the caller provided them (matches the
	// PATCH semantics in setFromData).
	if dAnnotations := d.GetAnnotations(); dAnnotations != nil {
		if len(a.Annotations) != len(dAnnotations) {
			return false
		}
		for k, v := range a.Annotations {
			if dv, ok := dAnnotations[k]; !ok || dv != v {
				return false
			}
		}
	}

	basicMatch := a.GetNamespace() == d.GetNamespace() &&
		a.ExternalId == d.GetExternalId() &&
		slices.EqualFunc(a.Permissions, d.GetPermissions(), func(p1, p2 aschema.Permission) bool { return p1.Equal(p2) })

	if !basicMatch {
		return false
	}

	// Handle extended fields via type assertion
	if extended, ok := d.(IActorDataExtended); ok {
		if !a.EncryptedKey.Equal(extended.GetEncryptedKey()) {
			return false
		}
	}

	return true
}

func (a *Actor) GetId() apid.ID {
	return a.Id
}

func (a *Actor) GetNamespace() string {
	return a.Namespace
}

func (a *Actor) GetLabels() map[string]string {
	return a.Labels
}

func (a *Actor) GetAnnotations() map[string]string {
	return a.Annotations
}

func (a *Actor) GetEncryptedKey() *encfield.EncryptedField {
	return a.EncryptedKey
}

func (a *Actor) normalize() {
	// No actions currently
}

func (a *Actor) validate() error {
	result := &multierror.Error{}

	if a.Id == apid.Nil {
		result = multierror.Append(result, errors.New("actor id is empty"))
	}

	if err := a.Id.ValidatePrefix(apid.PrefixActor); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid actor id: %w", err))
	}

	if err := ValidateNamespacePath(a.Namespace); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid actor namespace: %w", err))
	}

	if a.ExternalId == "" {
		result = multierror.Append(result, errors.New("actor external id is empty"))
	}

	for i, p := range a.Permissions {
		err := p.Validate()
		if err != nil {
			result = multierror.Append(result, fmt.Errorf("actor permission %d is invalid: %w", i, err))
		}
	}

	if err := a.Labels.Validate(); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid actor labels: %w", err))
	}

	if err := a.Annotations.Validate(); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid actor annotations: %w", err))
	}

	return result.ErrorOrNil()
}

var _ IActorData = (*Actor)(nil)
var _ IActorDataExtended = (*Actor)(nil)

func (s *service) GetActor(ctx context.Context, id apid.ID) (*Actor, error) {
	var result Actor
	err := s.sq.
		Select(result.cols()...).
		From(ActorTable).
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

func (s *service) GetActorByExternalId(ctx context.Context, namespace, externalId string) (*Actor, error) {
	var result Actor
	err := s.sq.
		Select(result.cols()...).
		From(ActorTable).
		Where(sq.Eq{
			"namespace":   namespace,
			"external_id": externalId,
			"deleted_at":  nil,
		}).
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

func (s *service) CreateActor(ctx context.Context, a *Actor) error {
	if a == nil {
		return errors.New("actor is nil")
	}

	a.normalize()

	validationErr := a.validate()
	if validationErr != nil {
		return validationErr
	}

	return s.transaction(func(tx *sql.Tx) error {
		var count int64
		err := s.sq.
			Select("COUNT(*)").
			From(ActorTable).
			Where(sq.Or{
				sq.Eq{"id": a.Id},
				sq.Eq{
					"namespace":   a.Namespace,
					"external_id": a.ExternalId,
				},
			}).
			RunWith(tx).
			QueryRow().
			Scan(&count)
		if err != nil {
			return err
		}

		if count > 0 {
			return fmt.Errorf("actor already exists: %w", ErrDuplicate)
		}

		err = s.sq.
			Select("COUNT(*)").
			From(NamespacesTable).
			Where(sq.Or{
				sq.Eq{"path": a.Namespace},
			}).
			RunWith(tx).
			QueryRow().
			Scan(&count)
		if err != nil {
			return err
		}

		if count == 0 {
			return fmt.Errorf("actor namespace does not exist: %w", ErrNamespaceDoesNotExist)
		}

		nsLabels, err := s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
			"path":       a.Namespace,
			"deleted_at": nil,
		})
		if err != nil {
			return err
		}

		cpy := *a
		cpy.Labels = ApplyParentCarryForward(
			cpy.Labels,
			ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels},
		)
		cpy.Labels = InjectSelfImplicitLabels(cpy.Id, cpy.Namespace, cpy.Labels)
		now := apctx.GetClock(ctx).Now()
		cpy.CreatedAt = now
		cpy.UpdatedAt = now

		result, err := s.sq.
			Insert(ActorTable).
			Columns(cpy.cols()...).
			Values(cpy.values()...).
			RunWith(tx).
			Exec()
		if err != nil {
			return err
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		if affected == 0 {
			return errors.New("failed to create actor; no rows inserted")
		}

		return nil
	})
}

func (s *service) UpsertActor(ctx context.Context, d IActorData) (*Actor, error) {
	if d == nil {
		return nil, errors.New("actor is nil")
	}

	var lookupCond sq.Eq

	if d.GetId() != apid.Nil {
		lookupCond = sq.Eq{"id": d.GetId()}
	} else {
		lookupCond = sq.Eq{
			"namespace":   d.GetNamespace(),
			"external_id": d.GetExternalId(),
		}

		// This is covered in validation, but cover here to prevent any sort of lookup against an invalid id
		if d.GetExternalId() == "" {
			return nil, errors.New("actor external id is empty")
		}

		if err := ValidateNamespacePath(d.GetNamespace()); err != nil {
			return nil, fmt.Errorf("invalid actor namespace: %w", err)
		}
	}

	var result *Actor

	err := s.transaction(func(tx *sql.Tx) error {
		var existingActor Actor
		err := s.sq.
			Select(existingActor.cols()...).
			From(ActorTable).
			Where(lookupCond).
			RunWith(tx).
			QueryRow().
			Scan(existingActor.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Actor does not exist. Create a new actor.
				now := apctx.GetClock(ctx).Now()
				idGen := apctx.GetIdGenerator(ctx)

				id := d.GetId()
				if id == apid.Nil {
					id = idGen.New(apid.PrefixActor)
				}
				newActor := Actor{
					Id:        id,
					CreatedAt: now,
					UpdatedAt: now,
				}
				newActor.setFromData(d)
				newActor.normalize()
				nsLabels, err := s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
					"path":       newActor.Namespace,
					"deleted_at": nil,
				})
				if err != nil {
					return err
				}
				newActor.Labels = ApplyParentCarryForward(
					newActor.Labels,
					ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels},
				)
				newActor.Labels = InjectSelfImplicitLabels(newActor.Id, newActor.Namespace, newActor.Labels)
				validationErr := newActor.validate()
				if validationErr != nil {
					return validationErr
				}

				dbResult, err := s.sq.
					Insert(ActorTable).
					Columns(newActor.cols()...).
					Values(newActor.values()...).
					RunWith(tx).
					Exec()
				if err != nil {
					return fmt.Errorf("failed to create actor on upsert: %w", err)
				}

				affected, err := dbResult.RowsAffected()
				if err != nil {
					return fmt.Errorf("failed to create actor on upsert: %w", err)
				}

				if affected == 0 {
					return errors.New("failed to upsert actor; no rows inserted")
				}

				result = &newActor
				return nil
			}

			return err
		}

		if !existingActor.sameAsData(d) {
			// Preserve apxy/ system labels — setFromData replaces Labels
			// wholesale with the data's user portion.
			_, existingApxy := SplitUserAndApxyLabels(existingActor.Labels)
			existingActor.setFromData(d)
			existingActor.normalize()
			existingActor.Labels = MergeApxyAndUserLabels(existingActor.Labels, existingApxy)
			validationErr := existingActor.validate()
			if validationErr != nil {
				return validationErr
			}

			existingActor.UpdatedAt = apctx.GetClock(ctx).Now()

			dbResult, err := s.sq.
				Update(ActorTable).
				SetMap(util.ZipToMap(existingActor.cols(), existingActor.values())).
				Where(sq.Eq{"id": existingActor.Id}).
				RunWith(tx).
				Exec()
			if err != nil {
				return fmt.Errorf("failed to update existing actor: %w", err)
			}

			affected, err := dbResult.RowsAffected()
			if err != nil {
				return fmt.Errorf("failed to update existing actor: %w", err)
			}

			if affected == 0 {
				return errors.New("failed to update actor; no rows updated")
			}
		}

		result = &existingActor
		return nil
	})

	// Return any errors that occurred during the transaction
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *service) DeleteActor(ctx context.Context, id apid.ID) error {
	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(ActorTable).
		Set("updated_at", now).
		Set("deleted_at", now).
		Where(sq.Eq{"id": id}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to soft delete actor: %w", err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to soft delete actor: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	if affected > 1 {
		return fmt.Errorf("multiple actors were soft deleted: %w", ErrViolation)
	}

	return nil
}

type ListActorsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[*Actor]
	Enumerate(context.Context, pagination.EnumerateCallback[*Actor]) error
}

type ListActorsBuilder interface {
	ListActorsExecutor
	ForExternalId(externalId string) ListActorsBuilder
	ForNamespaceMatcher(matcher string) ListActorsBuilder
	ForNamespaceMatchers(matchers []string) ListActorsBuilder
	Limit(int32) ListActorsBuilder
	OrderBy(ActorOrderByField, pagination.OrderBy) ListActorsBuilder
	IncludeDeleted() ListActorsBuilder
	ForLabelSelector(selector string) ListActorsBuilder
}

type listActorsFilters struct {
	s                 *service            `json:"-"`
	LimitVal          uint64              `json:"limit"`
	Offset            uint64              `json:"offset"`
	OrderByFieldVal   *ActorOrderByField  `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy `json:"order_by"`
	IncludeDeletedVal bool                `json:"include_deleted,omitempty"`
	ExternalIdVal     *string             `json:"external_id,omitempty"`
	NamespaceMatchers []string            `json:"namespace_matchers,omitempty"`
	LabelSelectorVal  *string             `json:"label_selector,omitempty"`
	Errors            *multierror.Error   `json:"-"`
}

func (l *listActorsFilters) Limit(limit int32) ListActorsBuilder {
	l.LimitVal = uint64(limit)
	return l
}

func (l *listActorsFilters) OrderBy(field ActorOrderByField, by pagination.OrderBy) ListActorsBuilder {
	if IsValidActorOrderByField(field) {
		l.OrderByFieldVal = &field
		l.OrderByVal = &by
	}
	return l
}

func (l *listActorsFilters) IncludeDeleted() ListActorsBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listActorsFilters) ForExternalId(externalId string) ListActorsBuilder {
	l.ExternalIdVal = &externalId
	return l
}

func (l *listActorsFilters) addError(e error) ListActorsBuilder {
	l.Errors = multierror.Append(l.Errors, e)
	return l
}

func (l *listActorsFilters) ForNamespaceMatcher(matcher string) ListActorsBuilder {
	if err := ValidateNamespaceMatcher(matcher); err != nil {
		return l.addError(err)
	}
	l.NamespaceMatchers = []string{matcher}
	return l
}

func (l *listActorsFilters) ForNamespaceMatchers(matchers []string) ListActorsBuilder {
	for _, matcher := range matchers {
		if err := ValidateNamespaceMatcher(matcher); err != nil {
			return l.addError(err)
		}
	}
	l.NamespaceMatchers = matchers
	return l
}

func (l *listActorsFilters) ForLabelSelector(selector string) ListActorsBuilder {
	l.LabelSelectorVal = &selector
	return l
}

func (l *listActorsFilters) FromCursor(ctx context.Context, cursor string) (ListActorsExecutor, error) {
	s := l.s
	parsed, err := pagination.ParseCursor[listActorsFilters](ctx, s.cursorEncryptor, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.s = s

	return l, nil
}

func (l *listActorsFilters) applyRestrictions(ctx context.Context) sq.SelectBuilder {
	q := l.s.sq.
		Select(util.ToPtr(Actor{}).cols()...).
		From(ActorTable)

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

	if len(l.NamespaceMatchers) > 0 {
		q = restrictToNamespaceMatchers(q, "namespace", l.NamespaceMatchers)
	}

	if l.OrderByFieldVal != nil {
		q = q.OrderBy(fmt.Sprintf("%s %s", string(*l.OrderByFieldVal), l.OrderByVal.String()))
	}

	return q
}

func (l *listActorsFilters) fetchPage(ctx context.Context) pagination.PageResult[*Actor] {
	var err error

	if err = l.Errors.ErrorOrNil(); err != nil {
		return pagination.PageResult[*Actor]{Error: err}
	}

	rows, err := l.applyRestrictions(ctx).
		RunWith(l.s.db).
		Query()
	if err != nil {
		return pagination.PageResult[*Actor]{Error: err}
	}
	defer rows.Close()

	var results []*Actor
	for rows.Next() {
		var r Actor
		err := rows.Scan(r.fields()...)
		if err != nil {
			return pagination.PageResult[*Actor]{Error: err}
		}
		results = append(results, &r)
	}

	l.Offset = l.Offset + uint64(len(results)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := uint64(len(results)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.cursorEncryptor, l)
		if err != nil {
			return pagination.PageResult[*Actor]{Error: err}
		}
	}

	return pagination.PageResult[*Actor]{
		HasMore: hasMore,
		Results: results[:util.MinUint64(l.LimitVal, uint64(len(results)))],
		Cursor:  cursor,
	}
}

func (l *listActorsFilters) FetchPage(ctx context.Context) pagination.PageResult[*Actor] {
	return l.fetchPage(ctx)
}

func (l *listActorsFilters) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[*Actor]) error {
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

func (s *service) ListActorsBuilder() ListActorsBuilder {
	return &listActorsFilters{
		s:        s,
		LimitVal: 100,
	}
}

func (s *service) ListActorsFromCursor(ctx context.Context, cursor string) (ListActorsExecutor, error) {
	b := &listActorsFilters{
		s:        s,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}

// PutActorLabels adds or updates the specified labels on an actor within a single transaction.
// Existing labels not in the provided map are preserved.
func (s *service) PutActorLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Actor, error) {
	if id == apid.Nil {
		return nil, errors.New("actor id is required")
	}

	if len(labels) == 0 {
		return s.GetActor(ctx, id)
	}

	if err := ValidateUserLabels(labels); err != nil {
		return nil, fmt.Errorf("invalid labels: %w", err)
	}

	var result *Actor

	err := s.transaction(func(tx *sql.Tx) error {
		var actor Actor
		err := s.sq.
			Select(actor.cols()...).
			From(ActorTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(actor.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		mergedLabels, now, err := s.putLabelsInTableTx(ctx, tx, ActorTable, sq.Eq{"id": id, "deleted_at": nil}, labels)
		if err != nil {
			return err
		}

		actor.Labels = mergedLabels
		actor.UpdatedAt = now
		result = &actor
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// UpdateActorAnnotations replaces all annotations on an actor within a single transaction.
// If annotations is nil, all annotations are removed.
func (s *service) UpdateActorAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Actor, error) {
	if id == apid.Nil {
		return nil, errors.New("actor id is required")
	}

	if annotations != nil {
		if err := ValidateAnnotations(annotations); err != nil {
			return nil, fmt.Errorf("invalid annotations: %w", err)
		}
	}

	var result *Actor

	err := s.transaction(func(tx *sql.Tx) error {
		var actor Actor
		err := s.sq.
			Select(actor.cols()...).
			From(ActorTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(actor.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		now, err := s.updateAnnotationsInTableTx(ctx, tx, ActorTable, sq.Eq{"id": id, "deleted_at": nil}, Annotations(annotations))
		if err != nil {
			return err
		}

		actor.Annotations = annotations
		actor.UpdatedAt = now
		result = &actor
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// PutActorAnnotations adds or updates the specified annotations on an actor within a single transaction.
// Existing annotations not in the provided map are preserved.
func (s *service) PutActorAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Actor, error) {
	if id == apid.Nil {
		return nil, errors.New("actor id is required")
	}

	if len(annotations) == 0 {
		return s.GetActor(ctx, id)
	}

	if err := ValidateAnnotations(annotations); err != nil {
		return nil, fmt.Errorf("invalid annotations: %w", err)
	}

	var result *Actor

	err := s.transaction(func(tx *sql.Tx) error {
		var actor Actor
		err := s.sq.
			Select(actor.cols()...).
			From(ActorTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(actor.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		mergedAnnotations, now, err := s.putAnnotationsInTableTx(ctx, tx, ActorTable, sq.Eq{"id": id, "deleted_at": nil}, annotations)
		if err != nil {
			return err
		}

		actor.Annotations = mergedAnnotations
		actor.UpdatedAt = now
		result = &actor
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteActorAnnotations removes the specified annotation keys from an actor within a single transaction.
// Keys that don't exist are ignored.
func (s *service) DeleteActorAnnotations(ctx context.Context, id apid.ID, keys []string) (*Actor, error) {
	if id == apid.Nil {
		return nil, errors.New("actor id is required")
	}

	if len(keys) == 0 {
		return s.GetActor(ctx, id)
	}

	var result *Actor

	err := s.transaction(func(tx *sql.Tx) error {
		var actor Actor
		err := s.sq.
			Select(actor.cols()...).
			From(ActorTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(actor.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		remainingAnnotations, now, err := s.deleteAnnotationsInTableTx(ctx, tx, ActorTable, sq.Eq{"id": id, "deleted_at": nil}, keys)
		if err != nil {
			return err
		}

		actor.Annotations = remainingAnnotations
		actor.UpdatedAt = now
		result = &actor
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteActorLabels removes the specified label keys from an actor within a single transaction.
// Keys that don't exist are ignored.
func (s *service) DeleteActorLabels(ctx context.Context, id apid.ID, keys []string) (*Actor, error) {
	if id == apid.Nil {
		return nil, errors.New("actor id is required")
	}

	if len(keys) == 0 {
		return s.GetActor(ctx, id)
	}

	if err := ValidateUserLabelDeletionKeys(keys); err != nil {
		return nil, fmt.Errorf("invalid label keys: %w", err)
	}

	var result *Actor

	err := s.transaction(func(tx *sql.Tx) error {
		var actor Actor
		err := s.sq.
			Select(actor.cols()...).
			From(ActorTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(actor.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		remainingLabels, now, err := s.deleteLabelsInTableTx(ctx, tx, ActorTable, sq.Eq{"id": id, "deleted_at": nil}, keys)
		if err != nil {
			return err
		}

		actor.Labels = remainingLabels
		actor.UpdatedAt = now
		result = &actor
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

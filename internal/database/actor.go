package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type ActorOrderByField string

const (
	ActorOrderByCreatedAt  ActorOrderByField = "created_at"
	ActorOrderByUpdatedAt  ActorOrderByField = "updated_at"
	ActorOrderByNamespace  ActorOrderByField = "namespace"
	ActorOrderByEmail      ActorOrderByField = "email"
	ActorOrderByExternalId ActorOrderByField = "external_id"
	ActorOrderByAdmin      ActorOrderByField = "admin"
	ActorOrderBySuperAdmin ActorOrderByField = "super_admin"
	ActorOrderByDeletedAt  ActorOrderByField = "deleted_at"
)

// Permissions is a custom type for a slice of permissions. The values are serlized to json.
type Permissions []aschema.Permission

// Value implements the driver.Valuer interface for Permissions
func (p Permissions) Value() (driver.Value, error) {
	if len(p) == 0 {
		return nil, nil
	}

	return json.Marshal(p)
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
		ActorOrderByEmail,
		ActorOrderByExternalId,
		ActorOrderByAdmin,
		ActorOrderBySuperAdmin,
		ActorOrderByDeletedAt:
		return true
	default:
		return false
	}
}

const ActorTable = "actors"

// Actor is some entity taking action within the system.
type Actor struct {
	Id           uuid.UUID
	Namespace    string
	ExternalId   string
	Email        string
	Permissions  Permissions
	Labels       Labels
	EncryptedKey *string
	Admin        bool
	SuperAdmin   bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

func (a *Actor) GetExternalId() string {
	return a.ExternalId
}

func (a *Actor) GetPermissions() []aschema.Permission {
	return a.Permissions
}

func (a *Actor) GetEmail() string {
	return a.Email
}

func (a *Actor) cols() []string {
	return []string{
		"id",
		"namespace",
		"external_id",
		"email",
		"permissions",
		"labels",
		"encrypted_key",
		"admin",
		"super_admin",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

func (a *Actor) fields() []any {
	return []any{
		&a.Id,
		&a.Namespace,
		&a.ExternalId,
		&a.Email,
		&a.Permissions,
		&a.Labels,
		&a.EncryptedKey,
		&a.Admin,
		&a.SuperAdmin,
		&a.CreatedAt,
		&a.UpdatedAt,
		&a.DeletedAt,
	}
}

func (a *Actor) values() []any {
	return []any{
		a.Id,
		a.Namespace,
		a.ExternalId,
		a.Email,
		a.Permissions,
		a.Labels,
		a.EncryptedKey,
		a.Admin,
		a.SuperAdmin,
		a.CreatedAt,
		a.UpdatedAt,
		a.DeletedAt,
	}
}

func (a *Actor) setFromData(d IActorData) {
	a.Namespace = d.GetNamespace()
	a.ExternalId = d.GetExternalId()
	a.Email = d.GetEmail()
	a.Permissions = d.GetPermissions()
	a.Admin = d.IsAdmin()
	a.SuperAdmin = d.IsSuperAdmin()
	a.Labels = d.GetLabels()

	// Handle extended fields via type assertion
	if extended, ok := d.(IActorDataExtended); ok {
		a.EncryptedKey = extended.GetEncryptedKey()
	}
}

func (a *Actor) sameAsData(d IActorData) bool {
	// Compare labels
	dLabels := d.GetLabels()

	if len(a.Labels) != len(dLabels) {
		return false
	}
	for k, v := range a.Labels {
		if dv, ok := dLabels[k]; !ok || dv != v {
			return false
		}
	}

	basicMatch := a.GetNamespace() == d.GetNamespace() &&
		a.ExternalId == d.GetExternalId() &&
		a.Email == d.GetEmail() &&
		slices.EqualFunc(a.Permissions, d.GetPermissions(), func(p1, p2 aschema.Permission) bool { return p1.Equal(p2) }) &&
		a.Admin == d.IsAdmin() &&
		a.SuperAdmin == d.IsSuperAdmin()

	if !basicMatch {
		return false
	}

	// Handle extended fields via type assertion
	if extended, ok := d.(IActorDataExtended); ok {
		// Compare encrypted key
		dEncryptedKey := extended.GetEncryptedKey()
		if (a.EncryptedKey == nil) != (dEncryptedKey == nil) {
			return false
		}
		if a.EncryptedKey != nil && dEncryptedKey != nil && *a.EncryptedKey != *dEncryptedKey {
			return false
		}
	}

	return true
}

func (a *Actor) GetId() uuid.UUID {
	return a.Id
}

func (a *Actor) GetNamespace() string {
	return a.Namespace
}

func (a *Actor) GetLabels() map[string]string {
	return a.Labels
}

func (a *Actor) GetEncryptedKey() *string {
	return a.EncryptedKey
}

// IsAdmin is a helper to wrap the Admin attribute
func (a *Actor) IsAdmin() bool {
	if a == nil {
		return false
	}

	return a.Admin
}

// IsSuperAdmin is a helper to wrap the SuperAdmin attribute
func (a *Actor) IsSuperAdmin() bool {
	if a == nil {
		return false
	}

	return a.SuperAdmin
}

// IsNormalActor indicates that an actor is not an admin or superadmin
func (a *Actor) IsNormalActor() bool {
	if a == nil {
		// actors default to normal
		return true
	}

	return !a.IsSuperAdmin() && !a.IsAdmin()
}

func (a *Actor) normalize() {
	// No actions currently
}

func (a *Actor) validate() error {
	result := &multierror.Error{}

	if a.Id == uuid.Nil {
		result = multierror.Append(result, errors.New("actor id is empty"))
	}

	if err := ValidateNamespacePath(a.Namespace); err != nil {
		result = multierror.Append(result, errors.Wrap(err, "invalid actor namespace"))
	}

	if a.ExternalId == "" {
		result = multierror.Append(result, errors.New("actor external id is empty"))
	}

	if a.Admin && !strings.HasPrefix(a.ExternalId, "admin/") {
		result = multierror.Append(result, errors.New("admin external id is not correctly formatted"))
	}

	if strings.HasPrefix(a.ExternalId, "admin/") && !a.Admin {
		result = multierror.Append(result, errors.New("normal actor cannot have admin/ Id prefix"))
	}

	if a.SuperAdmin && !strings.HasPrefix(a.ExternalId, "superadmin/") {
		result = multierror.Append(result, errors.New("super admin Id is not correctly formatted"))
	}

	if strings.HasPrefix(a.ExternalId, "superadmin/") && !a.SuperAdmin {
		result = multierror.Append(result, errors.New("normal actor cannot have superadmin/ Id prefix"))
	}

	for i, p := range a.Permissions {
		err := p.Validate()
		if err != nil {
			result = multierror.Append(result, fmt.Errorf("actor permission %d is invalid: %w", i, err))
		}
	}

	if err := a.Labels.Validate(); err != nil {
		result = multierror.Append(result, errors.Wrap(err, "invalid actor labels"))
	}

	return result.ErrorOrNil()
}

var _ IActorData = (*Actor)(nil)
var _ IActorDataExtended = (*Actor)(nil)

func (s *service) GetActor(ctx context.Context, id uuid.UUID) (*Actor, error) {
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
	err := sq.
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
			return errors.New("actor already exists")
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
			return errors.New("actor namespace does not exist")
		}

		cpy := *a
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

	if d.GetId() != uuid.Nil {
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
			return nil, errors.Wrap(err, "invalid actor namespace")
		}
	}

	var result *Actor

	err := s.transaction(func(tx *sql.Tx) error {
		var existingActor Actor
		err := s.sq.
			Select(existingActor.cols()...).
			From(ActorTable).
			Where(lookupCond).
			RunWith(s.db).
			QueryRow().
			Scan(existingActor.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Actor does not exist. Create a new actor.
				now := apctx.GetClock(ctx).Now()
				uuidGen := apctx.GetUuidGenerator(ctx)

				id := d.GetId()
				if id == uuid.Nil {
					id = uuidGen.New()
				}
				newActor := Actor{
					Id:        id,
					CreatedAt: now,
					UpdatedAt: now,
				}
				newActor.setFromData(d)
				newActor.normalize()
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
					return errors.Wrap(err, "failed to create actor on upsert")
				}

				affected, err := dbResult.RowsAffected()
				if err != nil {
					return errors.Wrap(err, "failed to create actor on upsert")
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
			existingActor.setFromData(d)
			existingActor.normalize()
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
				return errors.Wrap(err, "failed to update existing actor")
			}

			affected, err := dbResult.RowsAffected()
			if err != nil {
				return errors.Wrap(err, "failed to update existing actor")
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

func (s *service) DeleteActor(ctx context.Context, id uuid.UUID) error {
	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(ActorTable).
		Set("updated_at", now).
		Set("deleted_at", now).
		Where(sq.Eq{"id": id}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return errors.Wrap(err, "failed to soft delete actor")
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to soft delete actor")
	}

	if affected == 0 {
		return ErrNotFound
	}

	if affected > 1 {
		return errors.Wrap(ErrViolation, "multiple actors were soft deleted")
	}

	return nil
}

type ListActorsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[*Actor]
	Enumerate(context.Context, func(pagination.PageResult[*Actor]) (keepGoing bool, err error)) error
}

type ListActorsBuilder interface {
	ListActorsExecutor
	ForExternalId(externalId string) ListActorsBuilder
	ForEmail(email string) ListActorsBuilder
	ForIsAdmin(isAdmin bool) ListActorsBuilder
	ForIsSuperAdmin(isSuperAdmin bool) ListActorsBuilder
	ForNamespaceMatcher(matcher string) ListActorsBuilder
	ForNamespaceMatchers(matchers []string) ListActorsBuilder
	ForLabelExists(key string) ListActorsBuilder
	ForLabelEquals(key, value string) ListActorsBuilder
	Limit(int32) ListActorsBuilder
	OrderBy(ActorOrderByField, pagination.OrderBy) ListActorsBuilder
	IncludeDeleted() ListActorsBuilder
}

type listActorsFilters struct {
	s                 *service            `json:"-"`
	LimitVal          uint64              `json:"limit"`
	Offset            uint64              `json:"offset"`
	OrderByFieldVal   *ActorOrderByField  `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy `json:"order_by"`
	IncludeDeletedVal bool                `json:"include_deleted,omitempty"`
	ExternalIdVal     *string             `json:"external_id,omitempty"`
	EmailVal          *string             `json:"email,omitempty"`
	IsAdminVal        *bool               `json:"is_admin,omitempty"`
	IsSuperAdminVal   *bool               `json:"is_super_admin,omitempty"`
	NamespaceMatchers []string            `json:"namespace_matchers,omitempty"`
	LabelExistsKey    *string             `json:"label_exists_key,omitempty"`
	LabelEqualsKey    *string             `json:"label_equals_key,omitempty"`
	LabelEqualsValue  *string             `json:"label_equals_value,omitempty"`
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

func (l *listActorsFilters) ForEmail(email string) ListActorsBuilder {
	l.EmailVal = &email
	return l
}

func (l *listActorsFilters) ForIsAdmin(isAdmin bool) ListActorsBuilder {
	l.IsAdminVal = &isAdmin
	return l
}

func (l *listActorsFilters) ForIsSuperAdmin(isSuperAdmin bool) ListActorsBuilder {
	l.IsSuperAdminVal = &isSuperAdmin
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

func (l *listActorsFilters) ForLabelExists(key string) ListActorsBuilder {
	if err := ValidateLabelKey(key); err != nil {
		return l.addError(err)
	}
	l.LabelExistsKey = &key
	return l
}

func (l *listActorsFilters) ForLabelEquals(key, value string) ListActorsBuilder {
	if err := ValidateLabelKey(key); err != nil {
		return l.addError(err)
	}
	if err := ValidateLabelValue(value); err != nil {
		return l.addError(err)
	}
	l.LabelEqualsKey = &key
	l.LabelEqualsValue = &value
	return l
}

func (l *listActorsFilters) FromCursor(ctx context.Context, cursor string) (ListActorsExecutor, error) {
	s := l.s
	parsed, err := pagination.ParseCursor[listActorsFilters](ctx, s.secretKey, cursor)

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

	// Filter by label existence (key exists in labels JSON)
	if l.LabelExistsKey != nil {
		// Use json_extract to check if the key exists in the labels JSON
		// Use bracket notation for keys with special characters like slashes and dots
		jsonPath := fmt.Sprintf("$.\"%s\"", *l.LabelExistsKey)
		q = q.Where(sq.Expr("json_extract(labels, ?) IS NOT NULL", jsonPath))
	}

	// Filter by label key-value equality
	if l.LabelEqualsKey != nil && l.LabelEqualsValue != nil {
		// Use bracket notation for keys with special characters like slashes and dots
		jsonPath := fmt.Sprintf("$.\"%s\"", *l.LabelEqualsKey)
		q = q.Where(sq.Expr("json_extract(labels, ?) = ?", jsonPath, *l.LabelEqualsValue))
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
		cursor, err = pagination.MakeCursor(ctx, l.s.secretKey, l)
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

func (l *listActorsFilters) Enumerate(ctx context.Context, callback func(pagination.PageResult[*Actor]) (keepGoing bool, err error)) error {
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

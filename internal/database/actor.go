package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/jwt"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type ActorOrderByField string

const (
	ActorOrderByCreatedAt  ActorOrderByField = "created_at"
	ActorOrderByUpdatedAt  ActorOrderByField = "updated_at"
	ActorOrderByEmail      ActorOrderByField = "email"
	ActorOrderByExternalId ActorOrderByField = "external_id"
	ActorOrderByAdmin      ActorOrderByField = "admin"
	ActorOrderBySuperAdmin ActorOrderByField = "super_admin"
	ActorOrderByDeletedAt  ActorOrderByField = "deleted_at"
)

// IsValidActorOrderByField checks if the given value is a valid ActorOrderByField.
func IsValidActorOrderByField[T string | ActorOrderByField](field T) bool {
	switch ActorOrderByField(field) {
	case ActorOrderByCreatedAt,
		ActorOrderByUpdatedAt,
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
	ID         uuid.UUID  `gorm:"column:id;primarykey"`
	ExternalId string     `gorm:"column:external_id;unique,index"`
	Email      string     `gorm:"column:email;index"`
	Admin      bool       `gorm:"column:admin"`
	SuperAdmin bool       `gorm:"column:super_admin"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at"`
	DeletedAt  *time.Time `gorm:"column:deleted_at;index"`
}

func (a *Actor) cols() []string {
	return []string{
		"id",
		"external_id",
		"email",
		"admin",
		"super_admin",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

func (a *Actor) fields() []any {
	return []any{
		&a.ID,
		&a.ExternalId,
		&a.Email,
		&a.Admin,
		&a.SuperAdmin,
		&a.CreatedAt,
		&a.UpdatedAt,
		&a.DeletedAt,
	}
}

func (a *Actor) values() []any {
	return []any{
		a.ID,
		a.ExternalId,
		a.Email,
		a.Admin,
		a.SuperAdmin,
		a.CreatedAt,
		a.UpdatedAt,
		a.DeletedAt,
	}
}

func (a *Actor) setFromJwt(ja *jwt.Actor) {
	a.ExternalId = ja.ID
	a.Email = ja.Email
	a.Admin = ja.IsAdmin()
	a.SuperAdmin = ja.IsSuperAdmin()
}

func (a *Actor) sameAsJwt(ja *jwt.Actor) bool {
	return a.ExternalId == ja.ID &&
		a.Email == ja.Email &&
		a.Admin == ja.IsAdmin() &&
		a.SuperAdmin == ja.IsSuperAdmin()
}

func (a *Actor) ToJwtActor() jwt.Actor {
	return jwt.Actor{
		ID:         a.ExternalId,
		Email:      a.Email,
		Admin:      a.Admin,
		SuperAdmin: a.SuperAdmin,
	}
}

func (a *Actor) GetID() uuid.UUID {
	return a.ID
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

func (a *Actor) validate() error {
	if a.ID == uuid.Nil {
		return errors.New("actor id is empty")
	}

	if a.ExternalId == "" {
		return errors.New("actor external id is empty")
	}

	if a.Admin && !strings.HasPrefix(a.ExternalId, "admin/") {
		return errors.New("admin external id is not correctly formatted")
	}

	if strings.HasPrefix(a.ExternalId, "admin/") && !a.Admin {
		return errors.New("normal actor cannot have admin/ ID prefix")
	}

	if a.SuperAdmin && !strings.HasPrefix(a.ExternalId, "superadmin/") {
		return errors.New("super admin ID is not correctly formatted")
	}

	if strings.HasPrefix(a.ExternalId, "superadmin/") && !a.SuperAdmin {
		return errors.New("normal actor cannot have superadmin/ ID prefix")
	}

	return nil
}

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

func (s *service) GetActorByExternalId(ctx context.Context, externalId string) (*Actor, error) {
	var result Actor
	err := sq.
		Select(result.cols()...).
		From(ActorTable).
		Where(sq.Eq{"external_id": externalId, "deleted_at": nil}).
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
				sq.Eq{"id": a.ID},
				sq.Eq{"external_id": a.ExternalId},
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
			return errors.New("no rows inserted")
		}

		return nil
	})
}

func (s *service) UpsertActor(ctx context.Context, actor *jwt.Actor) (*Actor, error) {
	if actor == nil {
		return nil, errors.New("actor is nil")
	}

	// This is covered in validation, but cover here to prevent any sort of lookup against an invalid id
	if actor.ID == "" {
		return nil, errors.New("actor id is empty")
	}

	var result *Actor

	err := s.transaction(func(tx *sql.Tx) error {
		var existingActor Actor
		err := s.sq.
			Select(existingActor.cols()...).
			From(ActorTable).
			Where(sq.Eq{"external_id": actor.ID}).
			RunWith(s.db).
			QueryRow().
			Scan(existingActor.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Actor does not exist. Create a new actor.
				now := apctx.GetClock(ctx).Now()
				newActor := Actor{
					ID:        uuid.New(),
					CreatedAt: now,
					UpdatedAt: now,
				}
				newActor.setFromJwt(actor)
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

		if !existingActor.sameAsJwt(actor) {
			existingActor.setFromJwt(actor)
			validationErr := existingActor.validate()
			if validationErr != nil {
				return validationErr
			}

			existingActor.UpdatedAt = apctx.GetClock(ctx).Now()

			dbResult, err := s.sq.
				Update(ActorTable).
				SetMap(util.ZipToMap(existingActor.cols(), existingActor.values())).
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
		return errors.New("multiple actors were soft deleted")
	}

	return nil
}

type ListActorsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[Actor]
	Enumerate(context.Context, func(pagination.PageResult[Actor]) (keepGoing bool, err error)) error
}

type ListActorsBuilder interface {
	ListActorsExecutor
	ForExternalId(externalId string) ListActorsBuilder
	ForEmail(email string) ListActorsBuilder
	ForIsAdmin(isAdmin bool) ListActorsBuilder
	ForIsSuperAdmin(isSuperAdmin bool) ListActorsBuilder
	Limit(int32) ListActorsBuilder
	OrderBy(ActorOrderByField, pagination.OrderBy) ListActorsBuilder
	IncludeDeleted() ListActorsBuilder
}

type listActorsFilters struct {
	db                *service            `json:"-"`
	LimitVal          uint64              `json:"limit"`
	Offset            uint64              `json:"offset"`
	OrderByFieldVal   *ActorOrderByField  `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy `json:"order_by"`
	IncludeDeletedVal bool                `json:"include_deleted,omitempty"`
	ExternalIdVal     *string             `json:"external_id,omitempty"`
	EmailVal          *string             `json:"email,omitempty"`
	IsAdminVal        *bool               `json:"is_admin,omitempty"`
	IsSuperAdminVal   *bool               `json:"is_super_admin,omitempty"`
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

func (l *listActorsFilters) FromCursor(ctx context.Context, cursor string) (ListActorsExecutor, error) {
	db := l.db
	parsed, err := pagination.ParseCursor[listActorsFilters](ctx, db.secretKey, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.db = db

	return l, nil
}

func (l *listActorsFilters) applyRestrictions(ctx context.Context) sq.SelectBuilder {
	q := l.db.sq.
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

	if l.OrderByFieldVal != nil {
		q = q.OrderBy(fmt.Sprintf("%s %s", string(*l.OrderByFieldVal), l.OrderByVal.String()))
	}

	return q
}

func (l *listActorsFilters) fetchPage(ctx context.Context) pagination.PageResult[Actor] {
	var err error

	rows, err := l.applyRestrictions(ctx).
		RunWith(l.db.db).
		Query()
	if err != nil {
		return pagination.PageResult[Actor]{Error: err}
	}
	defer rows.Close()

	var actors []Actor
	for rows.Next() {
		var a Actor
		err := rows.Scan(a.fields()...)
		if err != nil {
			return pagination.PageResult[Actor]{Error: err}
		}
		actors = append(actors, a)
	}

	l.Offset = l.Offset + uint64(len(actors)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := uint64(len(actors)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.db.secretKey, l)
		if err != nil {
			return pagination.PageResult[Actor]{Error: err}
		}
	}

	return pagination.PageResult[Actor]{
		HasMore: hasMore,
		Results: actors[:util.MinUint64(l.LimitVal, uint64(len(actors)))],
		Cursor:  cursor,
	}
}

func (l *listActorsFilters) FetchPage(ctx context.Context) pagination.PageResult[Actor] {
	return l.fetchPage(ctx)
}

func (l *listActorsFilters) Enumerate(ctx context.Context, callback func(pagination.PageResult[Actor]) (keepGoing bool, err error)) error {
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
		db:       s,
		LimitVal: 100,
	}
}

func (s *service) ListActorsFromCursor(ctx context.Context, cursor string) (ListActorsExecutor, error) {
	b := &listActorsFilters{
		db:       s,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}

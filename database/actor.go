package database

import (
	"context"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/jwt"
	"github.com/rmorlok/authproxy/util"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type ActorOrderByField string

const (
	ActorOrderByCreatedAt  ActorOrderByField = "created_at"
	ActorOrderByUpdatedAt  ActorOrderByField = "updated_at"
	ActorOrderByEmail      ActorOrderByField = "email"
	ActorOrderByExternalId ActorOrderByField = "external_id"
	ActorOrderByDeletedAt  ActorOrderByField = "deleted_at"
)

// Actor is some entity taking action within the system.
type Actor struct {
	ID         uuid.UUID      `gorm:"column:id;primarykey"`
	ExternalId string         `gorm:"column:external_id;unique,index"`
	Email      string         `gorm:"column:email;index"`
	Admin      bool           `gorm:"column:admin"`
	SuperAdmin bool           `gorm:"column:super_admin"`
	CreatedAt  time.Time      `gorm:"column:created_at"`
	UpdatedAt  time.Time      `gorm:"column:updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"column:deleted_at;index"`
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

func (db *gormDB) GetActor(ctx context.Context, id uuid.UUID) (*Actor, error) {
	sess := db.session(ctx)

	var a Actor
	result := sess.First(&a, id)
	if result.Error != nil {
		if errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &a, nil
}

func (db *gormDB) GetActorByExternalId(ctx context.Context, externalId string) (*Actor, error) {
	q := db.session(ctx)

	var a Actor
	result := q.Where("external_id = ?", externalId).First(&a)
	if result.Error != nil {
		if errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &a, nil
}

func (db *gormDB) CreateActor(ctx context.Context, a *Actor) error {
	if a == nil {
		return errors.New("actor is nil")
	}

	validationErr := a.validate()
	if validationErr != nil {
		return validationErr
	}

	return db.gorm.Transaction(func(tx *gorm.DB) error {
		var count int64
		err := tx.Model(&Actor{}).Where("id = ? OR external_id = ?", a.ID, a.ExternalId).Count(&count).Error
		if err != nil {
			return err
		}

		if count > 0 {
			return errors.New("actor already exists")
		}

		result := tx.Create(a)
		if result.Error != nil {
			return result.Error
		}

		return nil
	})
}

func (db *gormDB) UpsertActor(ctx context.Context, actor *jwt.Actor) (*Actor, error) {
	if actor == nil {
		return nil, errors.New("actor is nil")
	}

	// This is covered in validation, but cover here to prevent any sort of lookup against an invalid id
	if actor.ID == "" {
		return nil, errors.New("actor id is empty")
	}

	var result *Actor

	err := db.gorm.Transaction(func(tx *gorm.DB) error {
		var existingActor Actor
		err := tx.Where("external_id = ?", actor.ID).First(&existingActor).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				// Actor does not exist. Create new actor.
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

				if createErr := tx.Create(&newActor).Error; createErr != nil {
					return createErr
				}

				result = &newActor
				return nil
			}

			// Return the error if it wasn't a "record not found" error
			return err
		}

		// Actor exists and was loaded
		if !existingActor.sameAsJwt(actor) {
			existingActor.setFromJwt(actor)
			validationErr := existingActor.validate()
			if validationErr != nil {
				return validationErr
			}

			if updateErr := tx.Save(&existingActor).Error; updateErr != nil {
				return updateErr
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

type ListActorsExecutor interface {
	FetchPage(context.Context) PageResult[Actor]
	Enumerate(context.Context, func(PageResult[Actor]) (keepGoing bool, err error)) error
}

type ListActorsBuilder interface {
	ListActorsExecutor
	Limit(int32) ListActorsBuilder
	OrderBy(ActorOrderByField, OrderBy) ListActorsBuilder
	IncludeDeleted() ListActorsBuilder
}

type listActorsFilters struct {
	db                *gormDB            `json:"-"`
	LimitVal          int32              `json:"limit"`
	Offset            int32              `json:"offset"`
	OrderByFieldVal   *ActorOrderByField `json:"order_by_field"`
	OrderByVal        *OrderBy           `json:"order_by"`
	IncludeDeletedVal bool               `json:"include_deleted,omitempty"`
}

func (l *listActorsFilters) Limit(limit int32) ListActorsBuilder {
	l.LimitVal = limit
	return l
}

func (l *listActorsFilters) OrderBy(field ActorOrderByField, by OrderBy) ListActorsBuilder {
	l.OrderByFieldVal = &field
	l.OrderByVal = &by
	return l
}

func (l *listActorsFilters) IncludeDeleted() ListActorsBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listActorsFilters) FromCursor(ctx context.Context, cursor string) (ListActorsExecutor, error) {
	db := l.db
	parsed, err := parseCursor[listActorsFilters](ctx, db.secretKey, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.db = db

	return l, nil
}

func (l *listActorsFilters) applyRestrictions(ctx context.Context) *gorm.DB {
	q := l.db.session(ctx)

	if l.IncludeDeletedVal {
		q = q.Unscoped()
	}

	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	// Always limit to one more than limit to check if there are more records
	q = q.Limit(int(l.LimitVal + 1)).Offset(int(l.Offset))

	if l.OrderByFieldVal != nil {
		q.Order(clause.OrderByColumn{Column: clause.Column{Name: string(*l.OrderByFieldVal)}, Desc: true})
	}

	return q
}

func (l *listActorsFilters) fetchPage(ctx context.Context) PageResult[Actor] {
	var err error
	var actors []Actor
	result := l.applyRestrictions(ctx).Find(&actors)
	if result.Error != nil {
		return PageResult[Actor]{Error: result.Error}
	}

	l.Offset = l.Offset + int32(len(actors)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := int32(len(actors)) > l.LimitVal
	if hasMore {
		cursor, err = makeCursor(ctx, l.db.secretKey, l)
		if err != nil {
			return PageResult[Actor]{Error: err}
		}
	}

	return PageResult[Actor]{
		HasMore: hasMore,
		Results: actors[:util.MinInt32(l.LimitVal, int32(len(actors)))],
		Cursor:  cursor,
	}
}

func (l *listActorsFilters) FetchPage(ctx context.Context) PageResult[Actor] {
	return l.fetchPage(ctx)
}

func (l *listActorsFilters) Enumerate(ctx context.Context, callback func(PageResult[Actor]) (keepGoing bool, err error)) error {
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

func (db *gormDB) ListActorsBuilder() ListActorsBuilder {
	return &listActorsFilters{
		db:       db,
		LimitVal: 100,
	}
}

func (db *gormDB) ListActorsFromCursor(ctx context.Context, cursor string) (ListActorsExecutor, error) {
	b := &listActorsFilters{
		db:       db,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}

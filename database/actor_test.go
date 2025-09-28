package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/jwt"
	"github.com/rmorlok/authproxy/util"
	"github.com/rmorlok/authproxy/util/pagination"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestActor(t *testing.T) {
	var db DB
	var ctx context.Context
	now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
	clk := clock.NewFakeClock(now)

	setup := func(t *testing.T) {
		_, db = MustApplyBlankTestDbConfig(t.Name(), nil)
		ctx = apctx.NewBuilderBackground().WithClock(clk).Build()
	}

	t.Run("Validation", func(t *testing.T) {
		require.NoError(t, util.ToPtr(Actor{
			ID:         uuid.New(),
			ExternalId: "1234567890",
		}).validate())
		require.Error(t, util.ToPtr(Actor{
			ExternalId: "1234567890",
		}).validate())
		require.Error(t, util.ToPtr(Actor{
			ID: uuid.New(),
		}).validate())
		require.Error(t, util.ToPtr(Actor{}).validate())
	})
	t.Run("GetActor", func(t *testing.T) {
		setup(t)

		otherId := uuid.New()
		otherActor := &Actor{
			ID:         otherId,
			ExternalId: otherId.String(),
			Email:      "billclinton@example.com",
		}
		require.NoError(t, db.CreateActor(ctx, otherActor))

		id := uuid.New()
		a, err := db.GetActor(ctx, id)
		require.NoError(t, err)
		require.Nil(t, a, "actor should not exist")

		actor := &Actor{
			ID:         id,
			ExternalId: id.String(),
			Email:      "bobdole@example.com",
		}
		require.NoError(t, db.CreateActor(ctx, actor))

		a, err = db.GetActor(ctx, id)
		require.NoError(t, err)
		require.Equal(t, actor.Email, a.Email)
	})
	t.Run("GetActorByExternalId", func(t *testing.T) {
		setup(t)

		otherId := uuid.New()
		otherActor := &Actor{
			ID:         otherId,
			ExternalId: otherId.String(),
			Email:      "billclinton@example.com",
		}
		require.NoError(t, db.CreateActor(ctx, otherActor))

		id := uuid.New()
		a, err := db.GetActorByExternalId(ctx, id.String())
		require.NoError(t, err)
		require.Nil(t, a, "actor should not exist")

		actor := &Actor{
			ID:         id,
			ExternalId: id.String(),
			Email:      "bobdole@example.com",
		}
		require.NoError(t, db.CreateActor(ctx, actor))

		a, err = db.GetActorByExternalId(ctx, actor.ExternalId)
		require.NoError(t, err)
		require.Equal(t, actor.Email, a.Email)
	})
	t.Run("CreateActor", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			setup(t)

			id := uuid.New()
			actor := &Actor{
				ID:         id,
				ExternalId: id.String(),
				Email:      "bobdole@example.com",
			}
			require.NoError(t, db.CreateActor(ctx, actor))

			a, err := db.GetActor(ctx, id)
			require.NoError(t, err)
			require.Equal(t, actor.Email, a.Email)
		})
		t.Run("validates", func(t *testing.T) {
			setup(t)

			id := uuid.New()
			actor := &Actor{
				ID: id,
				// ExternalId omitted
				Email: "bobdole@example.com",
			}
			require.Error(t, db.CreateActor(ctx, actor))
		})
		t.Run("doesn't allow duplicate external id", func(t *testing.T) {
			setup(t)

			actor1 := &Actor{
				ID:         uuid.New(),
				ExternalId: "duplicate",
				Email:      "bobdole@example.com",
			}
			require.NoError(t, db.CreateActor(ctx, actor1))

			actor2 := &Actor{
				ID:         uuid.New(),
				ExternalId: "duplicate",
				Email:      "billclinton@example.com",
			}
			require.Error(t, db.CreateActor(ctx, actor2))
		})
		t.Run("doesn't update from an existing id", func(t *testing.T) {
			setup(t)

			id := uuid.New()
			actor1 := &Actor{
				ID:         id,
				ExternalId: uuid.New().String(),
				Email:      "bobdole@example.com",
			}
			require.NoError(t, db.CreateActor(ctx, actor1))

			actor2 := &Actor{
				ID:         id,
				ExternalId: uuid.New().String(),
				Email:      "billclinton@example.com",
			}
			require.Error(t, db.CreateActor(ctx, actor2))
		})
	})
	t.Run("UpsertActor", func(t *testing.T) {
		t.Run("fresh", func(t *testing.T) {
			setup(t)

			externalId := "bobdole"
			actor, err := db.UpsertActor(ctx, &jwt.Actor{
				ID:    externalId,
				Email: "bobdole@example.com",
			})
			require.NoError(t, err)
			require.Equal(t, externalId, actor.ExternalId)

			retrieved, err := db.GetActorByExternalId(ctx, externalId)
			require.NoError(t, err)
			require.Equal(t, actor.ID, retrieved.ID)
			require.Equal(t, actor.Email, retrieved.Email)
		})

		t.Run("updates existing", func(t *testing.T) {
			setup(t)

			id := uuid.New()
			externalId := "bobdole"
			err := db.CreateActor(ctx, &Actor{
				ID:         id,
				ExternalId: externalId,
				Email:      "bobdole@example.com",
			})
			require.NoError(t, err)

			retrieved, err := db.GetActorByExternalId(ctx, externalId)
			require.NoError(t, err)
			require.Equal(t, id, retrieved.ID)
			require.Equal(t, "bobdole@example.com", retrieved.Email)

			actor, err := db.UpsertActor(ctx, &jwt.Actor{
				ID:    externalId,
				Email: "thomasjefferson@example.com",
			})
			require.NoError(t, err)
			require.Equal(t, externalId, actor.ExternalId)
			require.Equal(t, id, actor.ID)
			require.Equal(t, "thomasjefferson@example.com", actor.Email)

			retrieved, err = db.GetActorByExternalId(ctx, externalId)
			require.NoError(t, err)
			require.Equal(t, id, retrieved.ID)
			require.Equal(t, "thomasjefferson@example.com", retrieved.Email)
		})
	})
	t.Run("List", func(t *testing.T) {
		setup(t)

		var firstUuid, lastUuid uuid.UUID
		for i := 0; i < 50; i++ {
			now = now.Add(time.Second)
			clk.SetTime(now)

			u := uuid.New()
			if i == 0 {
				firstUuid = u
			}
			lastUuid = u

			isAdmin := false
			isSuperAdmin := false

			if i%5 == 1 {
				isAdmin = true
			} else if i%13 == 1 {
				isSuperAdmin = true
			}

			externalID := u.String()
			if isAdmin {
				externalID = "admin/" + externalID
			} else if isSuperAdmin {
				externalID = "superadmin/" + externalID
			}

			err := db.CreateActor(ctx, &Actor{ID: u, ExternalId: externalID, Email: u.String() + "@example.com", Admin: isAdmin, SuperAdmin: isSuperAdmin})
			require.NoError(t, err)
		}

		t.Run("all actors", func(t *testing.T) {
			result := db.ListActorsBuilder().Limit(10).FetchPage(ctx)
			require.NoError(t, result.Error)
			require.Len(t, result.Results, 10)
			require.Equal(t, result.Results[0].ID, firstUuid)
			require.True(t, result.HasMore)
			require.NotEmpty(t, result.Cursor)

			total := 10
			cursor := result.Cursor
			var last Actor

			for cursor != "" {
				ex, err := db.ListActorsFromCursor(ctx, cursor)
				require.NoError(t, err)
				result = ex.FetchPage(ctx)
				require.NoError(t, result.Error)
				require.True(t, len(result.Results) > 0)

				last = result.Results[len(result.Results)-1]
				total += len(result.Results)
				cursor = result.Cursor
			}

			require.Equal(t, 50, total)
			require.Equal(t, lastUuid, last.ID)
		})

		t.Run("reverse order", func(t *testing.T) {
			var allResults []Actor
			q := db.ListActorsBuilder().Limit(7).OrderBy(ActorOrderByCreatedAt, pagination.OrderByDesc)
			err := q.Enumerate(ctx, func(result pagination.PageResult[Actor]) (bool, error) {
				allResults = append(allResults, result.Results...)
				return true, nil
			})

			require.NoError(t, err)
			require.Len(t, allResults, 50)
			require.Equal(t, lastUuid, allResults[0].ID)
			require.Equal(t, firstUuid, allResults[49].ID)
		})
	})
	t.Run("IsAdmin", func(t *testing.T) {
		u := Actor{}
		require.False(t, u.IsAdmin())
		u.Admin = true
		require.True(t, u.IsAdmin())
		u.Admin = false
		require.False(t, u.IsAdmin())

		var nila *Actor
		require.False(t, nila.IsAdmin())
	})
	t.Run("IsSuperAdmin", func(t *testing.T) {
		u := Actor{}
		require.False(t, u.IsSuperAdmin())
		u.SuperAdmin = true
		require.True(t, u.IsSuperAdmin())
		u.SuperAdmin = false
		require.False(t, u.IsSuperAdmin())

		var nila *Actor
		require.False(t, nila.IsSuperAdmin())
	})
	t.Run("IsNormalActor", func(t *testing.T) {
		u := Actor{}
		require.True(t, u.IsNormalActor())
		u.SuperAdmin = true
		require.False(t, u.IsNormalActor())
		u.SuperAdmin = false
		u.Admin = true
		require.False(t, u.IsNormalActor())
		u.SuperAdmin = false
		u.Admin = false
		require.True(t, u.IsNormalActor())

		var nila *Actor
		require.True(t, nila.IsNormalActor())
	})
	t.Run("DeleteActor soft delete and IncludeDeleted listing", func(t *testing.T) {
		setup(t)

		// create a single actor
		id := uuid.New()
		a := &Actor{ID: id, ExternalId: id.String(), Email: "delete-me@example.com"}
		require.NoError(t, db.CreateActor(ctx, a))

		// delete it
		require.NoError(t, db.DeleteActor(ctx, id))

		// direct get should return nil (soft-deleted)
		got, err := db.GetActor(ctx, id)
		require.NoError(t, err)
		require.Nil(t, got)

		// default listing should not include deleted
		page := db.ListActorsBuilder().FetchPage(ctx)
		require.NoError(t, page.Error)
		require.Len(t, page.Results, 0)

		// IncludeDeleted should bring it back
		page = db.ListActorsBuilder().IncludeDeleted().FetchPage(ctx)
		require.NoError(t, page.Error)
		require.Len(t, page.Results, 1)
		require.Equal(t, id, page.Results[0].ID)
		require.False(t, page.Results[0].DeletedAt.Time.IsZero())
	})

	t.Run("GetID and ToJwtActor", func(t *testing.T) {
		setup(t)

		id := uuid.New()
		a := &Actor{ID: id, ExternalId: "user/" + id.String(), Email: "id@example.com", Admin: false, SuperAdmin: false}
		require.Equal(t, id, a.GetID())

		ja := a.ToJwtActor()
		require.Equal(t, a.ExternalId, ja.ID)
		require.Equal(t, a.Email, ja.Email)
		require.Equal(t, a.Admin, ja.Admin)
		require.Equal(t, a.SuperAdmin, ja.SuperAdmin)
	})

	t.Run("IsValidActorOrderByField", func(t *testing.T) {
		// Valid values (typed)
		require.True(t, IsValidActorOrderByField(ActorOrderByCreatedAt))
		require.True(t, IsValidActorOrderByField(ActorOrderByUpdatedAt))
		require.True(t, IsValidActorOrderByField(ActorOrderByEmail))
		require.True(t, IsValidActorOrderByField(ActorOrderByExternalId))
		require.True(t, IsValidActorOrderByField(ActorOrderByAdmin))
		require.True(t, IsValidActorOrderByField(ActorOrderBySuperAdmin))
		require.True(t, IsValidActorOrderByField(ActorOrderByDeletedAt))

		// Valid values (as strings)
		require.True(t, IsValidActorOrderByField("created_at"))
		require.True(t, IsValidActorOrderByField("updated_at"))
		require.True(t, IsValidActorOrderByField("email"))
		require.True(t, IsValidActorOrderByField("external_id"))
		require.True(t, IsValidActorOrderByField("admin"))
		require.True(t, IsValidActorOrderByField("super_admin"))
		require.True(t, IsValidActorOrderByField("deleted_at"))

		// Invalid values
		require.False(t, IsValidActorOrderByField(ActorOrderByField("nope")))
		require.False(t, IsValidActorOrderByField("nope"))
	})
}

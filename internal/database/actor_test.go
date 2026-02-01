package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apctx"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
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
			Id:         uuid.New(),
			Namespace:  "root",
			ExternalId: "1234567890",
		}).validate())
		require.Error(t, util.ToPtr(Actor{
			Namespace:  "root",
			ExternalId: "1234567890",
		}).validate())
		require.Error(t, util.ToPtr(Actor{
			Id:        uuid.New(),
			Namespace: "root",
		}).validate())
		require.Error(t, util.ToPtr(Actor{
			Id:         uuid.New(),
			Namespace:  "bad",
			ExternalId: "1234567890",
		}).validate())
		require.Error(t, util.ToPtr(Actor{}).validate())
	})
	t.Run("GetActor", func(t *testing.T) {
		setup(t)

		otherId := uuid.New()
		otherActor := &Actor{
			Id:         otherId,
			Namespace:  "root",
			ExternalId: otherId.String(),
		}
		require.NoError(t, db.CreateActor(ctx, otherActor))

		id := uuid.New()
		a, err := db.GetActor(ctx, id)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, a, "actor should not exist")

		actor := &Actor{
			Id:         id,
			Namespace:  "root",
			ExternalId: id.String(),
		}
		require.NoError(t, db.CreateActor(ctx, actor))

		a, err = db.GetActor(ctx, id)
		require.NoError(t, err)
		require.Equal(t, actor.ExternalId, a.ExternalId)
	})
	t.Run("GetActorByExternalId", func(t *testing.T) {
		setup(t)

		otherId := uuid.New()
		otherActor := &Actor{
			Id:         otherId,
			Namespace:  "root",
			ExternalId: otherId.String(),
		}
		require.NoError(t, db.CreateActor(ctx, otherActor))

		id := uuid.New()
		a, err := db.GetActorByExternalId(ctx, "root", id.String())
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, a, "actor should not exist")

		actor := &Actor{
			Id:         id,
			Namespace:  "root",
			ExternalId: id.String(),
		}
		require.NoError(t, db.CreateActor(ctx, actor))

		a, err = db.GetActorByExternalId(ctx, "root", actor.ExternalId)
		require.NoError(t, err)
		require.Equal(t, actor.ExternalId, a.ExternalId)

		err = db.DeleteActor(ctx, actor.Id)
		require.NoError(t, err)

		a, err = db.GetActorByExternalId(ctx, "root", actor.ExternalId)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, a, "actor should not exist")
	})
	t.Run("CreateActor", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			setup(t)

			id := uuid.New()
			actor := &Actor{
				Id:         id,
				Namespace:  "root",
				ExternalId: id.String(),
				Permissions: Permissions{
					aschema.Permission{
						Namespace: "root",
						Resources: []string{"*"},
						Verbs:     []string{"*"},
					},
				},
			}
			require.NoError(t, db.CreateActor(ctx, actor))

			a, err := db.GetActor(ctx, id)
			require.NoError(t, err)
			require.Equal(t, actor.Permissions, a.Permissions)
		})
		t.Run("validates", func(t *testing.T) {
			setup(t)

			id := uuid.New()
			actor := &Actor{
				Id: id,
				// ExternalId omitted
			}
			require.Error(t, db.CreateActor(ctx, actor))
		})
		t.Run("doesn't allow duplicate external id", func(t *testing.T) {
			setup(t)

			actor1 := &Actor{
				Id:         uuid.New(),
				Namespace:  "root",
				ExternalId: "duplicate",
			}
			require.NoError(t, db.CreateActor(ctx, actor1))

			actor2 := &Actor{
				Id:         uuid.New(),
				ExternalId: "duplicate",
			}
			require.Error(t, db.CreateActor(ctx, actor2))
		})
		t.Run("doesn't update from an existing id", func(t *testing.T) {
			setup(t)

			id := uuid.New()
			actor1 := &Actor{
				Id:         id,
				Namespace:  "root",
				ExternalId: uuid.New().String(),
			}
			require.NoError(t, db.CreateActor(ctx, actor1))

			actor2 := &Actor{
				Id:         id,
				ExternalId: uuid.New().String(),
			}
			require.Error(t, db.CreateActor(ctx, actor2))
		})
	})
	t.Run("UpsertActor", func(t *testing.T) {
		t.Run("fresh", func(t *testing.T) {
			setup(t)

			externalId := "bobdole"
			actor, err := db.UpsertActor(ctx, &core.Actor{
				ExternalId: externalId,
				Namespace:  "root",
			})
			require.NoError(t, err)
			require.Equal(t, externalId, actor.ExternalId)

			retrieved, err := db.GetActorByExternalId(ctx, "root", externalId)
			require.NoError(t, err)
			require.Equal(t, actor.Id, retrieved.Id)
		})

		t.Run("updates existing", func(t *testing.T) {
			t.Run("permissions", func(t *testing.T) {
				setup(t)

				id := uuid.New()
				externalId := "bobdole"
				err := db.CreateActor(ctx, &Actor{
					Id:         id,
					Namespace:  "root",
					ExternalId: externalId,
					Permissions: Permissions{
						aschema.Permission{
							Namespace: "root",
							Resources: []string{"connections", "connectors"},
							Verbs:     []string{"read", "create"},
						},
					},
				})
				require.NoError(t, err)

				retrieved, err := db.GetActorByExternalId(ctx, "root", externalId)
				require.NoError(t, err)
				require.Equal(t, id, retrieved.Id)
				require.Equal(t, Permissions{
					aschema.Permission{
						Namespace: "root",
						Resources: []string{"connections", "connectors"},
						Verbs:     []string{"read", "create"},
					},
				}, retrieved.Permissions)

				actor, err := db.UpsertActor(ctx, &core.Actor{
					ExternalId: externalId,
					Namespace:  "root",
					Permissions: []aschema.Permission{
						{
							Namespace: "root",
							Resources: []string{"connections", "connectors"},
							Verbs:     []string{"read", "create"},
						},
						{
							Namespace:   "root",
							Resources:   []string{"connections"},
							ResourceIds: []string{"1234567890"},
							Verbs:       []string{"proxy"},
						},
					},
				})
				require.NoError(t, err)
				require.Equal(t, externalId, actor.ExternalId)
				require.Equal(t, id, actor.Id)
				require.Equal(t, Permissions{
					{
						Namespace: "root",
						Resources: []string{"connections", "connectors"},
						Verbs:     []string{"read", "create"},
					},
					{
						Namespace:   "root",
						Resources:   []string{"connections"},
						ResourceIds: []string{"1234567890"},
						Verbs:       []string{"proxy"},
					},
				}, actor.Permissions)

				retrieved, err = db.GetActorByExternalId(ctx, "root", externalId)
				require.NoError(t, err)
				require.Equal(t, id, retrieved.Id)
				require.Equal(t, Permissions{
					{
						Namespace: "root",
						Resources: []string{"connections", "connectors"},
						Verbs:     []string{"read", "create"},
					},
					{
						Namespace:   "root",
						Resources:   []string{"connections"},
						ResourceIds: []string{"1234567890"},
						Verbs:       []string{"proxy"},
					},
				}, retrieved.Permissions)
			})
			t.Run("only updates target actor not others", func(t *testing.T) {
				setup(t)

				// Create first actor
				id1 := uuid.New()
				externalId1 := "actor1"
				originalPerms1 := Permissions{
					aschema.Permission{
						Namespace: "root",
						Resources: []string{"connections"},
						Verbs:     []string{"read"},
					},
				}
				err := db.CreateActor(ctx, &Actor{
					Id:          id1,
					Namespace:   "root",
					ExternalId:  externalId1,
					Permissions: originalPerms1,
				})
				require.NoError(t, err)

				// Create second actor
				id2 := uuid.New()
				externalId2 := "actor2"
				originalPerms2 := Permissions{
					aschema.Permission{
						Namespace: "root",
						Resources: []string{"connectors"},
						Verbs:     []string{"read"},
					},
				}
				err = db.CreateActor(ctx, &Actor{
					Id:          id2,
					Namespace:   "root",
					ExternalId:  externalId2,
					Permissions: originalPerms2,
				})
				require.NoError(t, err)

				// Create third actor
				id3 := uuid.New()
				externalId3 := "actor3"
				originalPerms3 := Permissions{
					aschema.Permission{
						Namespace: "root",
						Resources: []string{"*"},
						Verbs:     []string{"*"},
					},
				}
				err = db.CreateActor(ctx, &Actor{
					Id:          id3,
					Namespace:   "root",
					ExternalId:  externalId3,
					Permissions: originalPerms3,
				})
				require.NoError(t, err)

				// Update only actor2's permissions
				newPerms2 := []aschema.Permission{
					{
						Namespace: "root",
						Resources: []string{"connectors", "connections"},
						Verbs:     []string{"read", "create", "delete"},
					},
				}
				actor2, err := db.UpsertActor(ctx, &core.Actor{
					ExternalId:  externalId2,
					Namespace:   "root",
					Permissions: newPerms2,
				})
				require.NoError(t, err)
				require.Equal(t, id2, actor2.Id)
				require.Equal(t, Permissions(newPerms2), actor2.Permissions)

				// Verify actor1 was NOT affected
				retrieved1, err := db.GetActorByExternalId(ctx, "root", externalId1)
				require.NoError(t, err)
				require.Equal(t, id1, retrieved1.Id)
				require.Equal(t, originalPerms1, retrieved1.Permissions)

				// Verify actor2 was updated correctly
				retrieved2, err := db.GetActorByExternalId(ctx, "root", externalId2)
				require.NoError(t, err)
				require.Equal(t, id2, retrieved2.Id)
				require.Equal(t, Permissions(newPerms2), retrieved2.Permissions)

				// Verify actor3 was NOT affected
				retrieved3, err := db.GetActorByExternalId(ctx, "root", externalId3)
				require.NoError(t, err)
				require.Equal(t, id3, retrieved3.Id)
				require.Equal(t, originalPerms3, retrieved3.Permissions)
			})
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

			err := db.CreateActor(ctx, &Actor{
				Id:         u,
				Namespace:  "root",
				ExternalId: u.String(),
			})
			require.NoError(t, err)
		}

		t.Run("all actors", func(t *testing.T) {
			result := db.ListActorsBuilder().Limit(10).FetchPage(ctx)
			require.NoError(t, result.Error)
			require.Len(t, result.Results, 10)
			require.Equal(t, result.Results[0].Id, firstUuid)
			require.True(t, result.HasMore)
			require.NotEmpty(t, result.Cursor)

			total := 10
			cursor := result.Cursor
			var last *Actor

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
			require.Equal(t, lastUuid, last.Id)
		})

		t.Run("reverse order", func(t *testing.T) {
			var allResults []*Actor
			q := db.ListActorsBuilder().Limit(7).OrderBy(ActorOrderByCreatedAt, pagination.OrderByDesc)
			err := q.Enumerate(ctx, func(result pagination.PageResult[*Actor]) (bool, error) {
				allResults = append(allResults, result.Results...)
				return true, nil
			})

			require.NoError(t, err)
			require.Len(t, allResults, 50)
			require.Equal(t, lastUuid, allResults[0].Id)
			require.Equal(t, firstUuid, allResults[49].Id)
		})
	})
	t.Run("CanSelfSign", func(t *testing.T) {
		u := Actor{}
		require.False(t, u.CanSelfSign())

		encryptedKey := "some-encrypted-key"
		u.EncryptedKey = &encryptedKey
		require.True(t, u.CanSelfSign())

		u.EncryptedKey = nil
		require.False(t, u.CanSelfSign())

		var nila *Actor
		require.False(t, nila.CanSelfSign())
	})
	t.Run("DeleteActor soft delete and IncludeDeleted listing", func(t *testing.T) {
		setup(t)

		// create a single actor
		id := uuid.New()
		a := &Actor{Id: id, Namespace: "root", ExternalId: id.String()}
		require.NoError(t, db.CreateActor(ctx, a))

		// delete it
		require.NoError(t, db.DeleteActor(ctx, id))

		// direct get should return nil (soft-deleted)
		got, err := db.GetActor(ctx, id)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, got)

		// default listing should not include deleted
		page := db.ListActorsBuilder().FetchPage(ctx)
		require.NoError(t, page.Error)
		require.Len(t, page.Results, 0)

		// IncludeDeleted should bring it back
		page = db.ListActorsBuilder().IncludeDeleted().FetchPage(ctx)
		require.NoError(t, page.Error)
		require.Len(t, page.Results, 1)
		require.Equal(t, id, page.Results[0].Id)
		require.NotNil(t, page.Results[0].DeletedAt)
	})

	t.Run("IsValidActorOrderByField", func(t *testing.T) {
		// Valid values (typed)
		require.True(t, IsValidActorOrderByField(ActorOrderByCreatedAt))
		require.True(t, IsValidActorOrderByField(ActorOrderByUpdatedAt))
		require.True(t, IsValidActorOrderByField(ActorOrderByNamespace))
		require.True(t, IsValidActorOrderByField(ActorOrderByExternalId))
		require.True(t, IsValidActorOrderByField(ActorOrderByDeletedAt))

		// Valid values (as strings)
		require.True(t, IsValidActorOrderByField("created_at"))
		require.True(t, IsValidActorOrderByField("updated_at"))
		require.True(t, IsValidActorOrderByField("namespace"))
		require.True(t, IsValidActorOrderByField("external_id"))
		require.True(t, IsValidActorOrderByField("deleted_at"))

		// Invalid values
		require.False(t, IsValidActorOrderByField(ActorOrderByField("nope")))
		require.False(t, IsValidActorOrderByField("nope"))
	})

	t.Run("Namespace", func(t *testing.T) {
		t.Run("validation", func(t *testing.T) {
			t.Run("valid paths", func(t *testing.T) {
				require.NoError(t, util.ToPtr(Actor{
					Id:         uuid.New(),
					Namespace:  "root",
					ExternalId: "test",
				}).validate())
				require.NoError(t, util.ToPtr(Actor{
					Id:         uuid.New(),
					Namespace:  "root.tenant1",
					ExternalId: "test",
				}).validate())
				require.NoError(t, util.ToPtr(Actor{
					Id:         uuid.New(),
					Namespace:  "root.tenant1.subtenant",
					ExternalId: "test",
				}).validate())
			})
			t.Run("invalid paths", func(t *testing.T) {
				require.Error(t, util.ToPtr(Actor{
					Id:         uuid.New(),
					Namespace:  "",
					ExternalId: "test",
				}).validate())
				require.Error(t, util.ToPtr(Actor{
					Id:         uuid.New(),
					Namespace:  "invalid",
					ExternalId: "test",
				}).validate())
				require.Error(t, util.ToPtr(Actor{
					Id:         uuid.New(),
					Namespace:  "root.",
					ExternalId: "test",
				}).validate())
			})
		})

		t.Run("create with custom namespace", func(t *testing.T) {
			setup(t)

			err := db.EnsureNamespaceByPath(ctx, "root.tenant1")
			require.NoError(t, err)
			id := uuid.New()
			actor := &Actor{
				Id:         id,
				Namespace:  "root.tenant1",
				ExternalId: id.String(),
			}
			require.NoError(t, db.CreateActor(ctx, actor))

			a, err := db.GetActor(ctx, id)
			require.NoError(t, err)
			require.Equal(t, "root.tenant1", a.GetNamespace())
		})

		t.Run("upsert preserves namespace", func(t *testing.T) {
			setup(t)

			externalId := "namespaced-actor"

			// Create with custom namespace
			actor, err := db.UpsertActor(ctx, &core.Actor{
				ExternalId: externalId,
				Namespace:  "root.tenant1",
			})
			require.NoError(t, err)
			require.Equal(t, "root.tenant1", actor.GetNamespace())

			// Update with same namespace should preserve
			actor, err = db.UpsertActor(ctx, &core.Actor{
				ExternalId: externalId,
				Namespace:  "root.tenant1",
			})
			require.NoError(t, err)
			require.Equal(t, "root.tenant1", actor.GetNamespace())

			// Verify in database
			retrieved, err := db.GetActorByExternalId(ctx, "root.tenant1", externalId)
			require.NoError(t, err)
			require.Equal(t, "root.tenant1", retrieved.GetNamespace())
		})

		t.Run("list filtering", func(t *testing.T) {
			setup(t)

			// Create actors in different namespaces
			actors := []struct {
				namespace  string
				externalId string
			}{
				{"root", "actor1"},
				{"root", "actor2"},
				{"root.tenant1", "actor3"},
				{"root.tenant1", "actor4"},
				{"root.tenant1.sub", "actor5"},
				{"root.tenant2", "actor6"},
			}

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.tenant1",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			err = db.CreateNamespace(ctx, &Namespace{
				Path:  "root.tenant1.sub",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			err = db.CreateNamespace(ctx, &Namespace{
				Path:  "root.tenant2",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			for _, a := range actors {
				id := uuid.New()
				err := db.CreateActor(ctx, &Actor{
					Id:         id,
					Namespace:  a.namespace,
					ExternalId: a.externalId,
				})
				require.NoError(t, err)
			}

			t.Run("exact match", func(t *testing.T) {
				result := db.ListActorsBuilder().ForNamespaceMatcher("root").FetchPage(ctx)
				require.NoError(t, result.Error)
				require.Len(t, result.Results, 2)
				for _, a := range result.Results {
					require.Equal(t, "root", a.Namespace)
				}
			})

			t.Run("wildcard match", func(t *testing.T) {
				result := db.ListActorsBuilder().ForNamespaceMatcher("root.tenant1.**").FetchPage(ctx)
				require.NoError(t, result.Error)
				require.Len(t, result.Results, 3) // actor3, actor4, actor5
				for _, a := range result.Results {
					require.True(t, a.Namespace == "root.tenant1" || a.Namespace == "root.tenant1.sub")
				}
			})

			t.Run("multiple matchers", func(t *testing.T) {
				result := db.ListActorsBuilder().ForNamespaceMatchers([]string{"root", "root.tenant2"}).FetchPage(ctx)
				require.NoError(t, result.Error)
				require.Len(t, result.Results, 3) // actor1, actor2, actor6
			})

			t.Run("invalid matcher returns error", func(t *testing.T) {
				result := db.ListActorsBuilder().ForNamespaceMatcher("invalid").FetchPage(ctx)
				require.Error(t, result.Error)
			})
		})
	})

	t.Run("Labels", func(t *testing.T) {
		t.Run("ForLabelExists filters by label key", func(t *testing.T) {
			setup(t)

			// Create actors with different labels
			actor1 := &Actor{
				Id:         uuid.New(),
				Namespace:  "root",
				ExternalId: "actor1",
				Labels: Labels{
					"authproxy.io/actor-sync-source": "config-list",
				},
			}
			require.NoError(t, db.CreateActor(ctx, actor1))

			actor2 := &Actor{
				Id:         uuid.New(),
				Namespace:  "root",
				ExternalId: "actor2",
				Labels: Labels{
					"authproxy.io/actor-sync-source": "external-source",
				},
			}
			require.NoError(t, db.CreateActor(ctx, actor2))

			actor3 := &Actor{
				Id:         uuid.New(),
				Namespace:  "root",
				ExternalId: "actor3",
				// No labels
			}
			require.NoError(t, db.CreateActor(ctx, actor3))

			// Filter for label existence
			result := db.ListActorsBuilder().ForLabelExists("authproxy.io/actor-sync-source").FetchPage(ctx)
			require.NoError(t, result.Error)
			require.Len(t, result.Results, 2)
		})

		t.Run("ForLabelEquals filters by label key and value", func(t *testing.T) {
			setup(t)

			// Create actors with different label values
			actor1 := &Actor{
				Id:         uuid.New(),
				Namespace:  "root",
				ExternalId: "actor1",
				Labels: Labels{
					"authproxy.io/actor-sync-source": "config-list",
				},
			}
			require.NoError(t, db.CreateActor(ctx, actor1))

			actor2 := &Actor{
				Id:         uuid.New(),
				Namespace:  "root",
				ExternalId: "actor2",
				Labels: Labels{
					"authproxy.io/actor-sync-source": "external-source",
				},
			}
			require.NoError(t, db.CreateActor(ctx, actor2))

			actor3 := &Actor{
				Id:         uuid.New(),
				Namespace:  "root",
				ExternalId: "actor3",
				Labels: Labels{
					"authproxy.io/actor-sync-source": "config-list",
				},
			}
			require.NoError(t, db.CreateActor(ctx, actor3))

			// Filter for specific label value
			result := db.ListActorsBuilder().ForLabelEquals("authproxy.io/actor-sync-source", "config-list").FetchPage(ctx)
			require.NoError(t, result.Error)
			require.Len(t, result.Results, 2)
			for _, a := range result.Results {
				require.Equal(t, "config-list", a.Labels["authproxy.io/actor-sync-source"])
			}

			// Filter for different label value
			result = db.ListActorsBuilder().ForLabelEquals("authproxy.io/actor-sync-source", "external-source").FetchPage(ctx)
			require.NoError(t, result.Error)
			require.Len(t, result.Results, 1)
			require.Equal(t, "actor2", result.Results[0].ExternalId)
		})

		t.Run("Filtering", func(t *testing.T) {
			_, db, _ := MustApplyBlankTestDbConfigRaw(t.Name(), nil)
			now := time.Date(2023, 10, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Seed data for Actors
			actors := []*Actor{
				{
					Id:         uuid.New(),
					Namespace:  "root",
					ExternalId: "actor1",
					Labels:     Labels{"app": "web", "env": "prod"},
				},
				{
					Id:         uuid.New(),
					Namespace:  "root",
					ExternalId: "actor2",
					Labels:     Labels{"app": "api", "env": "prod"},
				},
				{
					Id:         uuid.New(),
					Namespace:  "root",
					ExternalId: "actor3",
					Labels:     Labels{"app": "web", "env": "dev", "tier": "frontend"},
				},
				{
					Id:         uuid.New(),
					Namespace:  "root",
					ExternalId: "actor4",
					Labels:     Labels{"env": "dev"},
				},
			}

			for _, a := range actors {
				require.NoError(t, db.CreateActor(ctx, a))
			}

			// Seed data for Namespaces
			namespaces := []*Namespace{
				{
					Path:   "root.n1",
					State:  NamespaceStateActive,
					Labels: Labels{"type": "system"},
				},
				{
					Path:   "root.n2",
					State:  NamespaceStateActive,
					Labels: Labels{"type": "user", "active": "true"},
				},
				{
					Path:   "root.n3",
					State:  NamespaceStateActive,
					Labels: Labels{"type": "user", "active": "false"},
				},
			}

			for _, ns := range namespaces {
				require.NoError(t, db.CreateNamespace(ctx, ns))
			}

			t.Run("Actor equality filter", func(t *testing.T) {
				pr := db.ListActorsBuilder().
					ForLabelSelector("app=web").
					FetchPage(ctx)
				require.NoError(t, pr.Error)
				require.Len(t, pr.Results, 2)

				ids := []string{pr.Results[0].ExternalId, pr.Results[1].ExternalId}
				require.Contains(t, ids, "actor1")
				require.Contains(t, ids, "actor3")
			})

			t.Run("Actor inequality filter", func(t *testing.T) {
				// env!=prod should return actor3, actor4
				pr := db.ListActorsBuilder().
					ForLabelSelector("env!=prod").
					FetchPage(ctx)
				require.NoError(t, pr.Error)
				require.Len(t, pr.Results, 2)

				ids := []string{pr.Results[0].ExternalId, pr.Results[1].ExternalId}
				require.Contains(t, ids, "actor3")
				require.Contains(t, ids, "actor4")
			})

			t.Run("Actor existence filter", func(t *testing.T) {
				pr := db.ListActorsBuilder().
					ForLabelSelector("tier").
					FetchPage(ctx)
				require.NoError(t, pr.Error)
				require.Len(t, pr.Results, 1)
				require.Equal(t, "actor3", pr.Results[0].ExternalId)
			})

			t.Run("Actor non-existence filter", func(t *testing.T) {
				// !tier should return actor1, actor2, actor4
				pr := db.ListActorsBuilder().
					ForLabelSelector("!tier").
					FetchPage(ctx)
				require.NoError(t, pr.Error)
				require.Len(t, pr.Results, 3)

				ids := []string{pr.Results[0].ExternalId, pr.Results[1].ExternalId, pr.Results[2].ExternalId}
				require.Contains(t, ids, "actor1")
				require.Contains(t, ids, "actor2")
				require.Contains(t, ids, "actor4")
			})

			t.Run("Actor multiple filters", func(t *testing.T) {
				pr := db.ListActorsBuilder().
					ForLabelSelector("app=web,env=dev").
					FetchPage(ctx)
				require.NoError(t, pr.Error)
				require.Len(t, pr.Results, 1)
				require.Equal(t, "actor3", pr.Results[0].ExternalId)
			})
		})
	})

	t.Run("EncryptedKey", func(t *testing.T) {
		t.Run("stores and retrieves encrypted key", func(t *testing.T) {
			setup(t)

			encryptedKeyVal := "base64encodedencryptedkey123"
			actor := &Actor{
				Id:           uuid.New(),
				Namespace:    "root",
				ExternalId:   "testuser",
				EncryptedKey: &encryptedKeyVal,
			}
			require.NoError(t, db.CreateActor(ctx, actor))

			retrieved, err := db.GetActor(ctx, actor.Id)
			require.NoError(t, err)
			require.NotNil(t, retrieved.EncryptedKey)
			require.Equal(t, encryptedKeyVal, *retrieved.EncryptedKey)
		})

		t.Run("nil encrypted key", func(t *testing.T) {
			setup(t)

			actor := &Actor{
				Id:           uuid.New(),
				Namespace:    "root",
				ExternalId:   "testuser2",
				EncryptedKey: nil,
			}
			require.NoError(t, db.CreateActor(ctx, actor))

			retrieved, err := db.GetActor(ctx, actor.Id)
			require.NoError(t, err)
			require.Nil(t, retrieved.EncryptedKey)
		})
	})
}

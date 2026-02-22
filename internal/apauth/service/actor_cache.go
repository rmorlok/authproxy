package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/database"
)

type contextKeyActorCache struct{}

// actorCache is a per-request cache for database.Actor lookups. It avoids redundant database
// queries when the same actor is loaded multiple times during a single auth flow (e.g. once
// during JWT key selection and again when building the RequestAuth).
type actorCache struct {
	byExternalId map[string]*database.Actor
	byId         map[uuid.UUID]*database.Actor
}

func newActorCache() *actorCache {
	return &actorCache{
		byExternalId: make(map[string]*database.Actor),
		byId:         make(map[uuid.UUID]*database.Actor),
	}
}

func externalIdKey(namespace, externalId string) string {
	return fmt.Sprintf("%s:%s", namespace, externalId)
}

// Put stores an actor in the cache, indexed by both external ID and internal ID.
func (c *actorCache) Put(actor *database.Actor) {
	if actor == nil {
		return
	}
	c.byExternalId[externalIdKey(actor.Namespace, actor.ExternalId)] = actor
	if actor.Id != uuid.Nil {
		c.byId[actor.Id] = actor
	}
}

// GetByExternalId returns a cached actor by namespace and external ID, or nil if not cached.
func (c *actorCache) GetByExternalId(namespace, externalId string) *database.Actor {
	return c.byExternalId[externalIdKey(namespace, externalId)]
}

// GetById returns a cached actor by internal UUID, or nil if not cached.
func (c *actorCache) GetById(id uuid.UUID) *database.Actor {
	return c.byId[id]
}

// withActorCache creates a new actor cache and stores it in the context. If a cache already
// exists in the context, the existing context is returned unchanged.
func withActorCache(ctx context.Context) context.Context {
	if ctx.Value(contextKeyActorCache{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, contextKeyActorCache{}, newActorCache())
}

// getActorCache retrieves the actor cache from the context, or nil if none exists.
func getActorCache(ctx context.Context) *actorCache {
	if c, ok := ctx.Value(contextKeyActorCache{}).(*actorCache); ok {
		return c
	}
	return nil
}

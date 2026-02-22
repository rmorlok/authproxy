package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/require"
)

func TestActorCache_PutAndGetByExternalId(t *testing.T) {
	t.Parallel()
	c := newActorCache()

	actor := &database.Actor{
		Id:         uuid.New(),
		Namespace:  "root",
		ExternalId: "alice",
	}

	c.Put(actor)

	got := c.GetByExternalId("root", "alice")
	require.Equal(t, actor, got)
}

func TestActorCache_PutAndGetById(t *testing.T) {
	t.Parallel()
	c := newActorCache()

	id := uuid.New()
	actor := &database.Actor{
		Id:         id,
		Namespace:  "root",
		ExternalId: "bob",
	}

	c.Put(actor)

	got := c.GetById(id)
	require.Equal(t, actor, got)
}

func TestActorCache_GetByExternalId_Miss(t *testing.T) {
	t.Parallel()
	c := newActorCache()

	got := c.GetByExternalId("root", "nonexistent")
	require.Nil(t, got)
}

func TestActorCache_GetById_Miss(t *testing.T) {
	t.Parallel()
	c := newActorCache()

	got := c.GetById(uuid.New())
	require.Nil(t, got)
}

func TestActorCache_PutNil(t *testing.T) {
	t.Parallel()
	c := newActorCache()

	// Should not panic
	c.Put(nil)

	require.Empty(t, c.byExternalId)
	require.Empty(t, c.byId)
}

func TestActorCache_PutNilId(t *testing.T) {
	t.Parallel()
	c := newActorCache()

	actor := &database.Actor{
		Id:         uuid.Nil,
		Namespace:  "root",
		ExternalId: "charlie",
	}

	c.Put(actor)

	// Should be retrievable by external ID but not by nil UUID
	got := c.GetByExternalId("root", "charlie")
	require.Equal(t, actor, got)

	got = c.GetById(uuid.Nil)
	require.Nil(t, got)
}

func TestActorCache_NamespaceIsolation(t *testing.T) {
	t.Parallel()
	c := newActorCache()

	actor1 := &database.Actor{
		Id:         uuid.New(),
		Namespace:  "ns1",
		ExternalId: "alice",
	}
	actor2 := &database.Actor{
		Id:         uuid.New(),
		Namespace:  "ns2",
		ExternalId: "alice",
	}

	c.Put(actor1)
	c.Put(actor2)

	got1 := c.GetByExternalId("ns1", "alice")
	require.Equal(t, actor1, got1)

	got2 := c.GetByExternalId("ns2", "alice")
	require.Equal(t, actor2, got2)
}

func TestWithActorCache_CreatesCache(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	require.Nil(t, getActorCache(ctx))

	ctx = withActorCache(ctx)
	cache := getActorCache(ctx)
	require.NotNil(t, cache)
}

func TestWithActorCache_Idempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ctx = withActorCache(ctx)
	cache1 := getActorCache(ctx)

	ctx = withActorCache(ctx)
	cache2 := getActorCache(ctx)

	// Should return the same cache instance, not create a new one
	require.Same(t, cache1, cache2)
}

func TestGetActorCache_NilWhenMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	require.Nil(t, getActorCache(ctx))
}

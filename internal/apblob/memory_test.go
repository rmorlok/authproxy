package apblob

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryClient_PutAndGet(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryClient()

	err := c.Put(ctx, PutInput{
		Key:  "test-key",
		Data: []byte("hello world"),
	})
	require.NoError(t, err)

	data, err := c.Get(ctx, "test-key")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello world"), data)
}

func TestMemoryClient_GetNotFound(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryClient()

	_, err := c.Get(ctx, "nonexistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrBlobNotFound))
}

func TestMemoryClient_PutOverwrite(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryClient()

	require.NoError(t, c.Put(ctx, PutInput{Key: "k", Data: []byte("v1")}))
	require.NoError(t, c.Put(ctx, PutInput{Key: "k", Data: []byte("v2")}))

	data, err := c.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, []byte("v2"), data)
}

func TestMemoryClient_Delete(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryClient()

	require.NoError(t, c.Put(ctx, PutInput{Key: "k", Data: []byte("v")}))
	require.NoError(t, c.Delete(ctx, "k"))

	_, err := c.Get(ctx, "k")
	assert.True(t, errors.Is(err, ErrBlobNotFound))
}

func TestMemoryClient_DeleteNonexistent(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryClient()

	// Deleting a key that doesn't exist should not error
	err := c.Delete(ctx, "nonexistent")
	assert.NoError(t, err)
}

func TestMemoryClient_Keys(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryClient()

	require.NoError(t, c.Put(ctx, PutInput{Key: "b", Data: []byte("1")}))
	require.NoError(t, c.Put(ctx, PutInput{Key: "a", Data: []byte("2")}))
	require.NoError(t, c.Put(ctx, PutInput{Key: "c", Data: []byte("3")}))

	keys := c.Keys()
	sort.Strings(keys)
	assert.Equal(t, []string{"a", "b", "c"}, keys)
}

func TestMemoryClient_KeysEmpty(t *testing.T) {
	c := NewMemoryClient()
	assert.Empty(t, c.Keys())
}

func TestMemoryClient_DataIsolation(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryClient()

	original := []byte("original")
	require.NoError(t, c.Put(ctx, PutInput{Key: "k", Data: original}))

	// Mutate the original slice after Put
	original[0] = 'X'

	data, err := c.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, byte('o'), data[0], "Put should copy input data")

	// Mutate the returned slice
	data[0] = 'Y'

	data2, err := c.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, byte('o'), data2[0], "Get should return a copy")
}

func TestMemoryClient_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryClient()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "key"
			_ = c.Put(ctx, PutInput{Key: key, Data: []byte("data")})
			_, _ = c.Get(ctx, key)
			_ = c.Delete(ctx, key)
			_ = c.Keys()
		}(i)
	}
	wg.Wait()
}

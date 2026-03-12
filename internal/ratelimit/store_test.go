package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_IsRateLimited_NotLimited(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	connID := apid.New(apid.PrefixConnection)

	_, limited, err := store.IsRateLimited(ctx, connID)
	require.NoError(t, err)
	assert.False(t, limited)
}

func TestStore_SetAndCheckRateLimited(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	connID := apid.New(apid.PrefixConnection)

	err := store.SetRateLimited(ctx, connID, 30*time.Second)
	require.NoError(t, err)

	remaining, limited, err := store.IsRateLimited(ctx, connID)
	require.NoError(t, err)
	assert.True(t, limited)
	assert.True(t, remaining > 0)
	assert.True(t, remaining <= 30*time.Second)
}

func TestStore_ConsecutiveCount(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	connID := apid.New(apid.PrefixConnection)

	// Initially zero
	count, err := store.GetConsecutive429Count(ctx, connID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Increment
	count, err = store.IncrementConsecutive429Count(ctx, connID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	count, err = store.IncrementConsecutive429Count(ctx, connID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Read back
	count, err = store.GetConsecutive429Count(ctx, connID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Clear
	err = store.ClearConsecutive429Count(ctx, connID)
	require.NoError(t, err)

	count, err = store.GetConsecutive429Count(ctx, connID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

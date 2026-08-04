package apredis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestNewRedis_ClientsHaveIndependentLifecycles(t *testing.T) {
	server, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(server.Close)

	redisConfig := &config.RedisReal{
		Provider: config.RedisProviderRedis,
		Address:  server.Addr(),
	}

	first, err := NewRedis(context.Background(), redisConfig)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })

	second, err := NewRedis(context.Background(), redisConfig)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	require.NotSame(t, first, second)
	require.NoError(t, first.Close())
	require.NoError(t, second.Ping(context.Background()).Err())
}

func TestNewMiniredis_ClientsHaveIndependentLifecycles(t *testing.T) {
	first, err := NewMiniredis(nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })

	second, err := NewMiniredis(nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	require.NotSame(t, first, second)
	require.NoError(t, first.Close())
	require.NoError(t, second.Ping(context.Background()).Err())
}

package apredis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

// NewRedis creates a Redis client for a real Redis instance.
//
// Parameters:
//   - redisConfig: the configuration for the Redis instance
//   - opts: optional functional options (e.g. WithTelemetry) for instrumenting
//     the client with OTel tracing and / or metrics
func NewRedis(ctx context.Context, redisConfig *config.RedisReal, opts ...Option) (Client, error) {
	resolved := resolveOpts(opts)
	cfg, err := redisConfig.ToRedisOptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to convert redis config to redis options: %w", err)
	}

	client := redis.NewClient(cfg)
	if err := instrumentClient(client, resolved); err != nil {
		_ = client.Close()
		return nil, err
	}

	// Test the connection to ensure it's working before handing ownership to
	// the caller. Each dependency manager owns this client and can close it
	// without interrupting sibling services in an all-in-one server process.
	if _, err := client.Ping(context.Background()).Result(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to real Redis server: %w", err)
	}

	return client, nil
}

package apredis

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/config"
)

var redisClient *redis.Client
var redisOnce sync.Once
var redisErr error

// NewRedis creates a new redis connection to a real redis instance.
//
// Parameters:
// - redisConfig: the configuration for the Redis instance
// - secretKey: the AES key used to secure cursors
func NewRedis(ctx context.Context, redisConfig *config.RedisReal) (Client, error) {
	if redisClient == nil {
		redisOnce.Do(func() {
			var err error

			cfg, err := redisConfig.ToRedisOptions(ctx)
			if err != nil {
				redisErr = errors.Wrap(err, "failed to convert redis config to redis options")
				return
			}

			// Configure the Redis client with the provided configuration
			redisClient = redis.NewClient(cfg)

			// Test the connection to ensure it's working
			_, err = redisClient.Ping(context.Background()).Result()
			if err != nil {
				redisErr = errors.Wrap(err, "failed to connect to real Redis server")
				return
			}
		})
	}

	if redisErr != nil {
		return nil, redisErr
	}

	return redisClient, nil
}

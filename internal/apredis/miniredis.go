package apredis

import (
	"context"
	"sync"

	"github.com/alicebob/miniredis/v2"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/internal/config"
)

var miniredisServer *miniredis.Miniredis
var miniredisClient *redis.Client
var miniredisMutex sync.Mutex
var miniredisErr error

// NewMiniredis creates a new redis connection to a miniredis instance.
//
// Parameters:
// - redisConfig: the configuration for the miniredis instance
func NewMiniredis(redisConfig *config.RedisMiniredis) (Client, error) {
	if miniredisServer == nil {
		miniredisMutex.Lock()
		defer miniredisMutex.Unlock()

		// Check again now that we are the primary
		if miniredisServer == nil {
			var err error
			// Create a new instance of miniredis for testing purposes
			miniredisServer, err = miniredis.Run()
			if err != nil {
				miniredisErr = errors.Wrap(err, "failed to start miniredis server")
			}

			// Configure the Redis client to use the miniredis instance
			miniredisClient = redis.NewClient(&redis.Options{
				Addr:     miniredisServer.Addr(),
				Protocol: 2, // Needed because RESP3 is unstable for Redis Search
			})

			// Test the connection to ensure it's working
			_, err = miniredisClient.Ping(context.Background()).Result()
			if err != nil {
				miniredisServer.Close()
				miniredisErr = errors.Wrap(err, "failed to connect to miniredis client")
			}
		}
	}

	if miniredisErr != nil {
		return nil, miniredisErr
	}

	return miniredisClient, nil
}

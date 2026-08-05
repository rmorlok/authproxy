package apredis

import (
	"context"
	"fmt"
	"sync"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

var miniredisServer *miniredis.Miniredis
var miniredisMutex sync.Mutex
var miniredisErr error

// NewMiniredis creates a Redis client connected to the shared miniredis
// instance. Each caller receives its own client and is responsible for
// closing it.
//
// Parameters:
//   - redisConfig: the configuration for the miniredis instance
//   - opts: optional functional options (e.g. WithTelemetry) for instrumenting
//     the client with OTel tracing and / or metrics
func NewMiniredis(redisConfig *config.RedisMiniredis, opts ...Option) (Client, error) {
	resolved := resolveOpts(opts)
	miniredisMutex.Lock()
	if miniredisServer == nil {
		var err error
		miniredisServer, err = miniredis.Run()
		if err != nil {
			miniredisErr = fmt.Errorf("failed to start miniredis server: %w", err)
		}
	}
	server := miniredisServer
	err := miniredisErr
	miniredisMutex.Unlock()

	if err != nil {
		return nil, err
	}

	client := redis.NewClient(&redis.Options{
		Addr:     server.Addr(),
		Protocol: 2, // Needed because RESP3 is unstable for Redis Search
	})
	if err := instrumentClient(client, resolved); err != nil {
		_ = client.Close()
		return nil, err
	}

	if _, err := client.Ping(context.Background()).Result(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to miniredis client: %w", err)
	}

	return client, nil
}

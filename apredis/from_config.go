package apredis

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
)

// NewForRoot creates a new redis client from the specified configuration. The type of the client
// returned will be determined by the configuration.
func NewForRoot(ctx context.Context, root *config.Root) (Client, error) {
	redisConfig := root.Redis

	switch v := redisConfig.(type) {
	case *config.RedisMiniredis:
		return NewMiniredis(v)
	case *config.RedisReal:
		return NewRedis(ctx, v)
	default:
		return nil, errors.New("redis type not supported")
	}
}

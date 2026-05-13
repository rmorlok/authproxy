package apredis

import (
	"context"
	"errors"

	"github.com/rmorlok/authproxy/internal/schema/config"
)

// NewForRoot creates a new redis client from the specified configuration. The type of the client
// returned will be determined by the configuration. Optional Options enable
// telemetry instrumentation; without them the returned client is a plain,
// uninstrumented redis.Client identical to the historic behaviour.
func NewForRoot(ctx context.Context, root *config.Root, opts ...Option) (Client, error) {
	switch v := root.Redis.InnerVal.(type) {
	case *config.RedisMiniredis:
		return NewMiniredis(v, opts...)
	case *config.RedisReal:
		return NewRedis(ctx, v, opts...)
	default:
		return nil, errors.New("redis type not supported")
	}
}

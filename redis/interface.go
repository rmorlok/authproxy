package redis

import (
	"context"
	"github.com/redis/go-redis/v9"
)

//go:generate mockgen -source=./interface.go -destination=./mock/redis.go -package=mock
type R interface {
	Ping(ctx context.Context) bool
	Close() error
	Client() *redis.Client
	NewMutex(key string, options ...MutexOption) Mutex
}
package apredis

import (
	"context"
	"time"

	v9 "github.com/redis/go-redis/v9"
)

//go:generate mockgen -source=./interface.go -destination=./mock/redis.go -package=mock
type Client interface {
	v9.Cmdable
	WithTimeout(timeout time.Duration) *v9.Client
	Conn() *v9.Conn
	Do(ctx context.Context, args ...interface{}) *v9.Cmd
	Process(ctx context.Context, cmd v9.Cmder) error
	Options() *v9.Options
	PoolStats() *v9.PoolStats
	Pipelined(ctx context.Context, fn func(v9.Pipeliner) error) ([]v9.Cmder, error)
	Pipeline() v9.Pipeliner
	TxPipelined(ctx context.Context, fn func(v9.Pipeliner) error) ([]v9.Cmder, error)
	TxPipeline() v9.Pipeliner
	Subscribe(ctx context.Context, channels ...string) *v9.PubSub
	PSubscribe(ctx context.Context, channels ...string) *v9.PubSub
	SSubscribe(ctx context.Context, channels ...string) *v9.PubSub
	Watch(ctx context.Context, fn func(*v9.Tx) error, keys ...string) error

	AddHook(v9.Hook)
	Close() error
}

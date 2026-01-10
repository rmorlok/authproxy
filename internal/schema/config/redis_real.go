package config

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type RedisReal struct {
	Provider RedisProvider `json:"provider" yaml:"provider"`

	// The network type, either tcp or unix.
	// Default is tcp.
	Network string `json:"network" yaml:"network"`

	// host:port address.
	Address string `json:"address" yaml:"address"`

	// Protocol 2 or 3. Use the version to negotiate RESP version with redis-server.
	// Default is 3.
	Protocol int `json:"protocol" yaml:"protocol"`

	// Use the specified Username to authenticate the current connection
	// with one of the connections defined in the ACL list when connecting
	// to a Redis 6.0 instance, or greater, that is using the Redis ACL system.
	Username *StringValue `json:"username" yaml:"username"`

	// Optional password. Must match the password specified in the
	// requirepass server configuration option (if connecting to a Redis 5.0 instance, or lower),
	// or the User Password when connecting to a Redis 6.0 instance, or greater,
	// that is using the Redis ACL system.
	Password *StringValue `json:"password" yaml:"password"`

	// Database to be selected after connecting to the server.
	DB int `json:"db" yaml:"db"`
}

func (d *RedisReal) GetProvider() RedisProvider {
	return RedisProviderRedis
}

func (d *RedisReal) ToRedisOptions(ctx context.Context) (*redis.Options, error) {
	protocol := 2 // Needed because RESP3 is unstable for Redis Search
	if d.Protocol == 3 {
		// This will break the request log features
		protocol = 3
	}

	options := redis.Options{
		Addr:                  d.Address,
		Network:               d.Network,
		Protocol:              protocol,
		DB:                    d.DB, // Redis database to connect to
		ContextTimeoutEnabled: true,
	}

	if d.Username != nil && d.Username.HasValue(ctx) {
		username, err := d.Username.GetValue(ctx)
		if err != nil {
			return nil, err
		}
		options.Username = username
	}

	if d.Password != nil && d.Password.HasValue(ctx) {
		password, err := d.Password.GetValue(ctx)
		if err != nil {
			return nil, err
		}
		options.Password = password
	}

	return &options, nil
}

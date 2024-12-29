package config

import (
	"fmt"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/context"
	"gopkg.in/yaml.v3"
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
	Username StringValue `json:"username" yaml:"username"`

	// Optional password. Must match the password specified in the
	// requirepass server configuration option (if connecting to a Redis 5.0 instance, or lower),
	// or the User Password when connecting to a Redis 6.0 instance, or greater,
	// that is using the Redis ACL system.
	Password StringValue `json:"password" yaml:"password"`

	// Database to be selected after connecting to the server.
	DB int `json:"db" yaml:"db"`
}

func (d *RedisReal) GetProvider() RedisProvider {
	return RedisProviderRedis
}

func (d *RedisReal) ToRedisOptions(ctx context.Context) (*redis.Options, error) {
	options := redis.Options{
		Addr:                  d.Address,
		Network:               d.Network,
		Protocol:              d.Protocol,
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

func (sa *RedisReal) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("redis expected a mapping node, got %s", KindToString(value.Kind))
	}

	var username StringValue
	var password StringValue

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "username":
			if username, err = stringValueUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "password":
			if password, err = stringValueUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		}

		if matched {
			// Remove the key/value from the raw unmarshalling, and pull back our index
			// because of the changing slice size to the left of what we are indexing
			value.Content = append(value.Content[:i], value.Content[i+2:]...)
			i -= 2
		}
	}

	// Let the rest unmarshall normally
	type RawType RedisReal
	raw := (*RawType)(sa)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.Username = username
	raw.Password = password

	return nil
}

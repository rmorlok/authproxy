package config


type RedisProvider string

const (
	RedisProviderMiniredis RedisProvider = "miniredis"
	RedisProviderRedis     RedisProvider = "redis"
)

// RedisImpl is the interface implemented by concrete Redis configurations.
type RedisImpl interface {
	GetProvider() RedisProvider
}

// Redis is the holder for a RedisImpl instance.
type Redis struct {
	InnerVal RedisImpl `json:"-" yaml:"-"`
}

func (r *Redis) GetProvider() RedisProvider {
	if r == nil || r.InnerVal == nil {
		return ""
	}
	return r.InnerVal.GetProvider()
}

var _ RedisImpl = (*Redis)(nil)

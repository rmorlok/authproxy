package config

type RedisMiniredis struct {
	Provider RedisProvider `json:"provider" yaml:"provider"`
}

func (d *RedisMiniredis) GetProvider() RedisProvider {
	return RedisProviderMiniredis
}

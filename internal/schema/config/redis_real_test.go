package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRedisReal(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("no username password", func(t *testing.T) {
			data := `
provider: redis
address: localhost:6379
network: tcp
protocol: 2
`
			var redis Redis
			assert.NoError(yaml.Unmarshal([]byte(data), &redis))
			assert.Equal(Redis{InnerVal: &RedisReal{
				Provider: RedisProviderRedis,
				Address:  "localhost:6379",
				Network:  "tcp",
				Protocol: 2,
			}}, redis)
		})
		t.Run("username password straight value", func(t *testing.T) {
			data := `
provider: redis
address: localhost:6379
network: tcp
protocol: 2
username: bobdole
password: secret
`
			var redis Redis
			assert.NoError(yaml.Unmarshal([]byte(data), &redis))
			assert.Equal(Redis{InnerVal: &RedisReal{
				Provider: RedisProviderRedis,
				Address:  "localhost:6379",
				Network:  "tcp",
				Protocol: 2,
				Username: NewStringValueDirectInline("bobdole"),
				Password: NewStringValueDirectInline("secret"),
			}}, redis)
		})
		t.Run("username password env var", func(t *testing.T) {
			data := `
provider: redis
address: localhost:6379
network: tcp
protocol: 2
username:
  env_var: REDIS_USERNAME
password:
  env_var: REDIS_PASSWORD
`
			var redis Redis
			assert.NoError(yaml.Unmarshal([]byte(data), &redis))
			assert.Equal(Redis{InnerVal: &RedisReal{
				Provider: RedisProviderRedis,
				Address:  "localhost:6379",
				Network:  "tcp",
				Protocol: 2,
				Username: &StringValue{&StringValueEnvVar{
					EnvVar: "REDIS_USERNAME",
				}},
				Password: &StringValue{&StringValueEnvVar{
					EnvVar: "REDIS_PASSWORD",
				}},
			}}, redis)
		})
		t.Run("miniredis", func(t *testing.T) {
			data := `
provider: miniredis
`
			var redis Redis
			assert.NoError(yaml.Unmarshal([]byte(data), &redis))
			assert.Equal(Redis{InnerVal: &RedisMiniredis{
				Provider: RedisProviderMiniredis,
			}}, redis)
		})
	})
}

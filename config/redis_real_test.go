package config

import (
	"testing"

	"github.com/stretchr/testify/require"
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
			redis, err := UnmarshallYamlRedisString(data)
			assert.NoError(err)
			assert.Equal(&RedisReal{
				Provider: RedisProviderRedis,
				Address:  "localhost:6379",
				Network:  "tcp",
				Protocol: 2,
			}, redis)
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
			expected := &RedisReal{
				Provider: RedisProviderRedis,
				Address:  "localhost:6379",
				Network:  "tcp",
				Protocol: 2,
				Username: NewStringValueDirectInline("bobdole"),
				Password: NewStringValueDirectInline("secret"),
			}
			redis, err := UnmarshallYamlRedisString(data)
			assert.NoError(err)
			assert.Equal(expected, redis)
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
			redis, err := UnmarshallYamlRedisString(data)
			assert.NoError(err)
			assert.Equal(&RedisReal{
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
			}, redis)
		})
	})
}

package apredis

import (
	"github.com/rmorlok/authproxy/internal/config"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

func MustApplyTestConfig(cfg config.C) (config.C, Client) {
	// Avoid shared singletons for test cases, while still going through wireup logic
	miniredisServerPrevious := miniredisServer
	miniredisClientPrevious := miniredisClient
	miniredisErrPrevious := miniredisErr
	defer func() {
		miniredisServer = miniredisServerPrevious
		miniredisClient = miniredisClientPrevious
		miniredisErr = miniredisErrPrevious
	}()
	miniredisServer = nil
	miniredisClient = nil
	miniredisErr = nil

	if cfg == nil {
		cfg = config.FromRoot(&sconfig.Root{})
	}

	root := cfg.GetRoot()

	if root == nil {
		panic("No root in config")
	}

	redisCfg := &sconfig.RedisMiniredis{
		Provider: sconfig.RedisProviderMiniredis,
	}
	root.Redis = &sconfig.Redis{InnerVal: redisCfg}
	if root.SystemAuth.GlobalAESKey == nil {
		root.SystemAuth.GlobalAESKey = &sconfig.KeyData{InnerVal: &sconfig.KeyDataRandomBytes{}}
	}

	r, err := NewMiniredis(redisCfg)
	if err != nil {
		panic(err)
	}

	return cfg, r
}

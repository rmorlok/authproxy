package apredis

import "github.com/rmorlok/authproxy/internal/config"

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
		cfg = config.FromRoot(&config.Root{})
	}

	root := cfg.GetRoot()

	if root == nil {
		panic("No root in config")
	}

	redisCfg := &config.RedisMiniredis{
		Provider: config.RedisProviderMiniredis,
	}
	root.Redis = redisCfg
	if root.SystemAuth.GlobalAESKey == nil {
		root.SystemAuth.GlobalAESKey = &config.KeyDataRandomBytes{}
	}

	r, err := NewMiniredis(redisCfg)
	if err != nil {
		panic(err)
	}

	return cfg, r
}

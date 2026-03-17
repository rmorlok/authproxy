package apredis

import (
	"github.com/alicebob/miniredis/v2"
	"github.com/rmorlok/authproxy/internal/config"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// mustApplyTestConfigInternal is the shared implementation that returns both the client and server.
func mustApplyTestConfigInternal(cfg config.C) (config.C, Client, *miniredis.Miniredis) {
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

	// Capture server before defer restores the global
	server := miniredisServer

	return cfg, r, server
}

func MustApplyTestConfig(cfg config.C) (config.C, Client) {
	cfg, client, _ := mustApplyTestConfigInternal(cfg)
	return cfg, client
}

// MustApplyTestConfigWithServer is like MustApplyTestConfig but also returns the underlying
// miniredis server, allowing tests to call FastForward to advance time for TTL expiry.
func MustApplyTestConfigWithServer(cfg config.C) (config.C, Client, *miniredis.Miniredis) {
	return mustApplyTestConfigInternal(cfg)
}

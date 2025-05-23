package httpf

import (
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/redis"
	"gopkg.in/h2non/gentleman.v2"
	"sync"
)

type clientFactory struct {
	cfg          config.C
	wrapper      redis.R
	toplevel     *gentleman.Client
	topLevelOnce sync.Once
}

func CreateFactory(cfg config.C, wrapper redis.R) F {
	return &clientFactory{
		cfg:     cfg,
		wrapper: wrapper,
	}
}

func (f *clientFactory) NewTopLevel() *gentleman.Client {
	f.topLevelOnce.Do(func() {
		f.toplevel = gentleman.New()
	})

	return gentleman.New().UseParent(f.toplevel)
}

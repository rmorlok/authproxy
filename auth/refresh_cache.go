package auth

import (
	"sync"
	"sync/atomic"
)

// RefreshCache defines interface storing and retrieving refreshed tokens
type RefreshCache interface {
	Get(key string) (value Claims, ok bool)
	Set(key string, value Claims)
}

type memoryRefreshCache struct {
	data map[string]Claims
	sync.RWMutex
	hits, misses int32
}

func NewMemoryRefreshCache() RefreshCache {
	return newMemoryRefreshCache()
}

func newMemoryRefreshCache() *memoryRefreshCache {
	return &memoryRefreshCache{data: make(map[string]Claims)}
}

func (c *memoryRefreshCache) Get(key string) (value Claims, ok bool) {
	c.RLock()
	defer c.RUnlock()
	value, ok = c.data[key]
	if ok {
		atomic.AddInt32(&c.hits, 1)
	} else {
		atomic.AddInt32(&c.misses, 1)
	}
	return value, ok
}

func (c *memoryRefreshCache) Set(key string, value Claims) {
	c.Lock()
	defer c.Unlock()
	c.data[key] = value
}

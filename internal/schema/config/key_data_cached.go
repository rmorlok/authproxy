package config

import (
	"sync"
	"time"
)

// cachedKeyFetcher provides TTL-based caching for external key providers.
// All external providers (Vault, AWS, GCP) embed this to avoid repeated
// network calls for key retrieval.
type cachedKeyFetcher struct {
	mu        sync.Mutex
	data      []byte
	fetchedAt time.Time
	ttl       time.Duration
	fetch     func() ([]byte, error)
}

func (c *cachedKeyFetcher) get() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.data != nil && (c.ttl <= 0 || time.Since(c.fetchedAt) < c.ttl) {
		return c.data, nil
	}

	data, err := c.fetch()
	if err != nil {
		return nil, err
	}

	c.data = data
	c.fetchedAt = time.Now()
	return c.data, nil
}
